package main

import (
	"flag"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestMain lets the test binary stand in for px0: with FAKE_PX0 set it accepts
// px0's viewer flags and serves (or, for "exit", refuses to serve) app.js.
func TestMain(m *testing.M) {
	mode := os.Getenv("FAKE_PX0")
	if mode == "" {
		os.Exit(m.Run())
	}
	fs := flag.NewFlagSet("px0", flag.ExitOnError)
	host := fs.String("host", "", "")
	port := fs.Int("port", 0, "")
	fs.Bool("no-open", false, "")
	version := fs.Bool("version", false, "")
	fs.Parse(os.Args[1:])
	if *version {
		os.Stdout.WriteString("px0 test\n")
		os.Exit(0)
	}
	if mode == "exit" {
		os.Stderr.WriteString("fake px0 failed\n")
		os.Exit(3)
	}
	// Never outlive the test run, even if cleanup is skipped.
	time.AfterFunc(time.Minute, func() { os.Exit(0) })
	ln, err := net.Listen("tcp", net.JoinHostPort(*host, strconv.Itoa(*port)))
	if err != nil {
		os.Exit(4)
	}
	http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/quit" {
			os.Exit(0)
		}
		if r.URL.Path == "/static/app.js" {
			w.Write([]byte("console.log('px0')"))
			return
		}
		http.NotFound(w, r)
	}))
}

func fakePx0(t *testing.T, mode string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PX0_BINARY", exe)
	t.Setenv("FAKE_PX0", mode)
}

func TestLaunchViewerWaitsForFrontend(t *testing.T) {
	fakePx0(t, "serve")
	repo := t.TempDir()
	viewer, port, err := launchViewer(repo, filepath.Join(t.TempDir(), "viewer.log"), repositorySpec{InitialFile: "a/b.go", InitialLine: 7})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// The viewer is detached, so stop it over HTTP rather than by Wait.
		http.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/quit")
	})
	want := "http://127.0.0.1:" + strconv.Itoa(port) + "/?line=7&path=a%2Fb.go"
	if viewer != want {
		t.Fatalf("viewer = %q, want %q", viewer, want)
	}
	if !viewerHealthy(port) {
		t.Fatal("viewer not healthy after launch")
	}
}

func TestLaunchViewerReportsEarlyExit(t *testing.T) {
	fakePx0(t, "exit")
	log := filepath.Join(t.TempDir(), "viewer.log")
	_, _, err := launchViewer(t.TempDir(), log, repositorySpec{})
	if err == nil || !strings.Contains(err.Error(), "exited before serving") {
		t.Fatalf("err = %v, want early-exit error", err)
	}
	if b, _ := os.ReadFile(log); !strings.Contains(string(b), "fake px0 failed") {
		t.Fatalf("viewer log = %q", b)
	}
}

func TestResolvePx0FromEnv(t *testing.T) {
	t.Setenv("PX0_BINARY", filepath.Join(t.TempDir(), "missing"))
	if _, err := resolvePx0(); err == nil {
		t.Fatal("missing PX0_BINARY resolved")
	}
	fakePx0(t, "serve")
	bin, err := resolvePx0()
	if err != nil {
		t.Fatal(err)
	}
	if v, err := px0Version(bin); err != nil || v != "px0 test" {
		t.Fatalf("version = %q, %v", v, err)
	}
}
