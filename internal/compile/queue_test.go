package compile

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ponytojas/Refract/internal/auth"
)

// gate is a Compiler that blocks until released and records what ran.
type gate struct {
	mu       sync.Mutex
	started  chan string // the project dir of each compile that starts
	release  chan struct{}
	running  int
	peak     int
	perUser  map[string]int
	overlaps atomic.Int32
}

func newGate() *gate {
	return &gate{started: make(chan string, 100), release: make(chan struct{}, 100), perUser: map[string]int{}}
}

func (g *gate) Compile(ctx context.Context, dir, _ string) (*Result, error) {
	u, _ := auth.UserFrom(ctx)
	g.mu.Lock()
	g.running++
	g.peak = max(g.peak, g.running)
	g.perUser[u.ID]++
	if g.perUser[u.ID] > 1 {
		g.overlaps.Add(1)
	}
	g.mu.Unlock()
	g.started <- dir
	<-g.release
	g.mu.Lock()
	g.running--
	g.perUser[u.ID]--
	g.mu.Unlock()
	return &Result{OK: true}, nil
}

func as(id string) context.Context {
	return auth.WithUser(context.Background(), auth.User{ID: id})
}

type outcome struct {
	dir string
	res *Result
	err error
}

// start runs a compile in the background and returns where its outcome lands.
func start(q *Queue, ctx context.Context, dir string) <-chan outcome {
	ch := make(chan outcome, 1)
	go func() {
		r, err := q.Compile(ctx, dir, "main.tex")
		ch <- outcome{dir, r, err}
	}()
	return ch
}

func waitStarted(t *testing.T, g *gate) string {
	t.Helper()
	select {
	case d := <-g.started:
		return d
	case <-time.After(5 * time.Second):
		t.Fatal("no compile started")
		return ""
	}
}

func noStart(t *testing.T, g *gate) {
	t.Helper()
	select {
	case d := <-g.started:
		t.Fatalf("%s started but should wait", d)
	case <-time.After(50 * time.Millisecond):
	}
}

// settle lets goroutines reach their wait.
func settle() { time.Sleep(30 * time.Millisecond) }

func TestQueueWorkersNeverExceeded(t *testing.T) {
	g := newGate()
	q := &Queue{Next: g, Workers: 3, MaxWait: 10 * time.Second}
	var outs []<-chan outcome
	for i := range 12 {
		outs = append(outs, start(q, as(string(rune('a'+i))), "p"))
	}
	for range 12 {
		waitStarted(t, g)
		g.release <- struct{}{}
	}
	for _, o := range outs {
		if r := <-o; r.err != nil || !r.res.OK {
			t.Fatalf("outcome = %+v", r)
		}
	}
	if g.peak > 3 || g.peak == 0 {
		t.Errorf("peak = %d; want 1..3", g.peak)
	}
}

func TestQueueOneRunningPerAccount(t *testing.T) {
	g := newGate()
	q := &Queue{Next: g, Workers: 4, MaxWait: 10 * time.Second}
	ctx := AsAssistant(as("u"))
	var outs []<-chan outcome
	for range 5 {
		outs = append(outs, start(q, ctx, "p"))
	}
	waitStarted(t, g)
	noStart(t, g) // three free workers, and still one compile
	for range 5 {
		g.release <- struct{}{}
	}
	for range 4 {
		waitStarted(t, g)
	}
	for _, o := range outs {
		<-o
	}
	if g.overlaps.Load() != 0 {
		t.Errorf("one account ran %d compiles at once", g.overlaps.Load())
	}
}

func TestQueueAccountsTakeTurns(t *testing.T) {
	g := newGate()
	q := &Queue{Next: g, Workers: 1, MaxWait: 10 * time.Second}
	hold := start(q, as("hold"), "hold")
	waitStarted(t, g)
	// a has three assistant compiles waiting, b has two; they must alternate.
	var outs []<-chan outcome
	for _, n := range []struct{ user, dir string }{{"a", "a1"}, {"a", "a2"}, {"a", "a3"}, {"b", "b1"}, {"b", "b2"}} {
		outs = append(outs, start(q, AsAssistant(as(n.user)), n.dir))
		settle()
	}
	var order []string
	g.release <- struct{}{} // hold finishes
	for range 5 {
		order = append(order, waitStarted(t, g))
		g.release <- struct{}{}
	}
	<-hold
	for _, o := range outs {
		<-o
	}
	want := []string{"a1", "b1", "a2", "b2", "a3"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v; want %v", order, want)
		}
	}
}

