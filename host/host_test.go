package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func TestNormalizeRepositoryURL(t *testing.T) {
	tests := []struct {
		url, clone, file string
		line             int
	}{
		{"https://github.com/px0-ai/px0", "https://github.com/px0-ai/px0.git", "", 0},
		{"https://github.com/px0-ai/px0/blob/master/server.go#L42", "https://github.com/px0-ai/px0.git", "server.go", 42},
		{"https://gitlab.com/group/sub/project/-/blob/main/src/app.go#L7", "https://gitlab.com/group/sub/project.git", "src/app.go", 7},
	}
	branch, err := normalizeRepositoryURL("https://github.com/px0-ai/px0/tree/feature")
	if err != nil || branch.Ref != "feature" {
		t.Fatalf("branch ref = %q, err = %v", branch.Ref, err)
	}
	for _, tt := range tests {
		spec, err := normalizeRepositoryURL(tt.url)
		if err != nil {
			t.Fatalf("normalizeRepositoryURL(%q): %v", tt.url, err)
		}
		if spec.CloneURL != tt.clone || spec.InitialFile != tt.file || spec.InitialLine != tt.line {
			t.Errorf("normalizeRepositoryURL(%q) = %+v", tt.url, spec)
		}
	}
	for _, raw := range []string{
		"http://github.com/o/r", "https://github.com.evil.test/o/r",
		"https://user:pass@github.com/o/r", "https://gitlab.com/only-one",
		"https://github.com/settings/profile", "https://gitlab.com/dashboard/projects",
		"https://github.com/o/r%3Fevil", "https://github.com/o%2Fevil/r",
	} {
		if _, err := normalizeRepositoryURL(raw); err == nil {
			t.Errorf("normalizeRepositoryURL(%q) succeeded", raw)
		}
	}
}

