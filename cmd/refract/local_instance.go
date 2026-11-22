package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// localLockFile is the file in the data folder that a running refract local
// holds locked. While it holds the lock it also records where it listens.
const localLockFile = "local.lock"

// instanceWait bounds how long a second start waits for the running instance
// to record its address and answer /healthz (it may be starting up).
var instanceWait = 8 * time.Second

// Instance states reported by `refract local --status`.
const (
	stateRunning      = "running"      // holds the lock and answers /healthz
	stateStopped      = "stopped"      // nobody holds the lock
	stateUnresponsive = "unresponsive" // holds the lock, no usable address or no answer
)

// instanceInfo is what the running instance writes into the lock file.
type instanceInfo struct {
	URL     string `json:"url"`
	PID     int    `json:"pid"`
	Version string `json:"version"`
}

// instanceStatus is the machine-readable answer of `refract local --status`,
// printed as one JSON line.
type instanceStatus struct {
	State   string `json:"state"`
	URL     string `json:"url,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Version string `json:"version,omitempty"`
	DataDir string `json:"dataDir"`
	Lock    string `json:"lockFile"`
}

// instanceLock is the exclusive lock on a data folder's lock file.
type instanceLock struct{ f *os.File }

// lockInstance takes the lock of dataDir. held is true when another process
// has it. The lock, not the file's existence, decides: a file left by a crash
// is simply locked again.
func lockInstance(dataDir string) (l *instanceLock, held bool, err error) {
	f, err := os.OpenFile(filepath.Join(dataDir, localLockFile), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("opening the lock file: %w", err)
	}
	if held, err = lockFile(f); held || err != nil {
		f.Close()
		if err != nil {
			err = fmt.Errorf("locking %s: %w", f.Name(), err)
		}
		return nil, held, err
	}
	return &instanceLock{f}, false, nil
}

// Record writes the address being served (replacing what a previous run left).
func (l *instanceLock) Record(u string) error {
	b, _ := json.Marshal(instanceInfo{URL: u, PID: os.Getpid(), Version: version})
	if err := l.f.Truncate(0); err != nil {
		return err
	}
	_, err := l.f.WriteAt(append(b, '\n'), 0)
	return err
}

// Release gives up the lock and clears the recorded address.
func (l *instanceLock) Release() {
	l.f.Truncate(0)
	l.f.Close() // closing drops the lock
}

func readInstanceInfo(dataDir string) (instanceInfo, error) {
	var info instanceInfo
	b, err := os.ReadFile(filepath.Join(dataDir, localLockFile))
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(b, &info)
	return info, err
}

// loopbackURL accepts only an http URL on 127.0.0.1 or ::1, so a doctored
// lock file can't make refract contact or open anything else.
func loopbackURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "http" || u.Port() == "" || u.Path != "" {
		return false
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && ip.IsLoopback()
}

// checkInstance describes the refract local using dataDir, given that its lock
// is held: it waits up to instanceWait for the recorded address to appear and
// answer /healthz. It contacts loopback addresses only.
func checkInstance(ctx context.Context, dataDir string) instanceStatus {
	st := instanceStatus{State: stateUnresponsive, DataDir: dataDir, Lock: filepath.Join(dataDir, localLockFile)}
	ctx, cancel := context.WithTimeout(ctx, instanceWait)
	defer cancel()
	for {
		info, err := readInstanceInfo(dataDir)
		if err == nil && loopbackURL(info.URL) {
			st.URL, st.PID, st.Version = info.URL, info.PID, info.Version
			if waitHealthy(ctx, info.URL+"/healthz") {
				st.State = stateRunning
				return st
			}
			return st
		}
		// No address yet (empty or half-written while the holder starts): retry.
		select {
		case <-ctx.Done():
			return st
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// recoveryHelp tells what to do when the lock is held but nothing answers.
func recoveryHelp(st instanceStatus) string {
	var b strings.Builder
	fmt.Fprintf(&b, "another refract local holds the lock on the data folder %s", st.DataDir)
	if st.URL != "" {
		fmt.Fprintf(&b, " but %s doesn't answer", st.URL)
	} else {
		b.WriteString(" but has not recorded an address")
	}
	b.WriteString(".\nIt may still be starting, or be stuck. Wait a moment and try again; if it stays like this, stop that process")
	if st.PID > 0 {
		fmt.Fprintf(&b, " (pid %d)", st.PID)
	} else {
		b.WriteString(" (see its pid in " + st.Lock + ")")
	}
	b.WriteString(" and start again. Nothing was deleted: the lock is released when the process ends, and " + localLockFile + " is safe to leave in place")
	return b.String()
}

// errExit is an error that ends the program with a given exit code. An empty
// message prints nothing.
type errExit struct {
	code int
	msg  string
}

func (e errExit) Error() string { return e.msg }

// localExitCode reports err the way refract local does and returns the exit code.
func localExitCode(err error) int {
	var e errExit
	switch {
	case err == nil:
		return 0
	case errors.As(err, &e):
		if e.msg != "" {
			fmt.Fprintln(os.Stderr, "refract local:", e.msg)
		}
		return e.code
	}
	fmt.Fprintln(os.Stderr, "refract local:", err)
	return 1
}
