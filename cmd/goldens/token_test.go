/*
 * Copyright (c) 2026 LoxiLB Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at:
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package goldens

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// authRecorder is a gateway that records every Authorization header it
// sees, so a test can prove which token actually went on the wire — and,
// for the refusal rows, that nothing went on the wire at all.
type authRecorder struct {
	mu     sync.Mutex
	seen   []string
	server *httptest.Server
}

func newAuthRecorder(t *testing.T) *authRecorder {
	t.Helper()
	rec := &authRecorder{}
	rec.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.seen = append(rec.seen, r.Header.Get("Authorization"))
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(successBody))
	}))
	t.Cleanup(rec.server.Close)
	return rec
}

func (rec *authRecorder) hostPort(t *testing.T) (string, string) {
	t.Helper()
	u, err := url.Parse(rec.server.URL)
	if err != nil {
		t.Fatalf("parse recorder URL: %v", err)
	}
	return u.Hostname(), u.Port()
}

func (rec *authRecorder) headers() []string {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return append([]string(nil), rec.seen...)
}

// buildCLIWithSessionPath builds the packaged binary with the session
// token path relocated into the test's own directory — the ldflags-only
// relocation mechanism the backend adapter also uses — so the suite never
// touches a real operator's /tmp/loxilbtoken.
func buildCLIWithSessionPath(t *testing.T, sessionPath string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping packaged-binary test in short mode")
	}
	if runtime.GOOS != "linux" {
		t.Skip("loxicmd links Linux netlink; the packaged binary only builds there")
	}
	binary := filepath.Join(t.TempDir(), "loxicmd")
	ldflags := "-X github.com/loxilb-io/loxicmd-inference-gateway/pkg/api.SessionTokenPath=" + sessionPath
	cmd := exec.Command("go", "build", "-ldflags", ldflags, "-o", binary, ".")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the CLI failed: %v\n%s", err, out)
	}
	return binary
}

// TestSessionTokenFallback pins the implicit session path: the token a
// previous "set login" left on disk authenticates later invocations, but
// only after passing the same secret-file rules as an explicit
// --token-file — the file lives in a world-writable directory and used to
// be read blindly. Explicit flags always win over the session file.
func TestSessionTokenFallback(t *testing.T) {
	dir := t.TempDir()
	sessionPath := filepath.Join(dir, "session")
	binary := buildCLIWithSessionPath(t, sessionPath)

	fileTokenPath := filepath.Join(dir, "explicit")
	if err := os.WriteFile(fileTokenPath, []byte("filetoken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(dir, "target")
	if err := os.WriteFile(targetPath, []byte("sessiontoken\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	setSession := func(t *testing.T, content string, mode os.FileMode) {
		t.Helper()
		if err := os.Remove(sessionPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err := os.WriteFile(sessionPath, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	clearSession := func(t *testing.T) {
		t.Helper()
		if err := os.Remove(sessionPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}

	run := func(t *testing.T, rec *authRecorder, args ...string) (int, string, string) {
		t.Helper()
		host, port := rec.hostPort(t)
		full := append([]string{"-s", host, "-p", port}, args...)
		cmd := exec.Command(binary, full...)
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		exit := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			exit = exitErr.ExitCode()
		} else if err != nil {
			t.Fatalf("running the CLI failed: %v", err)
		}
		if exit == 1 {
			t.Errorf("the reserved legacy exit 1 was emitted")
		}
		return exit, stdout.String(), stderr.String()
	}

	t.Run("valid-session-authenticates", func(t *testing.T) {
		setSession(t, "sessiontoken\n", 0o600)
		rec := newAuthRecorder(t)
		exit, _, stderr := run(t, rec, "delete", "vlan", "100")
		if exit != 0 {
			t.Fatalf("exit %d, want 0\nstderr:\n%s", exit, stderr)
		}
		if headers := rec.headers(); len(headers) == 0 || headers[0] != "Bearer sessiontoken" {
			t.Errorf("Authorization headers %q, want the session token as a Bearer credential", headers)
		}
		if strings.Contains(stderr, "Warning:") {
			t.Errorf("the session path must not warn:\n%s", stderr)
		}
	})

	t.Run("absent-session-is-unauthenticated", func(t *testing.T) {
		clearSession(t)
		rec := newAuthRecorder(t)
		exit, _, stderr := run(t, rec, "delete", "vlan", "100")
		if exit != 0 {
			t.Fatalf("exit %d, want 0\nstderr:\n%s", exit, stderr)
		}
		if headers := rec.headers(); len(headers) == 0 || headers[0] != "" {
			t.Errorf("Authorization headers %q, want one request with no credential", headers)
		}
	})

	t.Run("explicit-token-file-wins", func(t *testing.T) {
		setSession(t, "sessiontoken\n", 0o600)
		rec := newAuthRecorder(t)
		exit, _, stderr := run(t, rec, "--token-file", fileTokenPath, "delete", "vlan", "100")
		if exit != 0 {
			t.Fatalf("exit %d, want 0\nstderr:\n%s", exit, stderr)
		}
		if headers := rec.headers(); len(headers) == 0 || headers[0] != "Bearer filetoken" {
			t.Errorf("Authorization headers %q, want the explicit file's token over the session token", headers)
		}
	})

	t.Run("explicit-token-flag-wins", func(t *testing.T) {
		setSession(t, "sessiontoken\n", 0o600)
		rec := newAuthRecorder(t)
		exit, _, stderr := run(t, rec, "--token", "flagtoken", "delete", "vlan", "100")
		if exit != 0 {
			t.Fatalf("exit %d, want 0\nstderr:\n%s", exit, stderr)
		}
		if headers := rec.headers(); len(headers) == 0 || headers[0] != "Bearer flagtoken" {
			t.Errorf("Authorization headers %q, want the explicit flag's token over the session token", headers)
		}
		if got := strings.Count(stderr, "Warning:"); got != 1 {
			t.Errorf("deprecation warning on stderr %d times, want exactly once:\n%s", got, stderr)
		}
	})

	sessionRefusals := []struct {
		name       string
		setup      func(t *testing.T)
		wantExit   int
		wantStderr string
	}{
		{"symlink-session-is-refused", func(t *testing.T) {
			clearSession(t)
			if err := os.Symlink(targetPath, sessionPath); err != nil {
				t.Fatal(err)
			}
		}, 4, "symlink"},
		{"open-session-is-refused", func(t *testing.T) {
			setSession(t, "sessiontoken\n", 0o644)
		}, 4, "owner-only"},
		{"empty-session-is-refused", func(t *testing.T) {
			setSession(t, " \n", 0o600)
		}, 4, "empty"},
	}
	for _, tc := range sessionRefusals {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			rec := newAuthRecorder(t)
			exit, stdout, stderr := run(t, rec, "delete", "vlan", "100")
			if exit != tc.wantExit {
				t.Errorf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", exit, tc.wantExit, stdout, stderr)
			}
			if !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("stderr missing %q:\n%s", tc.wantStderr, stderr)
			}
			if headers := rec.headers(); len(headers) != 0 {
				t.Errorf("a refused session file still let %d request(s) reach the gateway", len(headers))
			}
		})
	}
}

// TestTokenSecretHandling pins the root --token/--token-file contract
// through the packaged binary: the file path enforces the shared
// secret-file rules and authenticates the request; the deprecated flag
// keeps working but warns exactly once on stderr; every refusal happens
// before any request is built.
func TestTokenSecretHandling(t *testing.T) {
	binary := buildCLI(t)

	dir := t.TempDir()
	goodFile := filepath.Join(dir, "token")
	if err := os.WriteFile(goodFile, []byte("sekrettoken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	openFile := filepath.Join(dir, "token-open")
	if err := os.WriteFile(openFile, []byte("sekrettoken\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	emptyFile := filepath.Join(dir, "token-empty")
	if err := os.WriteFile(emptyFile, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkFile := filepath.Join(dir, "token-link")
	if err := os.Symlink(goodFile, linkFile); err != nil {
		t.Fatal(err)
	}

	run := func(t *testing.T, rec *authRecorder, args ...string) (int, string, string) {
		t.Helper()
		host, port := rec.hostPort(t)
		full := append([]string{"-s", host, "-p", port}, args...)
		cmd := exec.Command(binary, full...)
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		exit := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			exit = exitErr.ExitCode()
		} else if err != nil {
			t.Fatalf("running the CLI failed: %v", err)
		}
		if exit == 1 {
			t.Errorf("the reserved legacy exit 1 was emitted")
		}
		return exit, stdout.String(), stderr.String()
	}

	t.Run("token-file-authenticates", func(t *testing.T) {
		rec := newAuthRecorder(t)
		exit, stdout, stderr := run(t, rec, "--token-file", goodFile, "delete", "vlan", "100")
		if exit != 0 {
			t.Fatalf("exit %d, want 0\nstderr:\n%s", exit, stderr)
		}
		headers := rec.headers()
		if len(headers) == 0 || headers[0] != "Bearer sekrettoken" {
			t.Errorf("Authorization headers %q, want the file's token as a Bearer credential", headers)
		}
		if strings.Contains(stderr, "Warning:") || strings.Contains(stdout, "Warning:") {
			t.Errorf("the safe path must not warn:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
		}
	})

	t.Run("token-flag-warns-once-and-still-works", func(t *testing.T) {
		rec := newAuthRecorder(t)
		exit, stdout, stderr := run(t, rec, "--token", "sekrettoken", "delete", "vlan", "100")
		if exit != 0 {
			t.Fatalf("exit %d, want 0\nstderr:\n%s", exit, stderr)
		}
		headers := rec.headers()
		if len(headers) == 0 || headers[0] != "Bearer sekrettoken" {
			t.Errorf("Authorization headers %q, want the deprecated flag's token still on the wire", headers)
		}
		if got := strings.Count(stderr, "Warning:"); got != 1 {
			t.Errorf("deprecation warning on stderr %d times, want exactly once:\n%s", got, stderr)
		}
		if !strings.Contains(stderr, "--token-file") {
			t.Errorf("the warning must name the replacement flag:\n%s", stderr)
		}
		if strings.Contains(stdout, "Warning:") {
			t.Errorf("warning leaked to stdout (machine surface):\n%s", stdout)
		}
	})

	refusals := []struct {
		name       string
		args       []string
		wantExit   int
		wantStderr string
	}{
		{"both-flags-is-usage", []string{"--token", "x", "--token-file", goodFile, "delete", "vlan", "100"}, 2, "token-file"},
		{"relative-path-is-invalid", []string{"--token-file", "rel/token", "delete", "vlan", "100"}, 2, "absolute"},
		{"missing-file-is-precondition", []string{"--token-file", filepath.Join(dir, "nope"), "delete", "vlan", "100"}, 4, "cannot read the token file"},
		{"open-permissions-are-refused", []string{"--token-file", openFile, "delete", "vlan", "100"}, 4, "owner-only"},
		{"symlink-is-refused", []string{"--token-file", linkFile, "delete", "vlan", "100"}, 4, "symlink"},
		{"empty-file-is-refused", []string{"--token-file", emptyFile, "delete", "vlan", "100"}, 4, "empty"},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			rec := newAuthRecorder(t)
			exit, stdout, stderr := run(t, rec, tc.args...)
			if exit != tc.wantExit {
				t.Errorf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", exit, tc.wantExit, stdout, stderr)
			}
			if !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("stderr missing %q:\n%s", tc.wantStderr, stderr)
			}
			if strings.Contains(stdout, "Error:") {
				t.Errorf("failure text leaked to stdout:\n%s", stdout)
			}
			if headers := rec.headers(); len(headers) != 0 {
				t.Errorf("a refused invocation still sent %d request(s) to the gateway", len(headers))
			}
		})
	}
}