func TestNativeMessageRoundTrip(t *testing.T) {
	want := nativeRequest{Version: 1, ID: 9, Action: "warm", URL: "https://github.com/o/r"}
	var framed bytes.Buffer
	if err := writeNativeMessage(&framed, want); err != nil {
		t.Fatal(err)
	}
	var got nativeRequest
	if err := readNativeMessage(&framed, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestNativeStatusDoesNotStartClone(t *testing.T) {
	cache := &repositoryCache{root: t.TempDir(), jobs: make(map[string]*repositoryJob)}
	resp := handleNativeRequest(cache, nativeRequest{
		Version: 1, ID: 12, Action: "status", URL: "https://github.com/px0-ai/px0",
	})
	if !resp.OK || resp.ID != 12 || resp.State != "missing" {
		t.Fatalf("status response = %+v", resp)
	}
	if len(cache.jobs) != 0 {
		t.Fatalf("status started %d clone jobs", len(cache.jobs))
	}
}

func TestRepositoryCloneIsShallowAndSingleBranch(t *testing.T) {
	spec := repositorySpec{CloneURL: "https://github.com/o/r.git", Ref: "feature"}
	got := repositoryCloneArgs(spec, "/cache/repo")
	want := []string{
		"-c", "checkout.workers=0", "-c", "checkout.thresholdForParallelism=100",
		"clone", "--depth=1", "--single-branch", "--no-tags",
		"--branch", "feature", "https://github.com/o/r.git", "/cache/repo",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("clone args = %q, want %q", got, want)
	}
}

func TestCacheEntryViewerRequiresFrontendBundle(t *testing.T) {
	entry := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/static/app.js" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("console.log('px0')"))
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	_, rawPort, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(rawPort)
	marker := filepath.Join(entry, "active-port")
	if err := os.WriteFile(marker, []byte(rawPort), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := cacheEntryViewerPort(entry); !ok || got != port {
		t.Fatalf("active viewer = (%d, %v), want (%d, true)", got, ok, port)
	}

	server.Close()
	if _, ok := cacheEntryViewerPort(entry); ok {
		t.Fatal("stopped viewer still reported active")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("stale marker was not removed: %v", err)
	}
}

func TestNativePingReportsPx0(t *testing.T) {
	cache := &repositoryCache{root: t.TempDir(), jobs: make(map[string]*repositoryJob)}
	fakePx0(t, "serve")
	resp := handleNativeRequest(cache, nativeRequest{Version: 1, ID: 3, Action: "ping"})
	if !resp.OK || resp.ID != 3 || resp.HostVersion != version || resp.Px0Version != "px0 test" {
		t.Fatalf("ping = %+v", resp)
	}
	t.Setenv("PX0_BINARY", filepath.Join(t.TempDir(), "missing"))
	resp = handleNativeRequest(cache, nativeRequest{Version: 1, ID: 4, Action: "ping"})
	if resp.OK || resp.HostVersion == "" || resp.Error == "" {
		t.Fatalf("ping without px0 = %+v", resp)
	}
}

func TestBranchURLReusesDefaultBranchCheckout(t *testing.T) {
	cache := &repositoryCache{root: t.TempDir(), jobs: make(map[string]*repositoryJob)}
	root, err := normalizeRepositoryURL("https://github.com/o/r")
	if err != nil {
		t.Fatal(err)
	}
	repo := cache.repoPath(root)
	if out, err := exec.Command("git", "init", "-q", "-b", "main", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}

	onMain, _ := normalizeRepositoryURL("https://github.com/o/r/blob/main/a.go#L3")
	got := cache.reuseDefaultBranch(onMain)
	if got.Key != root.Key || got.InitialFile != "a.go" || got.InitialLine != 3 {
		t.Fatalf("main URL = %+v, want default checkout %s with file kept", got, root.Key)
	}
	onOther, _ := normalizeRepositoryURL("https://github.com/o/r/tree/feature")
	if got := cache.reuseDefaultBranch(onOther); got.Key != onOther.Key {
		t.Fatalf("feature URL reused the main checkout")
	}
}

// fakeEntry creates a clean cached repository with the given age.
func fakeEntry(t *testing.T, root, name string, age time.Duration, prefetched bool) {
	t.Helper()
	entry := filepath.Join(root, name)
	if out, err := exec.Command("git", "init", "-q", filepath.Join(entry, "repo")).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if prefetched {
		if err := os.WriteFile(filepath.Join(entry, prefetchMarker), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(entry, when, when); err != nil {
		t.Fatal(err)
	}
}

func TestEvictBudgetsPrefetchesSeparately(t *testing.T) {
	root := t.TempDir()
	cache := &repositoryCache{root: root, jobs: make(map[string]*repositoryJob)}
	// 12 opened repositories; o00 is the most recently used.
	for i := 0; i < 12; i++ {
		fakeEntry(t, root, fmt.Sprintf("o%02d", i), time.Duration(i+1)*time.Hour, false)
	}
	// 7 hover prefetches, all newer than every opened repository.
	for i := 0; i < 7; i++ {
		fakeEntry(t, root, fmt.Sprintf("p%02d", i), time.Duration(i+1)*time.Minute, true)
	}
	cache.evict("")

	for i := 0; i < 12; i++ {
		_, err := os.Stat(filepath.Join(root, fmt.Sprintf("o%02d", i)))
		if kept := err == nil; kept != (i < repositoryCacheLimit) {
			t.Errorf("opened o%02d kept = %v", i, kept)
		}
	}
	for i := 0; i < 7; i++ {
		_, err := os.Stat(filepath.Join(root, fmt.Sprintf("p%02d", i)))
		if kept := err == nil; kept != (i < prefetchCacheLimit) {
			t.Errorf("prefetched p%02d kept = %v", i, kept)
		}
	}
}

func TestPrefetchIsNotUse(t *testing.T) {
	root := t.TempDir()
	cache := &repositoryCache{root: root, jobs: make(map[string]*repositoryJob)}
	spec, _ := normalizeRepositoryURL("https://github.com/o/r")
	fakeEntry(t, root, spec.Key, time.Hour, true)
	marker := filepath.Join(root, spec.Key, prefetchMarker)
	before, _ := os.Stat(filepath.Join(root, spec.Key))

	if _, state, err := cache.warm(spec, true); err != nil || state != "ready" {
		t.Fatalf("prefetch = %q, %v", state, err)
	}
	after, _ := os.Stat(filepath.Join(root, spec.Key))
	if _, err := os.Stat(marker); err != nil || !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("prefetch of a cached repository promoted it")
	}

	if _, _, err := cache.warm(spec, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("opening a prefetched repository did not promote it")
	}
}

func TestPrefetchConcurrencyIsCapped(t *testing.T) {
	cache := &repositoryCache{root: t.TempDir(), jobs: make(map[string]*repositoryJob)}
	for i := 0; i < prefetchConcurrency; i++ {
		cache.jobs[fmt.Sprint("busy", i)] = &repositoryJob{done: make(chan struct{}), prefetch: true}
	}
	spec, _ := normalizeRepositoryURL("https://github.com/o/r")
	job, state, err := cache.warm(spec, true)
	if err != nil || job != nil || state != "skipped" {
		t.Fatalf("prefetch over the cap = (%v, %q, %v), want skipped", job, state, err)
	}
	if len(cache.jobs) != prefetchConcurrency {
		t.Fatalf("prefetch over the cap started a clone")
	}
}
