/*
 * Copyright (c) 2025 LoxiLB Authors
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
package dump

import (
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// TestSaveApiRejectsCombinationsItCannotHonor pins the honesty rule: --api
// used to be accepted alongside the legacy dump flags and then silently
// performed only the API persist, so 'save --api --all' reported success
// having written none of the text dumps its flag named.
func TestSaveApiRejectsCombinationsItCannotHonor(t *testing.T) {
	for name, opts := range map[string]SaveOptions{
		"all":      {SaveViaApi: true, SaveAllConfig: true},
		"lb":       {SaveViaApi: true, SaveLBConfig: true},
		"session":  {SaveViaApi: true, SaveSessionConfig: true},
		"ulcl":     {SaveViaApi: true, SaveUlClConfig: true},
		"firewall": {SaveViaApi: true, SaveFWConfig: true},
		"endpoint": {SaveViaApi: true, SaveEPConfig: true},
		"bfd":      {SaveViaApi: true, SaveBFDConfig: true},
	} {
		t.Run("--api --"+name, func(t *testing.T) {
			err := validateSaveOptions(&opts)
			if err == nil {
				t.Fatal("combination was accepted")
			}
			if got := api.ReasonOf(err); got != api.ReasonInvalidArguments {
				t.Fatalf("reason = %q", got)
			}
			if !strings.Contains(err.Error(), "--"+name) {
				t.Fatalf("failure does not name the offending flag: %v", err)
			}
		})
	}
}

// TestSaveApiWithIpIsHonest: interface configuration is host-level state the
// snapshot document excludes, so it is the one dump that legitimately runs
// alongside the API persist.
func TestSaveApiWithIpIsHonest(t *testing.T) {
	if err := validateSaveOptions(&SaveOptions{SaveViaApi: true, SaveIpConfig: true}); err != nil {
		t.Fatalf("--api --ip was rejected: %v", err)
	}
	if err := validateSaveOptions(&SaveOptions{SaveViaApi: true}); err != nil {
		t.Fatalf("--api alone was rejected: %v", err)
	}
}

// TestSaveConfigPathIsClientLocal: --config-path names where the CLI writes
// text dumps. It has never had any bearing on where the gateway writes
// snapshot.json, and accepting it with --api alone reads as if it did.
func TestSaveConfigPathIsClientLocal(t *testing.T) {
	err := validateSaveOptions(&SaveOptions{SaveViaApi: true, ConfigPath: "/tmp/somewhere"})
	if err == nil {
		t.Fatal("--api --config-path was accepted")
	}
	if got := api.ReasonOf(err); got != api.ReasonInvalidArguments {
		t.Fatalf("reason = %q", got)
	}
	if !strings.Contains(err.Error(), "does not change where the gateway") {
		t.Fatalf("failure does not explain the split: %v", err)
	}
	// With a local dump to place, the path is meaningful again.
	if err := validateSaveOptions(&SaveOptions{SaveViaApi: true, SaveIpConfig: true, ConfigPath: "/tmp/somewhere"}); err != nil {
		t.Fatalf("--api --ip --config-path was rejected: %v", err)
	}
}

func TestSaveRequiresASelection(t *testing.T) {
	err := validateSaveOptions(&SaveOptions{})
	if err == nil {
		t.Fatal("an empty save was accepted")
	}
	if got := api.ReasonOf(err); got != api.ReasonInvalidArguments {
		t.Fatalf("reason = %q", got)
	}
}

// TestSaveHelpDoesNotOverclaim: the help text used to promise that --api
// covers "every configuration domain", which the snapshot document itself
// contradicts by declaring the domains it excludes.
func TestSaveHelpDoesNotOverclaim(t *testing.T) {
	cmd := SaveCmd(&SaveOptions{}, &api.RESTOptions{})
	help := cmd.Long
	for _, forbidden := range []string{"every configuration domain", "every config domain"} {
		if strings.Contains(help, forbidden) {
			t.Fatalf("help still claims blanket coverage: %q", forbidden)
		}
	}
	for _, want := range []string{"loxicmd create persist", "compatibility alias", "excludes"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help does not mention %q:\n%s", want, help)
		}
	}
}
