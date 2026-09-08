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

package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"testing"

	"github.com/spf13/cobra"
)

// runVersion executes the version command under a root that carries the same
// persistent -o/--output flag the real root defines, and returns what it
// wrote to stdout.
func runVersion(t *testing.T, args ...string) string {
	t.Helper()

	root := &cobra.Command{Use: "loxicmd"}
	var printOption string
	root.PersistentFlags().StringVarP(&printOption, "output", "o", "", "Set output layer (ex.) wide, json)")
	root.AddCommand(VersionCmd)
	root.SetArgs(append([]string{"version"}, args...))

	// The human path prints through fmt.Printf on the process stdout, so
	// capture the real stdout rather than only cobra's writer.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	VersionCmd.SetOut(w)
	execErr := root.Execute()
	w.Close()
	os.Stdout = old
	out, readErr := io.ReadAll(r)
	if execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	if readErr != nil {
		t.Fatalf("read captured stdout: %v", readErr)
	}
	return string(out)
}

// TestVersionHumanOutputUnchanged pins the released human output byte for
// byte: adding the JSON path must not move the default surface at all.
func TestVersionHumanOutputUnchanged(t *testing.T) {
	got := runVersion(t)
	want := fmt.Sprintf("Loxicmd version: %s\nLoxicmd build info: %s\n", Version, BuildInfo)
	if got != want {
		t.Errorf("human output changed:\ngot  %q\nwant %q", got, want)
	}
}

// TestVersionJSONEnvelope proves -o json emits exactly one CommandResult
// document whose identity fields carry the stamped build facts, with the Go
// toolchain read from the running binary.
func TestVersionJSONEnvelope(t *testing.T) {
	origRev, origContract := SourceRevision, GatewayContract
	SourceRevision = "0123456789abcdef0123456789abcdef01234567"
	GatewayContract = "feedface"
	defer func() { SourceRevision, GatewayContract = origRev, origContract }()

	out := runVersion(t, "-o", "json")

	dec := json.NewDecoder(bytes.NewReader([]byte(out)))
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("decode: %v\noutput: %q", err, out)
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		t.Fatal("more than one JSON document on stdout")
	}

	for key, want := range map[string]any{
		"apiVersion": "loxilb.io/appliance/v1",
		"kind":       "CommandResult",
		"command":    "version",
		"success":    true,
		"code":       "OK",
	} {
		if doc[key] != want {
			t.Errorf("%s = %v, want %v", key, doc[key], want)
		}
	}

	data, ok := doc["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is %T, want an object", doc["data"])
	}
	for key, want := range map[string]string{
		"version":         Version,
		"sourceRevision":  "0123456789abcdef0123456789abcdef01234567",
		"gatewayContract": "feedface",
		"goVersion":       runtime.Version(),
	} {
		if data[key] != want {
			t.Errorf("data.%s = %v, want %q", key, data[key], want)
		}
	}
	// Unstamped facts must be reported as empty strings, never omitted or
	// null: attestation distinguishes a stamped binary by exactly this.
	for _, key := range []string{"buildWorkflow", "sourceDateEpoch", "buildInfo"} {
		v, present := data[key]
		if !present {
			t.Errorf("data.%s missing; unstamped facts are reported empty, not omitted", key)
		} else if _, isString := v.(string); !isString {
			t.Errorf("data.%s is %T, want string", key, v)
		}
	}
}
