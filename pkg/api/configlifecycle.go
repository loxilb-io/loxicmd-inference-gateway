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

// Snapshot is the client for GET /config/snapshot — the versioned, checksummed
// snapshot document covering all v1 configuration domains (supersedes the
// deprecated /config/export).
type Snapshot struct {
	CommonAPI
}

// Restore is the client for POST /config/restore — runs the staged restore
// pipeline on a posted snapshot document (supersedes the deprecated
// /config/import). Default mode is dry-run; commit must be explicit.
type Restore struct {
	CommonAPI
}

// Persist is the client for POST /config/persist — dumps the running
// configuration to disk so it survives a daemon restart (the API-side "save").
type Persist struct {
	CommonAPI
}

// RestorePlanItem is a per-domain apply/delete count from the restore PLAN stage.
type RestorePlanItem struct {
	Domain   string `json:"domain"`
	ToDelete int    `json:"to_delete"`
	ToApply  int    `json:"to_apply"`
}

// RestoreResult is the POST /config/restore response (both dry-run and commit).
type RestoreResult struct {
	Mode                        string            `json:"mode"`
	Compatible                  bool              `json:"compatible"`
	SchemaVersion               string            `json:"schema_version"`
	SnapshotGatewayVersion      string            `json:"snapshot_gateway_version"`
	CurrentGatewayVersion       string            `json:"current_gateway_version"`
	Plan                        []RestorePlanItem `json:"plan"`
	Errors                      []string          `json:"errors"`
	Result                      string            `json:"result"`
	PreRestoreSnapshotPersisted string            `json:"pre_restore_snapshot_persisted"`
}

// PersistResult is the POST /config/persist response.
type PersistResult struct {
	Result   string `json:"result"`
	Path     string `json:"path"`
	Checksum string `json:"checksum"`
}
