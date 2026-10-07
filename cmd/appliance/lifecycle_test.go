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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the real binary and prove removed commands never reach the backend.
func TestExcludedLifecycleSurface(t *testing.T) {
	binary, backendDir := buildCLIWithBackend(t, `touch "$(dirname "$0")/called"
exit 99`)
	excluded := []string{"restore", "update", "rollback", "factory-reset", "reset"}
	for _, name := range excluded {
		t.Run(name, func(t *testing.T) {
			status, stdout, stderr := runAppliance(t, binary, "appliance", name, "plan")
			if status != 2 || !strings.Contains(stdout+stderr, "unknown command") {
				t.Fatalf("removed command: status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
		})
	}
	for _, args := range [][]string{
		{"appliance", "--help"},
		{"help", "appliance"},
		{"__complete", "appliance", ""},
		{"completion", "bash"},
		{"completion", "fish"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			status, stdout, stderr := runAppliance(t, binary, args...)
			if status != 0 {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
			}
			// Gateway configuration restore is a separate supported command.
			// Inspect appliance-specific completion functions in generated scripts.
			surface := stdout
			if args[0] == "completion" && args[1] == "bash" {
				start := strings.Index(stdout, "_loxicmd_appliance()")
				if start < 0 {
					t.Fatal("appliance completion function missing")
				}
				surface = stdout[start:]
				if end := strings.Index(surface, "\n}\n"); end >= 0 {
					surface = surface[:end]
				}
			}
			if args[0] == "completion" && args[1] == "fish" {
				// Cobra's fish script delegates to the dynamic completion path,
				// exercised above, instead of embedding the command tree.
				if !strings.Contains(stdout, "__complete") {
					t.Fatal("dynamic completion hook missing")
				}
				return
			}

			for _, name := range excluded {
				if strings.Contains(surface, name) {
					t.Fatalf("excluded %q in surface: %s", name, surface)
				}
			}
			for _, name := range []string{"status", "network", "public-address", "gateway", "credentials", "diagnostics", "logs", "backup"} {
				if !strings.Contains(surface, name) {
					t.Fatalf("supported %q missing: %s", name, surface)
				}
			}
		})
	}
	if _, err := os.Stat(filepath.Join(backendDir, "called")); !os.IsNotExist(err) {
		t.Fatalf("help, completion, or removed command dispatched to backend: %v", err)
	}
}
