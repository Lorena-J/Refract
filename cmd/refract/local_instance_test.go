package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

	"github.com/ponytojas/Refract/internal/auth"
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
	if st := checkInstance(context.Background(), dir); st.State != stateUnresponsive || st.URL != "" {
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
	if b, _ := os.ReadFile(filepath.Join(dir, localLockFile)); len(b) != 0 {
		t.Errorf("stale address kept: %q", b)
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
	if st := checkInstance(context.Background(), dir); st.State != stateStopped {
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

const testEmbedToken = "dGVzdC10b2tlbi10ZXN0LXRva2VuLTEyMzQ1Ng"

// embeddedChild is a real `refract local --embedded` process.
type embeddedChild struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	line   instanceStatus
	done   chan struct{}
}

func startEmbedded(t *testing.T, dataDir string, wantState string, extraEnv ...string) *embeddedChild {
	t.Helper()
	exe, _ := os.Executable()
	c := &embeddedChild{cmd: exec.Command(exe, "--embedded"), stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, done: make(chan struct{})}
	c.cmd.Env = append(localEnv(dataDir), append([]string{embedTokenEnv + "=" + testEmbedToken}, extraEnv...)...)
	var err error
	if c.stdin, err = c.cmd.StdinPipe(); err != nil {
		t.Fatal(err)
	}
	pr, pw := io.Pipe()
	c.cmd.Stdout = io.MultiWriter(c.stdout, pw)
	c.cmd.Stderr = c.stderr
	if err := c.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { c.cmd.Wait(); pw.Close(); close(c.done) }()
	t.Cleanup(func() { c.cmd.Process.Kill(); <-c.done })
	sc := bufio.NewScanner(pr)
	if !sc.Scan() {
		t.Fatalf("no JSON line; stderr %q", c.stderr.String())
	}
	if err := json.Unmarshal(sc.Bytes(), &c.line); err != nil || c.line.State != wantState {
		t.Fatalf("line %q (err %v), want state %s; stderr %q", sc.Text(), err, wantState, c.stderr.String())
	}
	go io.Copy(io.Discard, pr)
	return c
}

func get(t *testing.T, url string, cookie bool) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set(auth.CSRFHeader, "1")
	if cookie {
		req.AddCookie(&http.Cookie{Name: embedCookie, Value: testEmbedToken})
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestEmbeddedRequiresTheCookieAndStopsWithStdin(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Refract")
	c := startEmbedded(t, dir, stateReady)
	if !loopbackURL(c.line.URL) || c.line.Version != version || c.line.PID != c.cmd.Process.Pid {
		t.Fatalf("line = %+v", c.line)
	}
	if n := strings.Count(strings.TrimSpace(c.stdout.String()), "\n"); n != 0 {
		t.Errorf("stdout has more than one line: %q", c.stdout.String())
	}
	for _, p := range []string{"/", "/api/projects", "/assets/x.js"} {
		if r := get(t, c.line.URL+p, false); r.StatusCode != 401 {
			t.Errorf("%s without cookie: %d", p, r.StatusCode)
		}
	}
	if r := get(t, c.line.URL+"/healthz", false); r.StatusCode != 200 {
		t.Errorf("/healthz: %d", r.StatusCode)
	}
	if r := get(t, c.line.URL+"/api/projects", true); r.StatusCode != 200 {
		t.Errorf("/api/projects with cookie: %d", r.StatusCode)
	}
	req, _ := http.NewRequest("GET", c.line.URL+"/", nil)
	req.AddCookie(&http.Cookie{Name: embedCookie, Value: testEmbedToken + "x"})
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 401 {
		t.Errorf("wrong token: %v %v", resp, err)
	}
	// The wrong Host is still refused by the host check.
	req, _ = http.NewRequest("GET", c.line.URL+"/", nil)
	req.Host = "evil.example"
	req.AddCookie(&http.Cookie{Name: embedCookie, Value: testEmbedToken})
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode == 200 {
		t.Errorf("foreign host: %v %v", resp, err)
	}

	c.stdin.Close() // the parent is gone
	select {
	case <-c.done:
	case <-time.After(30 * time.Second):
		t.Fatal("still running 30s after stdin closed")
	}
	if c.cmd.ProcessState.ExitCode() != 0 {
		t.Errorf("exit %d; stderr %q", c.cmd.ProcessState.ExitCode(), c.stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, localLockFile)); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, localLockFile)); len(b) != 0 {
		t.Errorf("lock file not cleared: %q", b)
	}
	for name, s := range map[string]string{"stdout": c.stdout.String(), "stderr": c.stderr.String()} {
		if strings.Contains(s, testEmbedToken) {
			t.Errorf("the token is in %s", name)
		}
	}
}

func TestEmbeddedSecondStartReportsTheRunningInstance(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Refract")
	first := startEmbedded(t, dir, stateReady)
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "--embedded")
	cmd.Env = append(localEnv(dir), embedTokenEnv+"="+testEmbedToken)
	var out bytes.Buffer
	cmd.Stdout = &out
	stdin, _ := cmd.StdinPipe() // open, as the app keeps it
	defer stdin.Close()
	_ = cmd.Run()
	var st instanceStatus
	if err := json.Unmarshal(out.Bytes(), &st); err != nil || st.State != stateRunning || st.URL != first.line.URL || st.PID != first.cmd.Process.Pid || !st.Embedded {
		t.Fatalf("second: %q (%v)", out.String(), err)
	}
	if cmd.ProcessState.ExitCode() != 0 {
		t.Errorf("exit %d", cmd.ProcessState.ExitCode())
	}
	// A terminal refract local doesn't send the browser to a page that needs the app's token.
	if out, code := runChild(t, dir, "--no-browser"); code != 0 || !strings.Contains(out, "desktop app") || strings.Contains(out, first.line.URL) {
		t.Errorf("CLI start: exit %d, %q", code, out)
	}
}

