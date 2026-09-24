package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	nativeProtocolVersion = 1
	nativeMessageMax      = 4 << 20
	repositoryCacheLimit  = 10
	// Repositories cloned ahead of time because a link was hovered, and not
	// opened since, get their own smaller budget so hovering can never evict
	// a repository the user actually opened.
	prefetchCacheLimit = 5
	// Hover prefetches share bandwidth with the clone for the page being read.
	prefetchConcurrency = 2
	prefetchMarker      = "prefetched"
)

type nativeRequest struct {
	Version int    `json:"version"`
	ID      int64  `json:"id"`
	Action  string `json:"action"`
	URL     string `json:"url"`
}

type nativeResponse struct {
	Version   int    `json:"version"`
	ID        int64  `json:"id"`
	OK        bool   `json:"ok"`
	State     string `json:"state,omitempty"`
	ViewerURL string `json:"viewerUrl,omitempty"`
	Error     string `json:"error,omitempty"`

	// Set only for "ping", which the setup page uses to confirm the install.
	HostVersion string `json:"hostVersion,omitempty"`
	Px0         string `json:"px0,omitempty"`
	Px0Version  string `json:"px0Version,omitempty"`
}

type repositorySpec struct {
	PageURL     string
	CloneURL    string
	Key         string
	Ref         string
	InitialFile string
	InitialLine int
}

type repositoryJob struct {
	done chan struct{}
	err  error

	// prefetch is true while only hover prefetches have asked for this clone.
	// Guarded by repositoryCache.mu.
	prefetch bool
}

type repositoryCache struct {
	root string

	mu   sync.Mutex
	jobs map[string]*repositoryJob
}

func newRepositoryCache() (*repositoryCache, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("find user cache directory: %w", err)
	}
	return &repositoryCache{
		root: filepath.Join(root, "px0", "repositories"),
		jobs: make(map[string]*repositoryJob),
	}, nil
}

// normalizeRepositoryURL accepts only forge pages the extension is allowed to
// send. Credentials, ports, lookalike hosts, and non-HTTPS schemes are refused
// again here because content-script messages are untrusted input.
func normalizeRepositoryURL(raw string) (repositorySpec, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return repositorySpec{}, errors.New("unsupported repository URL")
	}
	parts := strings.FieldsFunc(u.EscapedPath(), func(r rune) bool { return r == '/' })
	for i := range parts {
		part, err := url.PathUnescape(parts[i])
		if err != nil || part == "" || part == "." || part == ".." || strings.ContainsAny(part, "/\\\x00") {
			return repositorySpec{}, errors.New("invalid repository path")
		}
		parts[i] = part
	}

	var repoParts, tail []string
	switch strings.ToLower(u.Hostname()) {
	case "github.com":
		if len(parts) < 2 || reservedForgeRoot("github.com", parts[0]) {
			return repositorySpec{}, errors.New("GitHub URL does not name a repository")
		}
		repoParts, tail = parts[:2], parts[2:]
	case "gitlab.com":
		marker := -1
		for i, part := range parts {
			if part == "-" {
				marker = i
				break
			}
		}
		if marker < 0 {
			marker = len(parts)
		}
		if marker < 2 || reservedForgeRoot("gitlab.com", parts[0]) {
			return repositorySpec{}, errors.New("GitLab URL does not name a repository")
		}
		repoParts = parts[:marker]
		if marker < len(parts) {
			tail = parts[marker+1:]
		}
	default:
		return repositorySpec{}, errors.New("unsupported repository host")
	}
	for _, part := range repoParts {
		if !validForgeSegment(part) {
			return repositorySpec{}, errors.New("invalid repository path")
		}
	}

	repoParts[len(repoParts)-1] = strings.TrimSuffix(repoParts[len(repoParts)-1], ".git")
	if repoParts[len(repoParts)-1] == "" {
		return repositorySpec{}, errors.New("invalid repository name")
	}
	cloneURL := "https://" + strings.ToLower(u.Hostname()) + "/" + strings.Join(repoParts, "/") + ".git"
	spec := repositorySpec{
		PageURL:  u.String(),
		CloneURL: cloneURL,
	}

	// A forge URL cannot unambiguously split branch names containing slashes.
	// For now preserve paths for the common single-segment branch case. Opening
	// the repository still succeeds when this hint cannot be derived.
	if len(tail) >= 2 && (tail[0] == "blob" || tail[0] == "tree") {
		spec.Ref = tail[1]
		if len(tail) >= 3 && tail[0] == "blob" {
			spec.InitialFile = strings.Join(tail[2:], "/")
		}
	}
	if fragment := strings.TrimPrefix(u.Fragment, "L"); fragment != u.Fragment {
		spec.InitialLine, _ = strconv.Atoi(strings.Split(fragment, "-")[0])
	}
	spec.Key = repositoryKey(cloneURL, spec.Ref)
	return spec, nil
}

