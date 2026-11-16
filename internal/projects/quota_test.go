package projects

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ponytojas/Refract/internal/auth"
)

func fixed(q Quota) func(context.Context, string) (Quota, error) {
	return func(context.Context, string) (Quota, error) { return q, nil }
}

func sizeOf(t *testing.T, s *Service, id string) int64 {
	t.Helper()
	var n int64
	if err := s.DB.QueryRow(`SELECT size_bytes FROM projects WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func write(t *testing.T, s *Service, id, name string, n int) error {
	t.Helper()
	return s.WriteFile(ownerCtx, id, name, bytes.NewReader([]byte(strings.Repeat("x", n))))
}

func TestQuotaProjects(t *testing.T) {
	s := newService(t)
	s.QuotaFor = fixed(Quota{MaxProjects: 2})
	for range 2 {
		if _, err := s.Create(ownerCtx, "p"); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.Create(ownerCtx, "p")
	if !errors.Is(err, ErrOverLimit) || !strings.Contains(err.Error(), "allows 2 projects") {
		t.Fatalf("third create: %v", err)
	}
	other := auth.WithUser(context.Background(), auth.User{ID: "other"})
	if _, err := s.Create(other, "p"); err != nil {
		t.Fatal(err)
	}
}

func TestQuotaIsPerAccount(t *testing.T) {
	s := newService(t)
	s.QuotaFor = func(_ context.Context, owner string) (Quota, error) {
		if owner == "owner" {
			return Quota{MaxProjects: 1}, nil
		}
		return Quota{MaxProjects: 3}, nil
	}
	if _, err := s.Create(ownerCtx, "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ownerCtx, "p"); !errors.Is(err, ErrOverLimit) {
		t.Fatalf("second create: %v", err)
	}
	other := auth.WithUser(context.Background(), auth.User{ID: "other"})
	for range 3 {
		if _, err := s.Create(other, "p"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestQuotaStorage(t *testing.T) {
	s := newService(t)
	s.QuotaFor = fixed(Quota{MaxBytes: 1 << 20})
	p, err := s.Create(ownerCtx, "p")
	if err != nil {
		t.Fatal(err)
	}
	id := p.ID
	for _, name := range []string{"a.bin", "b.bin"} {
		if err := write(t, s, id, name, 700<<10); err != nil {
			t.Fatal(err)
		}
	}
	err = write(t, s, id, "c.bin", 10)
	if !errors.Is(err, ErrOverLimit) || !strings.Contains(err.Error(), "storage") {
		t.Fatalf("third write: %v", err)
	}
	if err := s.Trash(ownerCtx, id); err != nil {
		t.Fatal(err)
	}
	// The trash still counts; purging frees it.
	if err := s.Purge(ownerCtx, id); err != nil {
		t.Fatal(err)
	}
	q, err := s.Create(ownerCtx, "q")
	if err != nil {
		t.Fatal(err)
	}
	if err := write(t, s, q.ID, "a.txt", 10); err != nil {
		t.Fatal(err)
	}
}

func TestQuotaCountsTrash(t *testing.T) {
	s := newService(t)
	s.QuotaFor = fixed(Quota{MaxProjects: 1})
	p, err := s.Create(ownerCtx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ownerCtx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ownerCtx, "q"); !errors.Is(err, ErrOverLimit) {
		t.Fatalf("create with a project in the trash: %v", err)
	}
}

func TestQuotaCountsHistoryNotRefractDir(t *testing.T) {
	s := newService(t)
	s.QuotaFor = fixed(Quota{MaxBytes: 1 << 30})
	res, err := s.Import(ownerCtx, "p", ZipSource(makeZip(t,
		entry{name: "main.tex", body: doc},
		entry{name: "big.bin", body: randomString(300 << 10)},
	)))
	if err != nil {
		t.Fatal(err)
	}
	id := res.Project.ID
	imported := sizeOf(t, s, id)
	if imported < 300<<10 {
		t.Fatalf("size after import = %d", imported)
	}
	_, dir, err := s.StartTurn(ownerCtx, id, "t1")
	if err != nil {
		t.Fatal(err)
	}
	imported = sizeOf(t, s, id) // includes the turn's worktree copy
	if imported < 600<<10 {
		t.Fatalf("size with a pending-review worktree = %d, want its copy counted", imported)
	}
	if err := os.WriteFile(filepath.Join(s.Path(id), ".refract", "cache.bin"), make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	s.measure(ownerCtx, id)
	if n := sizeOf(t, s, id); n != imported {
		t.Fatalf("size with a cache in .refract = %d, want %d", n, imported)
	}
	// Agent writes in the worktree are counted once the turn finishes.
	if err := os.WriteFile(filepath.Join(dir, "agent.bin"), []byte(randomString(200<<10)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishTurn(ownerCtx, id, "t1", "HEAD", "ai: work"); err != nil {
		t.Fatal(err)
	}
	if n := sizeOf(t, s, id); n < imported+200<<10 {
		t.Fatalf("size after the turn = %d, want at least %d", n, imported+200<<10)
	}
}

func TestQuotaParallelSavesCannotOvershoot(t *testing.T) {
	s := newService(t)
	s.QuotaFor = fixed(Quota{MaxBytes: 1 << 20})
	p, err := s.Create(ownerCtx, "p")
	if err != nil {
		t.Fatal(err)
	}
	base := sizeOf(t, s, p.ID)
	const n = 8
	var ok atomic.Int32
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := write(t, s, p.ID, fmt.Sprintf("f%d.txt", i), 700<<10); err == nil {
				ok.Add(1)
			} else if !errors.Is(err, ErrOverLimit) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if want := int32(2); ok.Load() != want {
		t.Fatalf("%d saves succeeded under a 1 MiB quota from %d bytes, want %d (as one after another)", ok.Load(), base, want)
	}
}

func TestQuotaReplacingAFileAddsOnlyTheDifference(t *testing.T) {
	s := newService(t)
	s.QuotaFor = fixed(Quota{MaxBytes: 1 << 30})
	p, err := s.Create(ownerCtx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := write(t, s, p.ID, "a.txt", 1000); err != nil {
		t.Fatal(err)
	}
	before := sizeOf(t, s, p.ID)
	for range 5 {
		if err := write(t, s, p.ID, "a.txt", 1000); err != nil {
			t.Fatal(err)
		}
	}
	if n := sizeOf(t, s, p.ID); n != before {
		t.Fatalf("size after autosaves = %d, want %d", n, before)
	}
	if err := write(t, s, p.ID, "a.txt", 10); err != nil {
		t.Fatal(err)
	}
	if n := sizeOf(t, s, p.ID); n != before {
		t.Fatalf("shrinking lowered the estimate: %d, want %d", n, before)
	}
}

func TestQuotaImportRefusedWhenItWouldPassTheLimit(t *testing.T) {
	s := newService(t)
	s.QuotaFor = fixed(Quota{MaxBytes: 800 << 10})
	zip := func() Source {
		return ZipSource(makeZip(t, entry{name: "main.tex", body: doc}, entry{name: "big.bin", body: randomString(300 << 10)}))
	}
	if _, err := s.Import(ownerCtx, "a", zip()); err != nil {
		t.Fatal(err)
	}
	_, err := s.Import(ownerCtx, "b", zip())
	var ol *OverLimit
	if !errors.Is(err, ErrOverLimit) || !errors.As(err, &ol) || ol.Kind != "storage" {
		t.Fatalf("second import: %v", err)
	}
	if !strings.Contains(err.Error(), "from the trash") {
		t.Fatalf("message doesn't say how to free space: %v", err)
	}
	var n int
	s.DB.QueryRow(`SELECT count(*) FROM projects`).Scan(&n)
	if n != 1 {
		t.Fatalf("%d projects after a refused import", n)
	}
}

func TestQuotaParallelCreates(t *testing.T) {
	s := newService(t)
	s.QuotaFor = fixed(Quota{MaxProjects: 3})
	var wg sync.WaitGroup
	var ok atomic.Int32
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Create(ownerCtx, "p"); err == nil {
				ok.Add(1)
			} else if !errors.Is(err, ErrOverLimit) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 3 {
		t.Fatalf("%d creates succeeded, want 3", ok.Load())
	}
}

func TestQuotaMeasureNeverUndercountsConcurrentSaves(t *testing.T) {
	s := newService(t)
	s.QuotaFor = fixed(Quota{MaxBytes: 1 << 30})
	p, err := s.Create(ownerCtx, "p")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				s.measure(ownerCtx, p.ID)
			}
		}
	}()
	for i := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 15 {
				if err := write(t, s, p.ID, fmt.Sprintf("f%d-%d.txt", i, j), 1000); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	close(stop)
	disk, err := dirSize(s.Path(p.ID))
	if err != nil {
		t.Fatal(err)
	}
	if n := sizeOf(t, s, p.ID); n < disk {
		t.Fatalf("stored %d, on disk %d", n, disk)
	}
}

func randomString(n int) string {
	b := make([]byte, n)
	x := uint32(12345)
	for i := range b {
		x = x*1664525 + 1013904223
		b[i] = byte('a' + x>>24%26)
	}
	return string(b)
}

func TestQuotaOverCountIsCorrected(t *testing.T) {
	s := newService(t)
	s.QuotaFor = fixed(Quota{MaxBytes: 1 << 30})
	p, err := s.Create(ownerCtx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := write(t, s, p.ID, "f.txt", 200<<10); err != nil {
		t.Fatal(err)
	}
	s.grew(ownerCtx, p.ID, 400<<10) // drift an estimate can have
	if n := sizeOf(t, s, p.ID); n < 600<<10 {
		t.Fatalf("size before checkpoint = %d", n)
	}
	if _, err := s.Checkpoint(ownerCtx, p.ID, "save"); err != nil {
		t.Fatal(err)
	}
	// One 200 KB file, one compressed copy in history (repeated bytes compress well).
	if n := sizeOf(t, s, p.ID); n >= 500<<10 {
		t.Fatalf("size after checkpoint = %d", n)
	}
}

// At the storage limit, saving a file again at the same size or smaller still
// works; only growing it is refused.
func TestQuotaAtLimitAllowsSavesThatDontGrow(t *testing.T) {
	s := newService(t)
	s.QuotaFor = fixed(Quota{})
	p, err := s.Create(ownerCtx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := write(t, s, p.ID, "a.txt", 1000); err != nil {
		t.Fatal(err)
	}
	s.QuotaFor = fixed(Quota{MaxBytes: sizeOf(t, s, p.ID)})
	if err := write(t, s, p.ID, "a.txt", 1000); err != nil {
		t.Fatalf("same size at the limit: %v", err)
	}
	if err := write(t, s, p.ID, "a.txt", 100); err != nil {
		t.Fatalf("smaller at the limit: %v", err)
	}
	if b, _, err := s.ReadFile(ownerCtx, p.ID, "a.txt"); err != nil || len(b) != 100 {
		t.Fatalf("after shrinking: %d bytes, %v", len(b), err)
	}
	s.grew(ownerCtx, p.ID, 1<<20) // over the limit now
	if err := write(t, s, p.ID, "a.txt", 50); err != nil {
		t.Fatalf("smaller over the limit: %v", err)
	}
	if err := write(t, s, p.ID, "a.txt", 60); !errors.Is(err, ErrOverLimit) {
		t.Fatalf("growing over the limit: %v; want refused", err)
	}
	if err := write(t, s, p.ID, "b.txt", 1); !errors.Is(err, ErrOverLimit) {
		t.Fatalf("new file over the limit: %v; want refused", err)
	}
}

func TestQuotaNeverBlocksShrinking(t *testing.T) {
	s := newService(t)
	s.QuotaFor = fixed(Quota{})
	p, err := s.Create(ownerCtx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := write(t, s, p.ID, "a.txt", 100<<10); err != nil {
		t.Fatal(err)
	}
	s.grew(ownerCtx, p.ID, 1<<20)
	s.QuotaFor = fixed(Quota{MaxBytes: 1 << 10, MaxProjects: 1})
	if err := write(t, s, p.ID, "b.txt", 1); !errors.Is(err, ErrOverLimit) {
		t.Fatalf("write over the limit: %v", err)
	}
	if _, _, err := s.ReadFile(ownerCtx, p.ID, "a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(ownerCtx, p.ID, "a.txt"); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := s.Export(ownerCtx, p.ID, &buf); err != nil {
		t.Fatal(err)
	}
	if err := s.Trash(ownerCtx, p.ID); err != nil {
		t.Fatal(err)
	}
}

func TestNoQuotaNoWork(t *testing.T) {
	s := newService(t)
	p, err := s.Create(ownerCtx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := write(t, s, p.ID, "a.txt", 1000); err != nil {
		t.Fatal(err)
	}
	if n := sizeOf(t, s, p.ID); n != 0 {
		t.Fatalf("size = %d, want 0", n)
	}
}

func TestMeasureAll(t *testing.T) {
	s := newService(t)
	s.QuotaFor = fixed(Quota{})
	var ids []string
	for range 2 {
		p, err := s.Create(ownerCtx, "p")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
	}
	if _, err := s.DB.Exec(`UPDATE projects SET size_bytes = 0, size_measured_at = 0`); err != nil {
		t.Fatal(err)
	}
	if err := s.MeasureAll(ownerCtx); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if sizeOf(t, s, id) == 0 {
			t.Fatalf("%s not measured", id)
		}
	}
	// Measured projects are left alone.
	if _, err := s.DB.Exec(`UPDATE projects SET size_bytes = 7`); err != nil {
		t.Fatal(err)
	}
	if err := s.MeasureAll(ownerCtx); err != nil {
		t.Fatal(err)
	}
	if n := sizeOf(t, s, ids[0]); n != 7 {
		t.Fatalf("measured again: %d", n)
	}
}