func TestEmbeddedFailuresPrintOneJSONLine(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Refract")
	for name, tc := range map[string]struct {
		args []string
		env  []string
		want string
	}{
		"no token":    {args: []string{"--embedded"}, want: "token"},
		"short token": {args: []string{"--embedded"}, env: []string{embedTokenEnv + "=abc"}, want: "too short"},
		"bad flag":    {args: []string{"--embedded", "--nope"}, env: []string{embedTokenEnv + "=" + testEmbedToken}, want: "nope"},
		"saas":        {args: []string{"--embedded"}, env: []string{embedTokenEnv + "=" + testEmbedToken, "REFRACT_MODE=saas"}, want: "hosted service"},
		"status":      {args: []string{"--status"}, env: []string{"REFRACT_MODE=bogus"}, want: "bogus"},
	} {
		t.Run(name, func(t *testing.T) {
			exe, _ := os.Executable()
			cmd := exec.Command(exe, tc.args...)
			cmd.Env = append(localEnv(dir), tc.env...)
			var out, errOut bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errOut
			cmd.Stdin = strings.NewReader("")
			err := cmd.Run()
			if err == nil {
				t.Fatal("expected a failure")
			}
			var j errorStatus
			if e := json.Unmarshal(out.Bytes(), &j); e != nil || j.State != stateError || !strings.Contains(j.Error, tc.want) || strings.Count(out.String(), "\n") != 1 {
				t.Fatalf("stdout %q (%v), want error containing %q", out.String(), e, tc.want)
			}
			if strings.Contains(out.String()+errOut.String(), testEmbedToken) {
				t.Error("the token is in the output")
			}
		})
	}
}

func TestReadEmbedTokenUnsetsTheEnvironment(t *testing.T) {
	t.Setenv(embedTokenEnv, testEmbedToken)
	tok, err := readEmbedToken(-1, os.Getenv, os.Unsetenv)
	if err != nil || tok != testEmbedToken {
		t.Fatal(tok, err)
	}
	if _, ok := os.LookupEnv(embedTokenEnv); ok {
		t.Fatal("variable still set")
	}
	// A child started now (tectonic, git) doesn't see it.
	out, _ := exec.Command("env").Output()
	if strings.Contains(string(out), testEmbedToken) {
		t.Fatal("child inherited the token")
	}
	if _, err := readEmbedToken(-1, os.Getenv, os.Unsetenv); err == nil {
		t.Fatal("no token should fail")
	}
	for _, bad := range []string{"short", strings.Repeat("a;", 10), strings.Repeat("a b", 10)} {
		t.Setenv(embedTokenEnv, bad)
		if _, err := readEmbedToken(-1, os.Getenv, os.Unsetenv); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestWatchStdinCancelsOnEOF(t *testing.T) {
	r, w := io.Pipe()
	ctx, cancel := watchStdin(context.Background(), r)
	defer cancel()
	select {
	case <-ctx.Done():
		t.Fatal("cancelled early")
	case <-time.After(50 * time.Millisecond):
	}
	w.Close()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("not cancelled")
	}
}

func TestEmbeddedStartReportsATerminalInstanceAsOpen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Refract")
	first := startChild(t, dir)
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "--embedded")
	cmd.Env = append(localEnv(dir), embedTokenEnv+"="+testEmbedToken)
	stdin, _ := cmd.StdinPipe()
	defer stdin.Close()
	out, _ := cmd.Output()
	var st instanceStatus
	if err := json.Unmarshal(out, &st); err != nil || st.State != stateRunning || st.URL != first.url || st.Embedded {
		t.Fatalf("embedded start: %q (%v)", out, err)
	}
}
