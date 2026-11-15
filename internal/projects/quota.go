package projects

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Quota caps what one account may hold. A zero field means no limit.
type Quota struct {
	MaxProjects int
	MaxBytes    int64 // all of the account's projects, with history and trash
}

// ErrOverLimit refuses an operation that would add to an account at its limit.
var ErrOverLimit = errors.New("limit reached")

// OverLimit is the refusal for one limit; it is an ErrOverLimit. Kind is
// "projects" or "storage", so the web can word it in the person's language.
type OverLimit struct {
	Kind string
	Max  int64 // projects, or bytes
}

func (e *OverLimit) Error() string {
	if e.Kind == "projects" {
		return fmt.Sprintf("Your plan allows %d projects, including those in the trash. Delete one forever from the trash to add another.", e.Max)
	}
	return fmt.Sprintf("Your plan's storage (%s) is full, counting projects in the trash. Delete projects forever from the trash to free space; deleted files stay in a project's history.", megabytes(e.Max))
}

func megabytes(n int64) string {
	if n >= 1<<20 {
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d KB", n>>10)
}

func (e *OverLimit) Is(target error) bool { return target == ErrOverLimit }

// A project's real content is its working tree, .git and the assistant's turn
// worktrees (.refract/worktrees: full copies that stay until accepted or
// rejected). The rest of .refract (build output, caches, compile staging) is
// derived and bounded elsewhere, and so are the transient accept-* worktrees.
const derivedDir = ".refract"

var (
	userLocks sync.Map // owner id → *sync.Mutex: count and insert of a new project
	sizeLocks sync.Map // project id → *sizeState
)

// sizeState serializes updates to one project's stored size. growth is
// everything grew() ever added: a measure that walked while saves landed adds
// the growth since its walk began to what it found, so it never under-counts.
type sizeState struct {
	mu     sync.Mutex
	growth int64
}

func sizeFor(id string) *sizeState {
	st, _ := sizeLocks.LoadOrStore(id, &sizeState{})
	return st.(*sizeState)
}

func lockUser(owner string) func() {
	m, _ := userLocks.LoadOrStore(owner, &sync.Mutex{})
	m.(*sync.Mutex).Lock()
	return m.(*sync.Mutex).Unlock
}

// checkQuota refuses when the account is at or over a limit, or when adding
// incoming bytes would pass it. Trashed projects count: they are still on
// disk. It is never called on reads or deletes. A clone, pull or accepted
// turn can overshoot (its size is only known afterwards); then further growth
// is refused until the account is back under.
func (s *Service) checkQuota(ctx context.Context, owner string, adding bool, incoming int64) error {
	if s.QuotaFor == nil {
		return nil
	}
	q, err := s.QuotaFor(ctx, owner)
	if err != nil {
		return err
	}
	if q.MaxProjects == 0 && q.MaxBytes == 0 {
		return nil
	}
	var count int
	var sum int64
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*), coalesce(sum(size_bytes), 0) FROM projects WHERE owner = ?`, owner).Scan(&count, &sum); err != nil {
		return err
	}
	if adding && q.MaxProjects > 0 && count >= q.MaxProjects {
		return &OverLimit{Kind: "projects", Max: int64(q.MaxProjects)}
	}
	if q.MaxBytes > 0 && (sum >= q.MaxBytes || sum+incoming > q.MaxBytes) {
		return &OverLimit{Kind: "storage", Max: q.MaxBytes}
	}
	return nil
}

// CheckStorage refuses when p's owner has no room left.
func (s *Service) CheckStorage(ctx context.Context, p *Project) error {
	return s.checkQuota(ctx, p.Owner, false, 0)
}

// checkQuotaFor is CheckStorage for project id.
func (s *Service) checkQuotaFor(ctx context.Context, id string) error {
	if s.QuotaFor == nil {
		return nil
	}
	p, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	return s.CheckStorage(ctx, p)
}

// dirSize adds up the regular files under dir, counting .refract only for its turn worktrees.
func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if rel, rerr := filepath.Rel(dir, p); rerr == nil {
			if parts := strings.Split(filepath.ToSlash(rel), "/"); parts[0] == derivedDir && len(parts) > 1 {
				switch {
				case parts[1] != "worktrees":
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				case len(parts) > 2 && strings.HasPrefix(parts[2], "accept-"):
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// measure stores what the project takes on disk: working tree and history.
// The walk holds no lock; the update adds what saves grew meanwhile. Failures
// are logged: a failed measurement must not fail a save.
func (s *Service) measure(ctx context.Context, id string) {
	if s.QuotaFor == nil {
		return
	}
	st := sizeFor(id)
	st.mu.Lock()
	g0 := st.growth
	st.mu.Unlock()
	total, err := dirSize(s.Path(id))
	if err == nil {
		st.mu.Lock()
		_, err = s.DB.ExecContext(ctx, `UPDATE projects SET size_bytes = ?, size_measured_at = ? WHERE id = ?`, total+st.growth-g0, time.Now().Unix(), id)
		st.mu.Unlock()
	}
	if err != nil {
		slog.Warn("measuring project", "project", id, "err", err)
	}
}

// grew adds n bytes to the stored size: an upper bound until the next measure.
func (s *Service) grew(ctx context.Context, id string, n int64) {
	if s.QuotaFor == nil || n <= 0 {
		return
	}
	st := sizeFor(id)
	st.mu.Lock()
	defer st.mu.Unlock()
	st.growth += n
	if _, err := s.DB.ExecContext(ctx, `UPDATE projects SET size_bytes = size_bytes + ? WHERE id = ?`, n, id); err != nil {
		slog.Warn("sizing project", "project", id, "err", err)
	}
}

// MeasureAll measures the projects (live and trashed) never measured, which
// existed before sizes were tracked.
func (s *Service) MeasureAll(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM projects WHERE size_measured_at = 0`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	for _, id := range ids {
		s.measure(ctx, id)
	}
	return nil
}

// admitGrowth is the one admission for a save: under the owner's lock it
// refuses when n more bytes would pass the storage limit, and otherwise runs
// publish (which makes the bytes visible) and records n before the next
// admission looks at the total. So parallel saves admit exactly as the same
// saves one after another would: the one that crosses the line is the last.
func (s *Service) admitGrowth(ctx context.Context, p *Project, n func() (int64, error), publish func() error) error {
	defer lockUser(p.Owner)()
	delta, err := n()
	if err != nil {
		return err
	}
	if delta > 0 { // a save that starts under the limit may end over it, as the plan says; one that starts at or over is refused
		if err := s.checkQuota(ctx, p.Owner, false, 0); err != nil {
			return err
		}
	}
	if err := publish(); err != nil {
		return err
	}
	s.grew(ctx, p.ID, delta)
	return nil
}
