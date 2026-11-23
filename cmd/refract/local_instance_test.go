package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// localChild is a real `refract local` process: the test binary re-executed.
type localChild struct {
	cmd  *exec.Cmd
	out  *bytes.Buffer
	url  string
	done chan struct{}
}

func localEnv(dataDir string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); !strings.HasPrefix(name, "REFRACT_") {
			env = append(env, kv)
		}
	}
	// A fixed key keeps the children away from the system credential store.
	return append(env, "REFRACT_TEST_LOCAL_CHILD=1", "REFRACT_DATA_DIR="+dataDir,
		"REFRACT_SECRET_KEY=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
}

func startChild(t *testing.T, dataDir string, args ...string) *localChild {
	t.Helper()
	exe, _ := os.Executable()
	c := &localChild{cmd: exec.Command(exe, append([]string{"--no-browser"}, args...)...), out: &bytes.Buffer{}, done: make(chan struct{})}
	c.cmd.Env = localEnv(dataDir)
	pr, pw := io.Pipe()
	c.cmd.Stdout = io.MultiWriter(c.out, pw)
	if err := c.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { c.cmd.Wait(); pw.Close(); close(c.done) }()
	t.Cleanup(func() { c.cmd.Process.Kill(); <-c.done })
	re := regexp.MustCompile(`running at (http://\S+)`)
	sc := bufio.NewScanner(pr)
	for sc.Scan() {
		if m := re.FindStringSubmatch(sc.Text()); m != nil {
			c.url = m[1]
			go io.Copy(io.Discard, pr)
			return c
		}
	}
	t.Fatalf("no address printed: %q", c.out.String())
	return nil
}

func runChild(t *testing.T, dataDir string, args ...string) (string, int) {
	t.Helper()
	exe, _ := os.Executable()
	cmd := exec.Command(exe, args...)
	cmd.Env = localEnv(dataDir)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String() + errOut.String(), code
}

func TestSecondLocalOpensTheFirst(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Refract")
	first := startChild(t, dir)
	out, code := runChild(t, dir, "--no-browser")
	if code != 0 || !strings.Contains(out, "Refract is already running at "+first.url) {
		t.Fatalf("second: exit %d, output %q", code, out)
	}

	out, code = runChild(t, dir, "--status")
	var st instanceStatus
	if err := json.Unmarshal([]byte(out), &st); err != nil || code != 0 {
		t.Fatalf("status: exit %d, output %q", code, out)
	}
	if st.State != stateRunning || st.URL != first.url || st.PID != first.cmd.Process.Pid || st.Version != version {
		t.Errorf("status = %+v", st)
	}
}