func repositoryKey(cloneURL, ref string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(cloneURL) + "\n" + ref))
	return hex.EncodeToString(sum[:12])
}

func validForgeSegment(segment string) bool {
	if segment == "" {
		return false
	}
	for _, r := range segment {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func reservedForgeRoot(host, root string) bool {
	root = strings.ToLower(root)
	var reserved map[string]bool
	if host == "github.com" {
		reserved = map[string]bool{
			"collections": true, "codespaces": true, "enterprise": true, "events": true,
			"explore": true, "features": true, "issues": true, "login": true,
			"marketplace": true, "new": true, "notifications": true, "orgs": true,
			"pricing": true, "pulls": true, "search": true, "settings": true,
			"signup": true, "sponsors": true, "topics": true, "trending": true,
		}
	} else {
		reserved = map[string]bool{
			"admin": true, "dashboard": true, "explore": true, "groups": true,
			"help": true, "projects": true, "search": true, "users": true,
		}
	}
	return reserved[root]
}

func (c *repositoryCache) entryPath(spec repositorySpec) string {
	return filepath.Join(c.root, spec.Key)
}

func (c *repositoryCache) repoPath(spec repositorySpec) string {
	return filepath.Join(c.entryPath(spec), "repo")
}

// reuseDefaultBranch maps a branch URL onto the default-branch checkout when
// that checkout is on the same branch. Landing on github.com/o/r and then
// opening github.com/o/r/blob/main/... would otherwise clone main twice.
func (c *repositoryCache) reuseDefaultBranch(spec repositorySpec) repositorySpec {
	if spec.Ref == "" || c.ready(spec) {
		return spec
	}
	base := spec
	base.Ref = ""
	base.Key = repositoryKey(spec.CloneURL, "")
	if !c.ready(base) {
		return spec
	}
	out, err := exec.Command("git", "--no-optional-locks", "-C", c.repoPath(base), "symbolic-ref", "--short", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(out)) != spec.Ref {
		return spec
	}
	return base
}

func (c *repositoryCache) ready(spec repositorySpec) bool {
	st, err := os.Stat(filepath.Join(c.repoPath(spec), ".git"))
	return err == nil && st.IsDir()
}

// warm starts cloning spec unless it is cached or already being cloned. A
// prefetch (hover) does not count as use: it neither refreshes a cached entry's
// LRU position nor promotes a prefetched entry, and it is skipped when enough
// prefetches are already running.
func (c *repositoryCache) warm(spec repositorySpec, prefetch bool) (*repositoryJob, string, error) {
	if err := os.MkdirAll(c.root, 0o700); err != nil {
		return nil, "", err
	}
	entry := c.entryPath(spec)
	if c.ready(spec) {
		if !prefetch {
			// The clone may have just landed with its job still finishing;
			// make sure that job does not mark this opened entry afterwards.
			c.mu.Lock()
			if job := c.jobs[spec.Key]; job != nil {
				job.prefetch = false
			}
			c.mu.Unlock()
			_ = os.Remove(filepath.Join(entry, prefetchMarker))
			_ = os.Chtimes(entry, time.Now(), time.Now())
		}
		return nil, "ready", nil
	}

	c.mu.Lock()
	if job := c.jobs[spec.Key]; job != nil {
		if !prefetch {
			job.prefetch = false
		}
		c.mu.Unlock()
		return job, "cloning", nil
	}
	if prefetch && c.runningPrefetches() >= prefetchConcurrency {
		c.mu.Unlock()
		return nil, "skipped", nil
	}
	job := &repositoryJob{done: make(chan struct{}), prefetch: prefetch}
	c.jobs[spec.Key] = job
	c.mu.Unlock()

	go func() {
		err := c.clone(spec)
		c.mu.Lock()
		// Decide under the lock so an open that joins at the last moment
		// still promotes the entry.
		if err == nil && job.prefetch {
			err = os.WriteFile(filepath.Join(entry, prefetchMarker), nil, 0o600)
		}
		job.err = err
		delete(c.jobs, spec.Key)
		c.mu.Unlock()
		close(job.done)
		if err == nil {
			c.evict(spec.Key)
		}
	}()
	return job, "cloning", nil
}

// runningPrefetches must be called with c.mu held.
func (c *repositoryCache) runningPrefetches() int {
	n := 0
	for _, job := range c.jobs {
		if job.prefetch {
			n++
		}
	}
	return n
}

func (c *repositoryCache) clone(spec repositorySpec) error {
	tmp, err := os.MkdirTemp(c.root, ".clone-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	repo := filepath.Join(tmp, "repo")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	args := repositoryCloneArgs(spec, repo)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		// LFS smudging can turn a small source checkout into a multi-gigabyte
		// warm-up. Pointer files are sufficient for code navigation.
		"GIT_LFS_SKIP_SMUDGE=1",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("clone %s: %w: %s", spec.CloneURL, err, strings.TrimSpace(string(out)))
	}
	entry := c.entryPath(spec)
	if c.ready(spec) {
		return nil
	}
	if err := os.RemoveAll(entry); err != nil {
		return err
	}
	return os.Rename(tmp, entry)
}

func repositoryCloneArgs(spec repositorySpec, destination string) []string {
	// px0 needs the complete current worktree for indexing and search, but it
	// does not need old commits or tags merely to open a forge page. No
	// --filter=blob:none: the checkout needs every blob anyway, and fetching
	// them in a second round trip made clones about twice as slow.
	//
	// Most of the remaining time is the single packfile download, which git
	// cannot split. Writing the files can run in parallel (git 2.32+; older
	// versions ignore these settings), which saves 1-2 s on large repositories.
	args := []string{
		"-c", "checkout.workers=0", "-c", "checkout.thresholdForParallelism=100",
		"clone", "--depth=1", "--single-branch", "--no-tags",
	}
	if spec.Ref != "" {
		args = append(args, "--branch", spec.Ref)
	}
	return append(args, spec.CloneURL, destination)
}

func (c *repositoryCache) wait(ctx context.Context, spec repositorySpec) (string, error) {
	job, _, err := c.warm(spec, false)
	if err != nil {
		return "", err
	}
	if job != nil {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-job.done:
			if job.err != nil {
				return "", job.err
			}
		}
	}
	return c.repoPath(spec), nil
}

// evict keeps the cache bounded by entry count, least recently used first.
// Opened and hover-prefetched repositories are budgeted separately, so
// prefetches only ever displace other prefetches. Active viewers are detected
// by their recorded localhost port; dirty repositories are retained so an
// agent's edits can never be discarded by cache maintenance.
func (c *repositoryCache) evict(keep string) {
	entries, err := os.ReadDir(c.root)
	if err != nil {
		return
	}
	type candidate struct {
		path string
		mod  time.Time
	}
	var opened, prefetched []candidate
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".clone-") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		item := candidate{path: filepath.Join(c.root, entry.Name()), mod: info.ModTime()}
		if entry.Name() == keep {
			// Counted toward its bucket but never removed.
			item.mod = time.Now().Add(time.Hour)
		}
		if _, err := os.Stat(filepath.Join(item.path, prefetchMarker)); err == nil {
			prefetched = append(prefetched, item)
		} else {
			opened = append(opened, item)
		}
	}
	trim := func(bucket []candidate, limit int) {
		sort.Slice(bucket, func(i, j int) bool { return bucket[i].mod.Before(bucket[j].mod) })
		remove := len(bucket) - limit
		for _, item := range bucket {
			if remove <= 0 {
				return
			}
			if filepath.Base(item.path) == keep || cacheEntryActive(item.path) {
				continue
			}
			repo := filepath.Join(item.path, "repo")
			if out, err := exec.Command("git", "--no-optional-locks", "-C", repo, "status", "--porcelain").Output(); err != nil || len(out) != 0 {
				continue
			}
			if os.RemoveAll(item.path) == nil {
				remove--
			}
		}
	}
	trim(opened, repositoryCacheLimit)
	trim(prefetched, prefetchCacheLimit)
}

