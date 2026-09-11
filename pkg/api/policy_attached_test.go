/*
 * Copyright (c) 2026 NetLOX Inc
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
	"encoding/json"
	"testing"
)

// The policy GET's "attached" field is the datapath truth about whether a
// policer is programmed or still pending its target. The pointer keeps three
// states apart: true, false, and absent (a gateway that predates the field)
// — collapsing absent into false would report a healthy policer on an old
// gateway as pending.
func TestPolicyAttachedDecodesThreeStates(t *testing.T) {
	payload := `{"polAttr":[
		{"policyIdent":"pol-live","attached":true},
		{"policyIdent":"pol-ghost","attached":false},
		{"policyIdent":"pol-old"}
	]}`
	var got PolInformationGet
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("decode policy GET payload: %v", err)
	}
	if len(got.PolModInfo) != 3 {
		t.Fatalf("decoded %d policies, want 3", len(got.PolModInfo))
	}
	byIdent := map[string]*bool{}
	for i := range got.PolModInfo {
		byIdent[got.PolModInfo[i].Ident] = got.PolModInfo[i].Attached
	}
	if v := byIdent["pol-live"]; v == nil || !*v {
		t.Errorf("pol-live attached = %v, want true", v)
	}
	if v := byIdent["pol-ghost"]; v == nil || *v {
		t.Errorf("pol-ghost attached = %v, want false", v)
	}
	if v := byIdent["pol-old"]; v != nil {
		t.Errorf("pol-old attached = %v, want nil (field absent on an old gateway)", *v)
	}
}

// Attached must never ride a create request: it is a read-only report, and
// serializing it on create would let a config file claim a datapath state.
func TestPolicyAttachedOmittedOnCreate(t *testing.T) {
	body, err := json.Marshal(PolMod{Ident: "pol-new"})
	if err != nil {
		t.Fatalf("marshal create body: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("re-decode create body: %v", err)
	}
	if _, present := raw["attached"]; present {
		t.Errorf("create body carries attached: %s", body)
	}
}