func TestLocalStartsAfterKill(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Refract")
	first := startChild(t, dir)
	first.cmd.Process.Kill() // SIGKILL: no chance to clean up
	<-first.done
	if _, err := os.Stat(filepath.Join(dir, localLockFile)); err != nil {
		t.Fatal("expected a stale lock file after kill -9:", err)
	}
	out, code := runChild(t, dir, "--status")
	if code != 1 || !strings.Contains(out, `"state":"stopped"`) {
		t.Fatalf("status after kill: exit %d, %q", code, out)
	}
	second := startChild(t, dir)
	if second.url == "" {
		t.Fatal("no address")
	}
	resp, err := http.Get(second.url + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestLocalDifferentFoldersSideBySide(t *testing.T) {
	root := t.TempDir()
	a := startChild(t, filepath.Join(root, "a"))
	b := startChild(t, filepath.Join(root, "b"))
	if a.url == b.url {
		t.Fatalf("both on %s", a.url)
	}
}

func TestLocalHolderThatDoesNotAnswer(t *testing.T) {
	dir := t.TempDir()
	l, held, err := lockInstance(dir)
	if err != nil || held {
		t.Fatal(err, held)
	}
	defer l.Release()
	// A port nobody listens on.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := "http://" + ln.Addr().String()
	ln.Close()
	if err := l.Record(dead); err != nil {
		t.Fatal(err)
	}
	old := instanceWait
	instanceWait = 500 * time.Millisecond
	defer func() { instanceWait = old }()

	opened := false
	err = startLocal(context.Background(), localOptions{
		getenv: func(string) string { return "" }, defaultDir: dir,
		out: &bytes.Buffer{}, open: func(string) { opened = true },
	})
	if err == nil || opened || !strings.Contains(err.Error(), dead) || !strings.Contains(err.Error(), "pid") || !strings.Contains(err.Error(), "Nothing was deleted") {
		t.Fatalf("err = %v, opened = %v", err, opened)
	}
	var st instanceStatus
	var out bytes.Buffer
	err = reportLocalStatus(context.Background(), &out, dir)
	if e, ok := err.(errExit); !ok || e.code != 1 {
		t.Fatalf("status err = %v", err)
	}
	json.Unmarshal(out.Bytes(), &st)
	if st.State != stateUnresponsive || st.URL != dead {
		t.Errorf("status = %+v", st)
	}
}

func TestLocalIgnoresNonLoopbackAddressInLockFile(t *testing.T) {
	dir := t.TempDir()
	l, _, _ := lockInstance(dir)
	defer l.Release()
	l.Record("http://example.com:80")
	old := instanceWait
	instanceWait = 300 * time.Millisecond
	defer func() { instanceWait = old }()
	if st := checkInstance(context.Background(), dir, 0); st.State != stateUnresponsive || st.URL != "" {
		t.Errorf("status = %+v", st)
	}
}

func TestLocalOpensRunningInstanceInBrowser(t *testing.T) {
	dir := t.TempDir()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go srv.Serve(ln)
	defer srv.Close()
	l, _, _ := lockInstance(dir)
	defer l.Release()
	u := "http://" + ln.Addr().String()
	l.Record(u)

	var opened string
	var out bytes.Buffer
	err := startLocal(context.Background(), localOptions{
		getenv: func(string) string { return "" }, defaultDir: dir,
		out: &out, open: func(u string) { opened = u },
	})
	if err != nil || opened != u || !strings.Contains(out.String(), "Refract is already running at "+u) {
		t.Fatalf("err = %v, opened = %q, out = %q", err, opened, out.String())
	}
}

func TestLocalLockClearsAStaleAddress(t *testing.T) {
	dir := t.TempDir()
	// Left by a run killed with kill -9: its port may now be another program's.
	os.WriteFile(filepath.Join(dir, localLockFile), []byte(`{"url":"http://127.0.0.1:8080","pid":1}`+"\n"), 0o600)
	l, held, err := lockInstance(dir)
	if err != nil || held {
		t.Fatal(err, held)
	}
	defer l.Release()
	info, err := readInstanceInfo(dir)
	if err != nil || info.URL != "" || info.PID != os.Getpid() {
		t.Errorf("lock file after locking: %+v %v", info, err)
	}
}

func TestLocalLockRefusesASymlink(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim")
	os.WriteFile(victim, []byte("keep me"), 0o600)
	if err := os.Symlink(victim, filepath.Join(dir, localLockFile)); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, _, err := lockInstance(dir); err == nil {
		t.Error("locked through a symbolic link")
	}
	var out bytes.Buffer
	if err := reportLocalStatus(context.Background(), &out, dir); err == nil {
		t.Error("status accepted a symbolic link")
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep me" {
		t.Errorf("the link's target changed: %q", b)
	}
}

func TestLocalStatusChangesNothing(t *testing.T) {
	dir := t.TempDir()
	want := []byte(`{"url":"http://127.0.0.1:1","pid":1}` + "\n")
	os.WriteFile(filepath.Join(dir, localLockFile), want, 0o600)
	var out bytes.Buffer
	if err := reportLocalStatus(context.Background(), &out, dir); err == nil || !strings.Contains(out.String(), `"state":"stopped"`) {
		t.Fatalf("status: %v %q", err, out.String())
	}
	if b, _ := os.ReadFile(filepath.Join(dir, localLockFile)); !bytes.Equal(b, want) {
		t.Errorf("status rewrote the lock file: %q", b)
	}
}

func TestLocalCheckDoesNotFollowRedirects(t *testing.T) {
	dir := t.TempDir()
	other := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })}
	oln, _ := net.Listen("tcp", "127.0.0.1:0")
	go other.Serve(oln)
	defer other.Close()
	redirect := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+oln.Addr().String()+"/healthz", http.StatusFound)
	})}
	rln, _ := net.Listen("tcp", "127.0.0.1:0")
	go redirect.Serve(rln)
	defer redirect.Close()
	l, _, _ := lockInstance(dir)
	defer l.Release()
	l.Record("http://" + rln.Addr().String())
	old := instanceWait
	instanceWait = 300 * time.Millisecond
	defer func() { instanceWait = old }()
	if st := checkInstance(context.Background(), dir, 0); st.State != stateUnresponsive {
		t.Errorf("a redirect counted as an answer: %+v", st)
	}
}