func cacheEntryViewerPort(entry string) (int, bool) {
	b, err := os.ReadFile(filepath.Join(entry, "active-port"))
	if err != nil {
		return 0, false
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err == nil && port > 0 && port < 65536 && viewerHealthy(port) {
		return port, true
	}
	_ = os.Remove(filepath.Join(entry, "active-port"))
	return 0, false
}

func cacheEntryActive(entry string) bool {
	_, active := cacheEntryViewerPort(entry)
	return active
}

func runNativeHost(in io.Reader, out io.Writer) error {
	cache, err := newRepositoryCache()
	if err != nil {
		return err
	}
	for {
		var req nativeRequest
		if err := readNativeMessage(in, &req); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		resp := handleNativeRequest(cache, req)
		if err := writeNativeMessage(out, resp); err != nil {
			return err
		}
	}
}

func handleNativeRequest(cache *repositoryCache, req nativeRequest) nativeResponse {
	resp := nativeResponse{Version: nativeProtocolVersion, ID: req.ID}
	if req.Version != nativeProtocolVersion {
		resp.Error = "unsupported protocol version"
		return resp
	}
	if req.Action == "ping" {
		return ping(resp)
	}
	spec, err := normalizeRepositoryURL(req.URL)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	spec = cache.reuseDefaultBranch(spec)
	switch req.Action {
	case "warm", "prefetch":
		_, state, err := cache.warm(spec, req.Action == "prefetch")
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		resp.OK, resp.State = true, state
	case "status":
		resp.OK = true
		if cache.ready(spec) {
			resp.State = "ready"
		} else {
			cache.mu.Lock()
			job := cache.jobs[spec.Key]
			cache.mu.Unlock()
			if job != nil {
				resp.State = "cloning"
			} else {
				resp.State = "missing"
			}
		}
	case "open":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		repo, err := cache.wait(ctx, spec)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		entry := cache.entryPath(spec)
		if port, active := cacheEntryViewerPort(entry); active {
			resp.OK, resp.State = true, "open"
			resp.ViewerURL = viewerURL(port, spec.InitialFile, spec.InitialLine)
			return resp
		}
		viewer, port, err := launchViewer(repo, filepath.Join(entry, "viewer.log"), spec)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		active := filepath.Join(entry, "active-port")
		_ = os.WriteFile(active, []byte(strconv.Itoa(port)), 0o600)
		resp.OK, resp.State, resp.ViewerURL = true, "open", viewer
	default:
		resp.Error = "unsupported action"
	}
	return resp
}

func readNativeMessage(r io.Reader, v any) error {
	var size uint32
	if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
		return err
	}
	if size == 0 || size > nativeMessageMax {
		return fmt.Errorf("invalid native message size %d", size)
	}
	b := make([]byte, size)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeNativeMessage(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > 1<<20 {
		return errors.New("native response exceeds Chrome's 1 MiB limit")
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(b))); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// ping reports the host version and whether px0 can be launched. The host
// itself answering proves the native-messaging registration works.
func ping(resp nativeResponse) nativeResponse {
	resp.HostVersion = version
	bin, err := resolvePx0()
	if err == nil {
		resp.Px0 = bin
		resp.Px0Version, err = px0Version(bin)
	}
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	resp.OK = true
	return resp
}
