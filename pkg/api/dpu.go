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

// DPU is the client for DPU offload debug (/config/dpu).
// This is a raw-middleware endpoint (see api/swagger-extras.yml); its error
// bodies use the SimpleError {"error": "..."} envelope.
// GET debug returns offload state/counters; GET hwcounters returns per-flow
// hardware counters; POST debug triggers a debug action.
type DPU struct {
	CommonAPI
}

// DPUDebugAction is the POST /config/dpu/debug body. action is required
// (unregister or cb_force); plugin is required for unregister, mode for
// cb_force (open or close).
type DPUDebugAction struct {
	Action string `json:"action"`
	Plugin string `json:"plugin,omitempty"`
	Mode   string `json:"mode,omitempty"`
}
