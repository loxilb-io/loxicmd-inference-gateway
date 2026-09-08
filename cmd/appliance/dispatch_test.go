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
