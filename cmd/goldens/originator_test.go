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
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestOriginatorFlag pins what --originator puts on the wire from the
// packaged binary: one X-Loxilb-Originator header naming the OS account
// and host the binary runs as, on reads and on changes alike, and no such
// header at all without the flag.
func TestOriginatorFlag(t *testing.T) {
	binary := buildCLIWithSessionPath(t, filepath.Join(t.TempDir(), "session"))

	u, err := user.Current()
	if err != nil {
		t.Fatalf("current user: %v", err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatalf("hostname: %v", err)
	}
	want := "cli:" + u.Username + "@" + host

	// run executes the binary against a gateway that keeps every
	// originator header of every request, and returns them per request.
	run := func(t *testing.T, args ...string) [][]string {
		t.Helper()
		var mu sync.Mutex
		var seen [][]string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			seen = append(seen, r.Header.Values("X-Loxilb-Originator"))
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(successBody))
		}))
		defer srv.Close()
		su, err := url.Parse(srv.URL)
		if err != nil {
			t.Fatalf("parse gateway URL: %v", err)
		}
		cmd := exec.Command(binary, append([]string{"-s", su.Hostname(), "-p", su.Port()}, args...)...)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		// The exit status is not what is pinned here: the gateway answers
		// every path with the same body, which a read may refuse to
		// decode. Only a binary that could not run at all is a failure.
		if err := cmd.Run(); err != nil {
			if _, exited := err.(*exec.ExitError); !exited {
				t.Fatalf("running the CLI failed: %v\nstderr:\n%s", err, stderr.String())
			}
		}
		mu.Lock()
		defer mu.Unlock()
		if len(seen) == 0 {
			t.Fatalf("the CLI made no request")
		}
		return seen
	}

	rows := []struct {
		name string
		args []string
		sent bool
	}{
		{"change-with-flag", []string{"--originator", "delete", "vlan", "100"}, true},
		{"flag-after-the-subcommand", []string{"delete", "vlan", "100", "--originator"}, true},
		{"read-with-flag", []string{"--originator", "get", "audit-status"}, true},
		{"change-without-flag", []string{"delete", "vlan", "100"}, false},
		{"flag-set-false", []string{"--originator=false", "delete", "vlan", "100"}, false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			for i, got := range run(t, row.args...) {
				if !row.sent {
					if len(got) != 0 {
						t.Errorf("request %d carried X-Loxilb-Originator %q, want none", i, got)
					}
					continue
				}
				if len(got) != 1 || got[0] != want {
					t.Errorf("request %d carried X-Loxilb-Originator %q, want exactly %q", i, got, want)
				}
			}
		})
	}
}
