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
	"encoding/json"
	"testing"
)

// Path/query construction for the v2 (Phase 5) resources must match the
// swagger and swagger-extras routes.
func TestV2ResourceURLs(t *testing.T) {
	c := NewLoxiClient(opts())
	const base = "http://127.0.0.1:11111/netlox/v1/"
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"gpu status", c.GPU().SubResources([]string{"status"}).GetUrlString(), base + "config/gpu/status"},
		{"gpu enable", c.GPU().SubResources([]string{"enable"}).GetUrlString(), base + "config/gpu/enable"},
		{"gpu cleanup", c.GPU().SubResources([]string{"conversations", "cleanup"}).GetUrlString(), base + "config/gpu/conversations/cleanup"},
		{"gpu cleanup q", c.GPU().SubResources([]string{"conversations", "cleanup"}).Query(map[string]string{"max_age_hours": "2"}).GetUrlString(), base + "config/gpu/conversations/cleanup?max_age_hours=2"},
		{"worker metrics", c.WorkerMetrics().GetUrlString(), base + "config/worker/metrics"},
		{"pii enable", c.PII().SubResources([]string{"enable"}).GetUrlString(), base + "config/pii/enable"},
		{"pii configure", c.PII().SubResources([]string{"configure"}).GetUrlString(), base + "config/pii/configure"},
		{"pii url-patterns", c.PII().SubResources([]string{"url-patterns"}).GetUrlString(), base + "config/pii/url-patterns"},
		{"pii status", c.PII().SubResources([]string{"status"}).GetUrlString(), base + "config/pii/status"},
		{"pii stats", c.PII().SubResources([]string{"stats"}).GetUrlString(), base + "config/pii/stats"},
		{"llamafirewall enable", c.LlamaFirewall().SubResources([]string{"enable"}).GetUrlString(), base + "config/llamafirewall/enable"},
		{"llamafirewall scanners", c.LlamaFirewall().SubResources([]string{"scanners"}).GetUrlString(), base + "config/llamafirewall/scanners"},
		{"llamafirewall health", c.LlamaFirewall().SubResources([]string{"health"}).GetUrlString(), base + "config/llamafirewall/health"},
		{"trace enable", c.Trace().SubResources([]string{"enable"}).GetUrlString(), base + "config/trace/enable"},
		{"trace status", c.Trace().SubResources([]string{"status"}).GetUrlString(), base + "config/trace/status"},
		{"trace otlp", c.Trace().SubResources([]string{"otlp"}).GetUrlString(), base + "config/trace/otlp"},
		{"trace parsers", c.Trace().SubResources([]string{"parsers"}).GetUrlString(), base + "config/trace/parsers"},
		{"l4trace enable", c.L4Trace().SubResources([]string{"enable"}).GetUrlString(), base + "config/l4trace/enable"},
		{"l4trace sampling", c.L4Trace().SubResources([]string{"sampling"}).GetUrlString(), base + "config/l4trace/sampling"},
		{"l4trace stats reset", c.L4Trace().SubResources([]string{"stats", "reset"}).GetUrlString(), base + "config/l4trace/stats/reset"},
		{"opa watcher", c.OPA().GetUrlString(), base + "config/opa/watcher"},
		{"dpu debug", c.DPU().SubResources([]string{"debug"}).GetUrlString(), base + "config/dpu/debug"},
		{"dpu debug q", c.DPU().SubResources([]string{"debug"}).Query(map[string]string{"flows": "1"}).GetUrlString(), base + "config/dpu/debug?flows=1"},
		{"dpu hwcounters", c.DPU().SubResources([]string{"hwcounters"}).GetUrlString(), base + "config/dpu/hwcounters"},
		{"snapshot", c.Snapshot().GetUrlString(), base + "config/snapshot"},
		{"restore commit", c.Restore().Query(map[string]string{"mode": "commit"}).GetUrlString(), base + "config/restore?mode=commit"},
		{"persist", c.Persist().GetUrlString(), base + "config/persist"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

// Configure bodies must omit unset fields so an unset flag never overwrites a
// server-side default, and must keep the exact swagger property names.
func TestV2ConfigureBodiesOmitEmpty(t *testing.T) {
	// PII: only mode + score_threshold set.
	mode := "mask"
	thr := 0.7
	b, _ := json.Marshal(PIIConfigEntry{Mode: &mode, ScoreThreshold: &thr})
	if string(b) != `{"mode":"mask","score_threshold":0.7}` {
		t.Errorf("pii configure body = %s", b)
	}
	// LlamaFirewall scanners: enabling prompt_guard, disabling code_shield.
	// A pointer keeps the explicit false; the untouched scanners are omitted.
	tr := true
	fl := false
	b, _ = json.Marshal(LlamaFirewallScannersEntry{PromptGuard: &tr, CodeShield: &fl})
	if string(b) != `{"prompt_guard":true,"code_shield":false}` {
		t.Errorf("llamafirewall scanners body = %s", b)
	}
	// Empty configure marshals to {} (nothing sent).
	b, _ = json.Marshal(LlamaFirewallConfigEntry{})
	if string(b) != `{}` {
		t.Errorf("empty llamafirewall configure body = %s", b)
	}
}

// Request bodies whose fields the server requires must always be present.
func TestV2RequiredBodies(t *testing.T) {
	b, _ := json.Marshal(PIIEnableRequest{Enabled: true})
	if string(b) != `{"enabled":true}` {
		t.Errorf("pii enable body = %s", b)
	}
	b, _ = json.Marshal(TraceOTLPConfig{Endpoint: "jaeger:4317", Protocol: "grpc"})
	if string(b) != `{"endpoint":"jaeger:4317","protocol":"grpc"}` {
		t.Errorf("trace otlp body = %s", b)
	}
	b, _ = json.Marshal(L4TraceSamplingRequest{SamplingRate: 10})
	if string(b) != `{"sampling_rate":10}` {
		t.Errorf("l4trace sampling body = %s", b)
	}
	b, _ = json.Marshal(OPAWatcherConfig{OpaURL: "http://opa:8181"})
	if string(b) != `{"opa_url":"http://opa:8181"}` {
		t.Errorf("opa config body = %s", b)
	}
	b, _ = json.Marshal(DPUDebugAction{Action: "cb_force", Mode: "open"})
	if string(b) != `{"action":"cb_force","mode":"open"}` {
		t.Errorf("dpu action body = %s", b)
	}
	b, _ = json.Marshal(PIIURLPatternsEntry{Mode: "replace", Patterns: []PIIURLPattern{{Pattern: "/v1/chat/*"}}})
	if string(b) != `{"mode":"replace","patterns":[{"pattern":"/v1/chat/*","is_exclude":false}]}` {
		t.Errorf("pii url-patterns body = %s", b)
	}
}
