package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// configName sits next to the host binary. The installer records the absolute
// px0 path there because Chrome starts native hosts with a minimal PATH (on
// macOS, launchd's /usr/bin:/bin:/usr/sbin:/sbin), which rarely includes px0.
const configName = "px0-extension-host.json"

type hostConfig struct {
	Px0 string `json:"px0"`
}

// resolvePx0 finds the px0 CLI: $PX0_BINARY, then the installer's config, then
// PATH, then common install locations.
func resolvePx0() (string, error) {
	if bin := os.Getenv("PX0_BINARY"); bin != "" {
		return checkExecutable(bin)
	}
	if exe, err := os.Executable(); err == nil {
		if b, err := os.ReadFile(filepath.Join(filepath.Dir(exe), configName)); err == nil {
			var cfg hostConfig
			if err := json.Unmarshal(b, &cfg); err != nil {
				return "", fmt.Errorf("read %s: %w", configName, err)
			}
			if cfg.Px0 != "" {
				return checkExecutable(cfg.Px0)
			}
		}
	}
	if bin, err := exec.LookPath("px0"); err == nil {
		return bin, nil
	}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{
		"/opt/homebrew/bin", "/usr/local/bin",
		filepath.Join(home, "go", "bin"), filepath.Join(home, ".local", "bin"),
	} {
		if bin, err := checkExecutable(filepath.Join(dir, "px0")); err == nil {
			return bin, nil
		}
	}
	return "", errors.New("px0 not found; re-run install-host.sh with the path to px0")
}

func checkExecutable(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("px0 not found at %s", path)
	}
	if st.IsDir() || st.Mode()&0o111 == 0 {
		return "", fmt.Errorf("px0 at %s is not executable", path)
	}
	return path, nil
}

func px0Version(bin string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "-version").Output()
	if err != nil {
		return "", fmt.Errorf("run %s -version: %w", bin, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// viewerHealthy reports whether a px0 server on port serves its frontend
// bundle. A listening port alone is not enough: a px0 built without web/app.js
// accepts connections but renders a blank page.
func viewerHealthy(port int) bool {
	client := &http.Client{Timeout: 250 * time.Millisecond}
	resp, err := client.Get("http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)) + "/static/app.js")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK && resp.ContentLength != 0
}

func viewerURL(port int, initialFile string, initialLine int) string {
	u := url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), Path: "/"}
	q := u.Query()
	if initialFile != "" {
		q.Set("path", initialFile)
	}
	if initialLine > 0 {
		q.Set("line", strconv.Itoa(initialLine))
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// launchViewer starts `px0 -host 127.0.0.1 -port N -no-open <repo>` detached
// from the host and waits until it serves the frontend. Output goes to logPath.
func launchViewer(repo, logPath string, spec repositorySpec) (string, int, error) {
	bin, err := resolvePx0()
	if err != nil {
		return "", 0, err
	}
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", 0, err
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	target := repo
	if spec.InitialFile != "" {
		target = filepath.Join(repo, filepath.FromSlash(spec.InitialFile))
		if spec.InitialLine > 0 {
			target += ":" + strconv.Itoa(spec.InitialLine)
		}
	}
	cmd := exec.Command(bin, "-host", "127.0.0.1", "-port", strconv.Itoa(port), "-no-open", target)
	detach(cmd)
	// Chrome may stop the native host when its extension service worker goes
	// idle. Never leave the viewer attached to the host's stdio pipes: a write
	// to a closed pipe would otherwise terminate an otherwise healthy viewer.
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return "", 0, err
	}
	defer devNull.Close()
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", 0, err
	}
	defer logFile.Close()
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, logFile, logFile
	if err := cmd.Start(); err != nil {
		return "", 0, err
	}
	// Reaping in a goroutine only lets us notice an early exit. It does not tie
	// the viewer's lifetime to the host: if the host exits first the viewer is
	// simply reparented.
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for {
		if viewerHealthy(port) {
			return viewerURL(port, spec.InitialFile, spec.InitialLine), port, nil
		}
		select {
		case err := <-exited:
			return "", 0, fmt.Errorf("px0 exited before serving (%v); see %s", err, logPath)
		case <-deadline.C:
			_ = cmd.Process.Kill()
			return "", 0, fmt.Errorf("timed out waiting for px0 viewer; see %s", logPath)
		case <-time.After(50 * time.Millisecond):
		}
	}
}
