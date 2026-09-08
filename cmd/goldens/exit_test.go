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
	"os/exec"
	"strings"
	"testing"
)

// TestExitTaxonomy pins the failure side of the released surface the same
// way TestGolden pins the success side: through the packaged binary. Each
// case is one row of contracts/exit-codes.md observed end to end — a
// handler that stopped classifying (print-and-return, a stray os.Exit, a
// silent non-200 fallthrough) fails here, not in review.
func TestExitTaxonomy(t *testing.T) {
	binary := buildCLI(t)

	cases := []struct {
		name string
		args []string
		// status/body drive the fake gateway; status 0 means "no
		// gateway at all" and the case runs against a closed port.
		status   int
		body     string
		wantExit int
		// wantStderr must appear on stderr — the only failure stream.
		wantStderr string
	}{
		{"missing-args-is-usage", []string{"delete", "vlan"}, http.StatusOK, successBody, 2, "Error:"},
		{"invalid-value-is-usage", []string{"delete", "vlan", "not-a-vid"}, http.StatusOK, successBody, 2, "not valid"},
		{"unknown-flag-is-usage", []string{"delete", "vlan", "100", "--frobnicate"}, http.StatusOK, successBody, 2, "unknown flag"},
		{"unknown-subcommand-is-usage", []string{"delete", "vlans", "100"}, http.StatusOK, successBody, 2, "unknown command"},
		{"unauthorized-is-auth", []string{"delete", "vlan", "100"}, http.StatusUnauthorized, `{}`, 3, "401"},
		{"absent-target-is-precondition", []string{"delete", "vlan", "100"}, http.StatusNotFound, `{}`, 4, "404"},
		{"refusing-service-is-unavailable", []string{"delete", "vlan", "100"}, http.StatusServiceUnavailable, `{}`, 5, "503"},
		{"peer-validation-is-contract-mismatch", []string{"delete", "vlan", "100"}, http.StatusBadRequest, `{}`, 6, "400"},
		{"server-error-is-failed", []string{"delete", "vlan", "100"}, http.StatusInternalServerError, `{}`, 7, "500"},
		{"unreachable-is-unavailable", []string{"delete", "vlan", "100"}, 0, "", 5, "Error:"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var host, port string
			if tc.status == 0 {
				// A port nothing listens on: reserve one, close it.
				gw := newFakeGateway(t, http.StatusOK, "")
				host, port = gw.hostPort(t)
				gw.server.Close()
			} else {
				gw := newFakeGateway(t, tc.status, tc.body)
				host, port = gw.hostPort(t)
			}
			args := append([]string{"-s", host, "-p", port}, tc.args...)
			cmd := exec.Command(binary, args...)
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
			if exit != tc.wantExit {
				t.Errorf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", exit, tc.wantExit, stdout.String(), stderr.String())
			}
			// Exit 1 is the reserved pre-taxonomy code: seeing it means
			// something bypassed the single exit point.
			if exit == 1 {
				t.Errorf("the reserved legacy exit 1 was emitted")
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr missing %q:\n%s", tc.wantStderr, stderr.String())
			}
			if strings.Contains(stdout.String(), "Error:") {
				t.Errorf("failure text leaked to stdout (must print once, to stderr):\n%s", stdout.String())
			}
		})
	}
}
