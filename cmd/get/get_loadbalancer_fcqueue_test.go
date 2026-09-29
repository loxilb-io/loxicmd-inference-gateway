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
package get

import (
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// A declared value is shown as is; an undeclared one (0) with a process
// default in force shows the default beside it; with neither, 0.
func TestFcQueueCell(t *testing.T) {
	cases := []struct {
		declared, effective uint32
		want                string
	}{
		{4, 4, "4"},
		{4, 16, "4"},
		{0, 16, "0 (default 16)"},
		{0, 0, "0"},
	}
	for _, c := range cases {
		if got := fcQueueCell(c.declared, c.effective); got != c.want {
			t.Errorf("fcQueueCell(%d, %d) = %q, want %q", c.declared, c.effective, got, c.want)
		}
	}
}

func TestFcGateCells(t *testing.T) {
	for _, c := range []struct {
		name         string
		eff          *api.FcEffective
		gate, maxOut string
	}{
		{"no gate state", nil, "-", "-"},
		{"with sources", &api.FcEffective{Mode: "enforce", MaxOutstanding: 4,
			Source: &api.FcEffectiveSource{Mode: "env", MaxOutstanding: "rule"}}, "enforce (env)", "4 (rule)"},
		{"a gateway without sources", &api.FcEffective{Mode: "observe", MaxOutstanding: 8}, "observe", "8"},
	} {
		t.Run(c.name, func(t *testing.T) {
			g, m := fcGateCells(c.eff)
			if g != c.gate || m != c.maxOut {
				t.Fatalf("cells %q / %q, want %q / %q", g, m, c.gate, c.maxOut)
			}
		})
	}
}

func TestFcAdaptCell(t *testing.T) {
	for _, c := range []struct {
		name string
		eff  *api.FcEffective
		want string
	}{
		{"no gate state", nil, "-"},
		{"a gateway without the adaptive ceiling", &api.FcEffective{Mode: "enforce", MaxOutstanding: 8}, "-"},
		{"off", &api.FcEffective{Mode: "enforce", MaxOutstanding: 8, AdaptState: "off", EffectiveMaxOutstanding: 8}, "off"},
		{"open", &api.FcEffective{Mode: "enforce", MaxOutstanding: 8, AdaptState: "open", AdaptReason: "none", EffectiveMaxOutstanding: 8}, "open 8/8"},
		{"tightened, with its reason", &api.FcEffective{Mode: "enforce", MaxOutstanding: 8, AdaptState: "tightened", AdaptReason: "ttft", EffectiveMaxOutstanding: 2}, "tightened 2/8 (ttft)"},
		{"warming endpoints", &api.FcEffective{Mode: "enforce", MaxOutstanding: 8, AdaptState: "open", EffectiveMaxOutstanding: 8, WarmingEndpoints: 2}, "open 8/8, 2 warming"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := fcAdaptCell(c.eff); got != c.want {
				t.Fatalf("cell %q, want %q", got, c.want)
			}
		})
	}
}

func TestFcTenantCell(t *testing.T) {
	for _, c := range []struct {
		name string
		eff  *api.FcEffective
		want string
	}{
		{"no gate state", nil, "-"},
		{"a gateway without the tenant share", &api.FcEffective{Mode: "enforce", MaxOutstanding: 8}, "-"},
		{"a hundred is no share", &api.FcEffective{Mode: "enforce", MaxOutstanding: 8, TenantMaxSharePct: 100}, "-"},
		{"in force, with its source", &api.FcEffective{Mode: "enforce", MaxOutstanding: 8, TenantMaxSharePct: 25, TenantsActive: 3,
			Source: &api.FcEffectiveSource{TenantMaxSharePct: "rule"}}, "25% (rule), 3 active"},
		{"idle, a gateway without sources", &api.FcEffective{Mode: "enforce", MaxOutstanding: 8, TenantMaxSharePct: 50}, "50%, 0 active"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := fcTenantCell(c.eff); got != c.want {
				t.Fatalf("cell %q, want %q", got, c.want)
			}
		})
	}
}
