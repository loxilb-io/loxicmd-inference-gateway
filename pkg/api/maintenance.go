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

// Maintenance is the client for GET/PUT /maintenance — the operator
// maintenance state of the gateway's management plane, with its drain
// read-back.
type Maintenance struct {
	CommonAPI
}

// MaintenanceRequest is the PUT /maintenance body.
type MaintenanceRequest struct {
	// Enabled true enters maintenance, false leaves it. Both directions
	// are idempotent on the gateway.
	Enabled bool `json:"enabled"`
	// DrainTimeoutSeconds declares the drain window on enter (0 = no
	// deadline). The gateway ignores it on a repeat enter and on leave.
	DrainTimeoutSeconds uint32 `json:"drain_timeout_seconds,omitempty"`
}

// MaintenanceState mirrors the gateway's MaintenanceStatus contract. The
// refusal booleans and counters are the gateway's observed truth — the CLI
// renders them, it never infers or embellishes them.
type MaintenanceState struct {
	State                 string `json:"state"`
	OperationID           string `json:"operation_id,omitempty"`
	RefusingNewConfig     bool   `json:"refusing_new_config"`
	RefusingNewInference  bool   `json:"refusing_new_inference"`
	InFlightStreams       int64  `json:"in_flight_streams"`
	EnteredAt             string `json:"entered_at,omitempty"`
	ElapsedSeconds        int64  `json:"elapsed_seconds"`
	DrainTimeoutSeconds   uint32 `json:"drain_timeout_seconds,omitempty"`
	DrainDeadlineExceeded bool   `json:"drain_deadline_exceeded"`
	Cancellable           bool   `json:"cancellable"`
}
