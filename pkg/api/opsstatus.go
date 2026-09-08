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
package api

// Diagnostics is the client for GET /diagnostics — the gateway's
// secret-safe, allowlist-only diagnostic assembly.
type Diagnostics struct {
	CommonAPI
}

// EbpfAttachment mirrors one entry of the gateway's per-interface eBPF
// attachment report (kernel-verified).
type EbpfAttachment struct {
	Name     string `json:"name"`
	Mode     string `json:"mode"`
	Attached bool   `json:"attached"`
}

// ReadyState mirrors the /status/ready contract fields the CLI renders.
// The gateway serves the same body on 200 (ready) and 503 (not ready);
// the pointer on Ready keeps "absent" distinguishable from "false".
type ReadyState struct {
	Ready           *bool            `json:"ready"`
	Reasons         []string         `json:"reasons"`
	EbpfAttachments []EbpfAttachment `json:"ebpf_attachments"`
}

// MapUtilization mirrors one bounded per-table utilization entry.
type MapUtilization struct {
	Name     string `json:"name"`
	Count    int64  `json:"count"`
	Capacity int64  `json:"capacity"`
}

// DependencyDiagnostic mirrors one external dependency's reachability
// with its latency class - identity by type only.
type DependencyDiagnostic struct {
	Type         string `json:"type"`
	Required     bool   `json:"required"`
	Status       string `json:"status"`
	LatencyClass string `json:"latency_class"`
}

// DiagnosticsState mirrors the /diagnostics contract fields the CLI
// renders.
type DiagnosticsState struct {
	Version              string                 `json:"version"`
	BuildInfo            string                 `json:"build_info"`
	Product              string                 `json:"product"`
	APIVersion           string                 `json:"api_version"`
	UptimeSeconds        int64                  `json:"uptime_seconds"`
	Ready                *bool                  `json:"ready"`
	ReadyReasons         []string               `json:"ready_reasons"`
	MaintenanceState     string                 `json:"maintenance_state"`
	EbpfAttachments      []EbpfAttachment       `json:"ebpf_attachments"`
	Maps                 []MapUtilization       `json:"maps"`
	ExternalDependencies []DependencyDiagnostic `json:"external_dependencies"`
}
