// Package herdrtest starts throwaway headless Herdr servers for tests.
//
// Each server gets its own XDG config, state, data and cache dirs, so it
// never touches the user's sessions, config, installed plugins or plugin
// state. Tests skip when the herdr binary is
// not on PATH or when run with -short.
package herdrtest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	herdr "github.com/nerdwave-nick/herdr-go"
)

// Server starts a headless Herdr server and returns a client for it. The
// server is stopped when the test ends.
func Server(t testing.TB) *herdr.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("herdrtest: skipping live Herdr server in -short mode")
	}
	bin, err := exec.LookPath("herdr")
	if err != nil {
		t.Skip("herdrtest: herdr binary not found on PATH")
	}
	// Unix socket paths are limited to ~108 bytes, so use a short base dir.
	dir, err := os.MkdirTemp(shortTempBase(), "hg")
	if err != nil {
		t.Fatalf("herdrtest: temp dir: %v", err)
	}
	cmd := exec.Command(bin, "server")
	cmd.Env = append(cleanEnv(),
		"XDG_CONFIG_HOME="+dir,
		"XDG_STATE_HOME="+filepath.Join(dir, "state"),
		"XDG_DATA_HOME="+filepath.Join(dir, "data"),
		"XDG_CACHE_HOME="+filepath.Join(dir, "cache"),
	)
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		t.Fatalf("herdrtest: start server: %v", err)
	}
	sock := filepath.Join(dir, "herdr", "herdr.sock")
	c := herdr.New(sock)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.Call(ctx, "server.stop", struct{}{}, nil)
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		os.RemoveAll(dir)
	})

	deadline := time.Now().Add(10 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err := c.Ping(ctx)
		cancel()
		if err == nil {
			return c
		}
		if time.Now().After(deadline) {
			t.Fatalf("herdrtest: server did not come up: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func shortTempBase() string {
	if base := os.TempDir(); len(base) < 40 {
		return base
	}
	return "/tmp"
}

// cleanEnv drops the caller's Herdr context so the child server does not
// think it is nested inside another Herdr pane.
func cleanEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "HERDR_") || strings.HasPrefix(kv, "XDG_CONFIG_HOME=") ||
			strings.HasPrefix(kv, "XDG_STATE_HOME=") || strings.HasPrefix(kv, "XDG_DATA_HOME=") ||
			strings.HasPrefix(kv, "XDG_CACHE_HOME=") {
			continue
		}
		env = append(env, kv)
	}
	return env
}