func TestLocalWaitsForAnInstanceThatIsStarting(t *testing.T) {
	dir := t.TempDir()
	l, _, _ := lockInstance(dir) // holds the lock, records its pid, doesn't listen yet
	defer l.Release()
	oldWait, oldStarting := instanceWait, instanceStartingWait
	instanceWait, instanceStartingWait = 200*time.Millisecond, 10*time.Second
	defer func() { instanceWait, instanceStartingWait = oldWait, oldStarting }()

	var out bytes.Buffer
	err := reportLocalStatus(context.Background(), &out, dir)
	if e, ok := err.(errExit); !ok || e.code != statusExitStarting {
		t.Fatalf("status err = %v", err)
	}
	var st instanceStatus
	if json.Unmarshal(out.Bytes(), &st); st.State != stateStarting || st.PID != os.Getpid() {
		t.Fatalf("status = %q", out.String())
	}

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	go srv.Serve(ln)
	defer srv.Close()
	u := "http://" + ln.Addr().String()
	go func() { time.Sleep(time.Second); l.Record(u) }() // starts listening after the plain wait
	var opened string
	out.Reset()
	err = startLocal(context.Background(), localOptions{
		getenv: func(string) string { return "" }, defaultDir: dir,
		out: &out, open: func(u string) { opened = u },
	})
	if err != nil || opened != u || !strings.Contains(out.String(), "still starting") {
		t.Fatalf("err = %v, opened = %q, out = %q", err, opened, out.String())
	}
}

func TestLocalGivesUpOnAnInstanceStuckStarting(t *testing.T) {
	dir := t.TempDir()
	l, _, _ := lockInstance(dir)
	defer l.Release()
	oldWait, oldStarting := instanceWait, instanceStartingWait
	instanceWait, instanceStartingWait = 100*time.Millisecond, 400*time.Millisecond
	defer func() { instanceWait, instanceStartingWait = oldWait, oldStarting }()
	err := startLocal(context.Background(), localOptions{
		getenv: func(string) string { return "" }, defaultDir: dir,
		out: &bytes.Buffer{}, open: func(string) {},
	})
	if err == nil || !strings.Contains(err.Error(), "still starting") || !strings.Contains(err.Error(), fmt.Sprintf("pid %d", os.Getpid())) {
		t.Fatalf("err = %v", err)
	}
}

func TestLocalCheckSeesAHolderThatStops(t *testing.T) {
	dir := t.TempDir()
	l, _, _ := lockInstance(dir)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	dead := "http://" + ln.Addr().String()
	ln.Close()
	l.Record(dead) // shutting down: no longer answers, still holds the lock
	go func() { time.Sleep(300 * time.Millisecond); l.Release() }()
	old := instanceWait
	instanceWait = 5 * time.Second
	defer func() { instanceWait = old }()
	t0 := time.Now()
	if st := checkInstance(context.Background(), dir, 0); st.State != stateStopped {
		t.Errorf("status = %+v", st)
	}
	if took := time.Since(t0); took > 3*time.Second {
		t.Errorf("took %v to notice the lock was released", took)
	}
}

func TestLocalStatusLeavesAMissingFolderAlone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never-started")
	var out bytes.Buffer
	if err := reportLocalStatus(context.Background(), &out, dir); err == nil {
		t.Fatal("status of a missing folder succeeded")
	}
	if !strings.Contains(out.String(), `"state":"stopped"`) {
		t.Errorf("out = %q", out.String())
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("status created the folder: %v", err)
	}
}