func TestQueueNewInteractiveReplacesWaiting(t *testing.T) {
	g := newGate()
	q := &Queue{Next: g, Workers: 1, MaxWait: 10 * time.Second}
	hold := start(q, as("other"), "hold")
	waitStarted(t, g)
	first := start(q, as("u"), "proj")
	settle()
	second := start(q, as("u"), "proj")
	r := <-first
	if r.err != nil || !r.res.Superseded {
		t.Fatalf("first = %+v; want Superseded", r)
	}
	select {
	case <-hold:
		t.Fatal("the running compile was touched")
	default:
	}
	g.release <- struct{}{}
	if d := waitStarted(t, g); d != "proj" {
		t.Fatalf("started %q; want proj", d)
	}
	g.release <- struct{}{}
	<-hold
	if r := <-second; !r.res.OK {
		t.Errorf("second = %+v", r)
	}
}

func TestQueueAssistantCompilesAreNotReplaced(t *testing.T) {
	g := newGate()
	q := &Queue{Next: g, Workers: 1, MaxWait: 10 * time.Second}
	hold := start(q, as("other"), "hold")
	waitStarted(t, g)
	a1 := start(q, AsAssistant(as("u")), "a1")
	settle()
	a2 := start(q, AsAssistant(as("u")), "a2")
	settle()
	g.release <- struct{}{}
	waitStarted(t, g)
	g.release <- struct{}{}
	waitStarted(t, g)
	g.release <- struct{}{}
	<-hold
	for _, o := range []<-chan outcome{a1, a2} {
		if r := <-o; r.res == nil || r.res.Superseded {
			t.Errorf("assistant compile = %+v", r)
		}
	}
}

func TestQueueInteractiveFirst(t *testing.T) {
	g := newGate()
	q := &Queue{Next: g, Workers: 1, MaxWait: 10 * time.Second}
	hold := start(q, as("hold"), "hold")
	waitStarted(t, g)
	// Waiting: u's assistant compile, v's assistant compile, then u's interactive one.
	outs := []<-chan outcome{start(q, AsAssistant(as("u")), "u-assistant")}
	settle()
	outs = append(outs, start(q, AsAssistant(as("v")), "v-assistant"))
	settle()
	outs = append(outs, start(q, as("u"), "u-interactive"))
	settle()
	var order []string
	g.release <- struct{}{}
	for range 3 {
		order = append(order, waitStarted(t, g))
		g.release <- struct{}{}
	}
	<-hold
	for _, o := range outs {
		<-o
	}
	if order[0] != "u-interactive" || order[1] != "v-assistant" || order[2] != "u-assistant" {
		t.Errorf("order = %v", order)
	}
}

func TestQueueMaxWait(t *testing.T) {
	g := newGate()
	q := &Queue{Next: g, Workers: 1, MaxWait: 50 * time.Millisecond}
	hold := start(q, as("a"), "hold")
	waitStarted(t, g)
	r := <-start(q, as("b"), "late")
	if !errors.Is(r.err, ErrBusy) {
		t.Fatalf("outcome = %+v; want ErrBusy", r)
	}
	// The slot count is intact: after the first finishes, a new compile runs.
	g.release <- struct{}{}
	<-hold
	next := start(q, as("b"), "next")
	waitStarted(t, g)
	g.release <- struct{}{}
	if r := <-next; r.err != nil || !r.res.OK {
		t.Fatalf("next = %+v", r)
	}
	if q.running != 0 || len(q.accounts) != 0 {
		t.Errorf("running=%d accounts=%d; want both 0", q.running, len(q.accounts))
	}
}

func TestQueueCancelledWhileWaiting(t *testing.T) {
	g := newGate()
	q := &Queue{Next: g, Workers: 1, MaxWait: 10 * time.Second}
	hold := start(q, as("a"), "hold")
	waitStarted(t, g)
	ctx, cancel := context.WithCancel(as("b"))
	w := start(q, ctx, "cancelled")
	settle()
	cancel()
	r := <-w
	if r.err != nil || !r.res.Superseded {
		t.Fatalf("outcome = %+v; want Superseded", r)
	}
	g.release <- struct{}{}
	<-hold
	if q.running != 0 || len(q.accounts) != 0 {
		t.Errorf("running=%d accounts=%d; want both 0", q.running, len(q.accounts))
	}
	noStart(t, g) // the cancelled compile never runs
}

