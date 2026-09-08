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
package appliance

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildCLIWithBackend builds the packaged binary with the backend path
// relocated to a fake via the build-time variable — the only relocation
// mechanism the invocation contract permits, so the test proves it works.
func buildCLIWithBackend(t *testing.T, backendScript string) (binary, backendDir string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping packaged-binary test in short mode")
	}
	if runtime.GOOS != "linux" {
		t.Skip("loxicmd links Linux netlink; the packaged binary only builds there")
	}
	backendDir = t.TempDir()
	backendPath := filepath.Join(backendDir, "loxilb-appliance-backend")
	if err := os.WriteFile(backendPath, []byte("#!/bin/sh\n"+backendScript), 0o755); err != nil {
		t.Fatal(err)
	}
	binary = filepath.Join(t.TempDir(), "loxicmd")
	cmd := exec.Command("go", "build",
		"-ldflags", "-X github.com/loxilb-io/loxicmd-inference-gateway/pkg/backend.executablePath="+backendPath,
		"-o", binary, ".")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the CLI failed: %v\n%s", err, out)
	}
	return binary, backendDir
}

func runAppliance(t *testing.T, binary string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	status := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		status = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running the CLI failed: %v", err)
	}
	return status, stdout.String(), stderr.String()
}

// TestApplianceDispatch proves the family end to end through the packaged
// binary: no gateway is running anywhere in this test, which is itself a
// contract clause — appliance commands must work with the gateway down.
func TestApplianceDispatch(t *testing.T) {
	binary, _ := buildCLIWithBackend(t,
		`case "$1" in
status) if [ "$4" = "--json" ]; then echo '{"release":"v0.9.8.9-rc.1","planes":{"data":"READY"}}'; else echo "Appliance: READY"; fi ;;
network) echo '{"profile":"single-arm","errors":[]}' ;;
*) echo "unknown" >&2; exit 64 ;;
esac`)

	t.Run("status human passthrough exits 0", func(t *testing.T) {
		status, stdout, stderr := runAppliance(t, binary, "appliance", "status")
		if status != 0 || !strings.Contains(stdout, "Appliance: READY") {
			t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
		}
	})

	t.Run("status json is the CommandResult envelope around the backend document", func(t *testing.T) {
		status, stdout, _ := runAppliance(t, binary, "appliance", "status", "-o", "json")
		if status != 0 {
			t.Fatalf("status=%d stdout=%q", status, stdout)
		}
		var doc struct {
			APIVersion    string `json:"apiVersion"`
			Kind          string `json:"kind"`
			Command       string `json:"command"`
			Success       bool   `json:"success"`
			CorrelationID string `json:"correlationId"`
			Data          struct {
				Backend struct {
					Release string `json:"release"`
				} `json:"backend"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("stdout is not the envelope (%v): %s", err, stdout)
		}
		if doc.Kind != "CommandResult" || !doc.Success || doc.Command != "appliance.status" ||
			doc.Data.Backend.Release != "v0.9.8.9-rc.1" {
			t.Fatalf("unexpected envelope: %s", stdout)
		}
		if !strings.HasPrefix(doc.CorrelationID, "cli-") {
			t.Fatalf("correlationId %q not echoed", doc.CorrelationID)
		}
	})

	t.Run("network validate dispatches its two-word subcommand", func(t *testing.T) {
		status, stdout, _ := runAppliance(t, binary, "appliance", "network", "validate", "-o", "json")
		var doc struct {
			Command string `json:"command"`
			Data    struct {
				Backend struct {
					Profile string `json:"profile"`
				} `json:"backend"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("stdout is not the envelope (%v): %s", err, stdout)
		}
		if status != 0 || doc.Command != "appliance.network.validate" ||
			doc.Data.Backend.Profile != "single-arm" {
			t.Fatalf("status=%d stdout=%q", status, stdout)
		}
	})

	t.Run("unavailable command exits 6 and never stubs success", func(t *testing.T) {
		status, stdout, stderr := runAppliance(t, binary, "appliance", "factory-reset", "-o", "json")
		if status != 6 {
			t.Fatalf("status=%d, want the taxonomy's 6\nstdout=%q stderr=%q", status, stdout, stderr)
		}
		var doc struct {
			Success bool `json:"success"`
			Data    struct {
				ComponentCode string `json:"componentCode"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("failure envelope missing (%v): %s", err, stdout)
		}
		if doc.Success || doc.Data.ComponentCode != "command-unavailable" {
			t.Fatalf("unexpected failure envelope: %s", stdout)
		}
	})

	t.Run("bare appliance is an invalid invocation", func(t *testing.T) {
		status, _, _ := runAppliance(t, binary, "appliance")
		if status != 2 {
			t.Fatalf("status=%d, want 2", status)
		}
	})
}

// TestApplianceBackendFailures proves refusal and absence classify onto the
// taxonomy with the backend's own signal preserved.
func TestApplianceBackendFailures(t *testing.T) {
	t.Run("backend refusal exits 7 with its code preserved", func(t *testing.T) {
		binary, _ := buildCLIWithBackend(t, "echo broken-state >&2\nexit 12")
		status, stdout, stderr := runAppliance(t, binary, "appliance", "status", "-o", "json")
		if status != 7 {
			t.Fatalf("status=%d, want 7\nstderr=%q", status, stderr)
		}
		var doc struct {
			Data struct {
				Origin        string `json:"origin"`
				ComponentCode string `json:"componentCode"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("failure envelope missing (%v): %s", err, stdout)
		}
		if doc.Data.Origin != "backend" || doc.Data.ComponentCode != "backend-exit-12" {
			t.Fatalf("backend signal lost: %s", stdout)
		}
	})

	t.Run("absent backend exits 5 BACKEND_UNAVAILABLE", func(t *testing.T) {
		binary, backendDir := buildCLIWithBackend(t, "")
		if err := os.Remove(filepath.Join(backendDir, "loxilb-appliance-backend")); err != nil {
			t.Fatal(err)
		}
		status, _, stderr := runAppliance(t, binary, "appliance", "status")
		if status != 5 || !strings.Contains(stderr, "could not be executed") {
			t.Fatalf("status=%d stderr=%q, want 5 + the unavailability line", status, stderr)
		}
	})

	t.Run("non-json backend answer in json mode is a contract error", func(t *testing.T) {
		binary, _ := buildCLIWithBackend(t, "echo this is prose")
		status, stdout, _ := runAppliance(t, binary, "appliance", "status", "-o", "json")
		if status != 6 || !strings.Contains(stdout, "contract-invalid") {
			t.Fatalf("status=%d stdout=%q, want 6 + contract-invalid", status, stdout)
		}
	})
}

// mutatingFake answers the handshake advertising the full mutating slice
// and records what reaches each channel.
const mutatingFakeScript = `mkdir -p "$RECDIR" 2>/dev/null
printf '%s\n' "$@" > "$RECDIR/argv"
env > "$RECDIR/env"
cat > "$RECDIR/stdin"
if [ "$1" = "contract-version" ]; then
  echo '{"apiVersion":"loxilb.io/appliance-backend/v1","kind":"BackendContract","backendVersion":"0.1.0","productRelease":"rc","schemaVersion":1,"commands":[{"name":"gateway register-local","readOnly":false,"capabilities":[]},{"name":"logs","readOnly":false,"capabilities":[]}]}'
else
  echo '{"done":true}'
fi`

// TestApplianceMutatingDispatch proves the mutating path end to end: the
// handshake gate, the secret's stdin-only travel, and the CLI-side
// validations that refuse an invocation before any process starts.
func TestApplianceMutatingDispatch(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("loxicmd links Linux netlink; the packaged binary only builds there")
	}
	recDir := t.TempDir()
	binary, _ := buildCLIWithBackend(t, strings.ReplaceAll(mutatingFakeScript, "$RECDIR", recDir))
	secretDir := t.TempDir()
	passFile := filepath.Join(secretDir, "oam.pass")
	if err := os.WriteFile(passFile, []byte("correcthorsebatterystaple\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("register-local streams the secret on stdin only", func(t *testing.T) {
		status, stdout, stderr := runAppliance(t, binary,
			"appliance", "gateway", "register-local", "--username", "admin", "--password-file", passFile, "-o", "json")
		if status != 0 {
			t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
		}
		stdin, _ := os.ReadFile(filepath.Join(recDir, "stdin"))
		argv, _ := os.ReadFile(filepath.Join(recDir, "argv"))
		env, _ := os.ReadFile(filepath.Join(recDir, "env"))
		if !strings.Contains(string(stdin), "correcthorsebatterystaple") {
			t.Fatalf("secret did not reach stdin: %q", stdin)
		}
		if strings.Contains(string(argv), "correcthorse") || strings.Contains(string(env), "correcthorse") ||
			strings.Contains(stdout, "correcthorse") {
			t.Fatal("the secret leaked outside the backend's stdin")
		}
	})

	t.Run("group-readable password file is refused before any spawn", func(t *testing.T) {
		loose := filepath.Join(secretDir, "loose.pass")
		if err := os.WriteFile(loose, []byte("opensesame"), 0o644); err != nil {
			t.Fatal(err)
		}
		status, _, stderr := runAppliance(t, binary,
			"appliance", "gateway", "register-local", "--username", "admin", "--password-file", loose)
		if status != 4 || !strings.Contains(stderr, "owner-only") {
			t.Fatalf("status=%d stderr=%q, want 4 + the owner-only refusal", status, stderr)
		}
	})

	t.Run("unadvertised mutating command exits 6 after the handshake", func(t *testing.T) {
		status, _, stderr := runAppliance(t, binary,
			"appliance", "public-address", "configure", "203.0.113.10")
		if status != 6 || !strings.Contains(stderr, "does not provide") {
			t.Fatalf("status=%d stderr=%q, want 6 + the unadvertised refusal", status, stderr)
		}
	})

	t.Run("logs dispatches through the gate", func(t *testing.T) {
		status, _, stderr := runAppliance(t, binary,
			"appliance", "logs", "gateway", "--redact", "--since", "1h", "--lines", "200")
		if status != 0 {
			t.Fatalf("status=%d stderr=%q", status, stderr)
		}
	})

	for name, tc := range map[string]struct {
		args     []string
		wantExit int
		wantErr  string
	}{
		"bad ipv4":                    {[]string{"appliance", "public-address", "configure", "not.an.ip"}, 2, "canonical IPv4"},
		"non-canonical ipv4":          {[]string{"appliance", "public-address", "configure", "203.000.113.10"}, 2, "canonical IPv4"},
		"bad log component":           {[]string{"appliance", "logs", "sshd", "--redact"}, 2, "allowlist"},
		"logs without redact":         {[]string{"appliance", "logs", "gateway"}, 2, "--redact is required"},
		"diagnostics without redact":  {[]string{"appliance", "diagnostics", "create"}, 2, "--redact is required"},
		"relative diagnostics output": {[]string{"appliance", "diagnostics", "create", "--redact", "--output", "rel.tar"}, 2, "absolute"},
		"oversized since":             {[]string{"appliance", "logs", "gateway", "--redact", "--since", "400h"}, 2, "up to"},
		"oversized lines":             {[]string{"appliance", "logs", "gateway", "--redact", "--lines", "99999"}, 2, "between"},
		"flag-shaped username":        {[]string{"appliance", "gateway", "register-local", "--username", "-admin", "--password-file", passFile}, 2, "refuses"},
		"relative archive":            {[]string{"appliance", "backup", "verify", "rel.tar", "--key-file", passFile}, 2, "absolute"},
	} {
		t.Run(name+" is refused CLI-side", func(t *testing.T) {
			status, _, stderr := runAppliance(t, binary, tc.args...)
			if status != tc.wantExit || !strings.Contains(stderr, tc.wantErr) {
				t.Fatalf("status=%d stderr=%q, want %d + %q", status, stderr, tc.wantExit, tc.wantErr)
			}
		})
	}

	t.Run("credentials bootstrap refuses a non-terminal session", func(t *testing.T) {
		// runAppliance wires pipes, which is exactly the redirected
		// session the console-only guard must refuse.
		status, _, stderr := runAppliance(t, binary, "appliance", "credentials", "bootstrap")
		if status != 4 || !strings.Contains(stderr, "interactive local console") {
			t.Fatalf("status=%d stderr=%q, want 4 + the console-only refusal", status, stderr)
		}
	})

	t.Run("credentials bootstrap has no json mode", func(t *testing.T) {
		status, stdout, _ := runAppliance(t, binary, "appliance", "credentials", "bootstrap", "-o", "json")
		if status != 2 || strings.Contains(stdout, "CommandResult") {
			t.Fatalf("status=%d stdout=%q, want 2 and no envelope", status, stdout)
		}
	})
}
