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
package api

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// lifecycleModels maps the gateway definitions the CLI consumes onto the Go
// types that model them.
var lifecycleModels = map[string]reflect.Type{
	"PersistResult":            reflect.TypeOf(PersistResult{}),
	"RestoreResult":            reflect.TypeOf(RestoreResult{}),
	"RestorePlanItem":          reflect.TypeOf(RestorePlanItem{}),
	"ExternalDependencyStatus": reflect.TypeOf(ExternalDependencyStatus{}),
}

// TestLifecycleModelsCoverTheContractManifest closes the loop the manifest
// opens. TestGatewayContractAgainstCheckout proves the manifest describes the
// gateway's spec; this proves the Go models describe the manifest. Without it
// a field could be added to the spec and the manifest and still be silently
// dropped on decode, which is how a durability flag goes unread.
func TestLifecycleModelsCoverTheContractManifest(t *testing.T) {
	manifest := loadGatewayContractManifest(t)
	for _, field := range manifest.Fields {
		model, tracked := lifecycleModels[field.Definition]
		if !tracked || len(field.PropertyPath) != 1 {
			continue
		}
		property := field.PropertyPath[0]
		if !hasJSONField(model, property) {
			t.Errorf("%s does not model %s.%s, so the gateway's value is dropped on decode",
				model.Name(), field.Definition, property)
		}
	}
}

// TestLifecycleEnumsAreDeclaredByTheGateway pins the other direction for the
// values the CLI branches on: every constant it compares against must be a
// value the spec actually declares, or the comparison can never match.
func TestLifecycleEnumsAreDeclaredByTheGateway(t *testing.T) {
	manifest := loadGatewayContractManifest(t)
	enums := make(map[string][]string)
	for _, field := range manifest.Fields {
		if len(field.PropertyPath) == 1 && len(field.Enum) > 0 {
			enums[field.Definition+"."+field.PropertyPath[0]] = field.Enum
		}
	}

	for name, tc := range map[string]struct {
		key    string
		values []string
	}{
		"restore modes": {"RestoreResult.mode", []string{RestoreModeDryRun, RestoreModeCommit}},
		"dependency dispositions": {"ExternalDependencyStatus.status", []string{
			DependencyStatusReady, DependencyStatusConfigured, DependencyStatusVerified,
			DependencyStatusWarning, DependencyStatusFailed, DependencyStatusDeclared}},
	} {
		t.Run(name, func(t *testing.T) {
			declared, ok := enums[tc.key]
			if !ok {
				t.Fatalf("the contract manifest does not pin %s", tc.key)
			}
			for _, value := range tc.values {
				if !containsContractValue(declared, value) {
					t.Errorf("the CLI compares against %q, which %s does not declare (declared: %s)",
						value, tc.key, strings.Join(sorted(declared), ", "))
				}
			}
		})
	}
}

func hasJSONField(model reflect.Type, property string) bool {
	for i := 0; i < model.NumField(); i++ {
		tag := model.Field(i).Tag.Get("json")
		if name, _, _ := strings.Cut(tag, ","); name == property {
			return true
		}
	}
	return false
}

func sorted(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