// A compile that is granted a slot at the moment it gives up must release it.
func TestQueueGrantRacesGiveUp(t *testing.T) {
	for range 200 {
		g := newGate()
		g.release <- struct{}{} // the compile, if it runs, finishes at once
		q := &Queue{Next: g, Workers: 1, MaxWait: time.Hour}
		ctx, cancel := context.WithCancel(as("a"))
		out := start(q, ctx, "x")
		cancel()
		<-out
		for len(g.started) > 0 {
			<-g.started
		}
		g.release <- struct{}{}
		// Whatever happened, the slot must be free again.
		next := start(q, as("b"), "y")
		select {
		case <-g.started:
			g.release <- struct{}{}
		case <-time.After(5 * time.Second):
			t.Fatal("the slot was never released")
		}
		<-next
	}
}

// A newer compile replaces the waiting one of the same project, not another project's.
func TestQueueReplacesPerProject(t *testing.T) {
	g := newGate()
	q := &Queue{Next: g, Workers: 1, MaxWait: 10 * time.Second}
	hold := start(q, as("a"), "hold")
	waitStarted(t, g)
	p1 := start(q, as("a"), "p1")
	settle()
	p2 := start(q, as("a"), "p2")
	settle()
	p1b := start(q, as("a"), "p1")
	if r := <-p1; r.err != nil || !r.res.Superseded {
		t.Fatalf("first p1 = %+v; want Superseded", r)
	}
	g.release <- struct{}{}
	<-hold
	var order []string
	for range 2 {
		order = append(order, waitStarted(t, g))
		g.release <- struct{}{}
	}
	if r := <-p2; r.err != nil || r.res.Superseded {
		t.Errorf("p2 = %+v; want it to run", r)
	}
	if r := <-p1b; r.err != nil || r.res.Superseded {
		t.Errorf("second p1 = %+v; want it to run", r)
	}
	if order[0] != "p2" || order[1] != "p1" {
		t.Errorf("order = %v; want p2 then p1", order)
	}
}

func TestQueueOwnBusy(t *testing.T) {
	g := newGate()
	q := &Queue{Next: g, Workers: 2, MaxWait: 50 * time.Millisecond}
	hold := start(q, as("a"), "hold")
	waitStarted(t, g)
	r := <-start(q, as("a"), "second")
	if !errors.Is(r.err, ErrOwnBusy) {
		t.Fatalf("outcome = %+v; want ErrOwnBusy", r)
	}
	g.release <- struct{}{}
	<-hold
}

// An account keeps its place in the rotation while its compile runs, even if
// nothing of it waits: arriving then must not jump ahead of the others.
func TestQueueBusyAccountKeepsItsTurn(t *testing.T) {
	g := newGate()
	q := &Queue{Next: g, Workers: 1, MaxWait: 10 * time.Second}
	hold := start(q, as("hold"), "hold")
	waitStarted(t, g)
	outs := []<-chan outcome{hold}
	for _, n := range []struct{ user, dir string }{{"b", "b1"}, {"a", "a1"}, {"b", "b2"}} {
		outs = append(outs, start(q, as(n.user), n.dir))
		settle()
	}
	g.release <- struct{}{} // hold finishes: b1 runs
	if d := waitStarted(t, g); d != "b1" {
		t.Fatalf("started %s; want b1", d)
	}
	g.release <- struct{}{} // a1 runs, with nothing of a waiting
	if d := waitStarted(t, g); d != "a1" {
		t.Fatalf("started %s; want a1", d)
	}
	outs = append(outs, start(q, as("a"), "a2")) // arrives during a1's compile
	settle()
	g.release <- struct{}{}
	if d := waitStarted(t, g); d != "b2" {
		t.Fatalf("started %s; want b2 before a2", d)
	}
	g.release <- struct{}{}
	if d := waitStarted(t, g); d != "a2" {
		t.Fatalf("started %s; want a2", d)
	}
	g.release <- struct{}{}
	for _, o := range outs {
		<-o
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.accounts) != 0 || len(q.order) != 0 || len(q.busy) != 0 {
		t.Errorf("state left over: %d accounts, %d in order, %d busy", len(q.accounts), len(q.order), len(q.busy))
	}
}

// An account that always has a compile arriving as its last one ends can't
// starve another waiting account.
func TestQueueNoStarvation(t *testing.T) {
	g := newGate()
	q := &Queue{Next: g, Workers: 1, MaxWait: 10 * time.Second}
	first := start(q, as("a"), "a0")
	waitStarted(t, g)
	b := start(q, as("b"), "b")
	settle()
	outs := []<-chan outcome{first}
	for i := range 3 {
		outs = append(outs, start(q, as("a"), "a"+string(rune('1'+i))))
		settle()
		g.release <- struct{}{} // the running compile finishes
		d := waitStarted(t, g)
		if d == "b" {
			g.release <- struct{}{}
			<-b
			g.release <- struct{}{}
			for _, o := range outs {
				<-o
			}
			return
		}
	}
	t.Fatal("b never ran while a kept queueing")
}
