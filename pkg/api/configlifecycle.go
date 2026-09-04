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
	"fmt"
	"strings"
)

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

// ExternalDependencyStatus is one entry of a snapshot document's
// recovery-dependency manifest plus the reporting operation's disposition
// toward it. Identity only — the gateway never puts store content or
// credentials in this structure.
//
// Persist responses report "ready" (identity read from the live process) or
// "configured" (store wired, reachability deliberately unclaimed). Restore
// responses report "verified", "warning", "failed" (the restore stopped
// before mutating anything) or "declared" (optional entry, informational).
type ExternalDependencyStatus struct {
	Type       string `json:"type"`
	ID         string `json:"id,omitempty"`
	Generation string `json:"generation,omitempty"`
	Digest     string `json:"digest,omitempty"`
	Required   bool   `json:"required,omitempty"`
	Status     string `json:"status,omitempty"`
}

// Dependency dispositions a restore response can report.
const (
	DependencyStatusVerified = "verified"
	DependencyStatusWarning  = "warning"
	DependencyStatusFailed   = "failed"
	DependencyStatusDeclared = "declared"
)

// String renders one dependency entry for human output: identity first, then
// the disposition, so an operator reading a failure sees which concrete store
// the restore refused to proceed without.
func (d ExternalDependencyStatus) String() string {
	id := d.Type
	if d.ID != "" {
		id += "/" + d.ID
	}
	scope := "optional"
	if d.Required {
		scope = "required"
	}
	status := d.Status
	if status == "" {
		status = "unreported"
	}
	return fmt.Sprintf("%s (%s): %s", id, scope, status)
}

// PersistResult is the POST /config/persist response: the persisted
// document's identity and coverage, so automation can verify what was saved
// without re-reading the file.
//
// Fields the gateway added with the durable-persist contract are pointers or
// slices precisely so "the server did not send it" stays distinguishable from
// "the server sent a zero" — an older gateway must degrade into legacy mode
// (see Capabilities), never into a false coverage claim.
type PersistResult struct {
	Result               string                     `json:"result"`
	Path                 string                     `json:"path"`
	Checksum             string                     `json:"checksum"`
	SchemaVersion        string                     `json:"schema_version,omitempty"`
	Generation           *uint64                    `json:"generation,omitempty"`
	IncludedDomains      []string                   `json:"included_domains,omitempty"`
	ExcludedDomains      []string                   `json:"excluded_domains,omitempty"`
	ExternalDependencies []ExternalDependencyStatus `json:"external_dependencies,omitempty"`
	Warnings             []string                   `json:"warnings,omitempty"`
}

// RestoreResult is the POST /config/restore response (dry-run, commit and the
// boot replay all use this shape). The gateway returns it as the body of 200,
// 400 and 500 alike, so the status code alone never decides the verdict —
// Verdict does, from the fields.
type RestoreResult struct {
	Mode                        string                     `json:"mode"`
	Compatible                  *bool                      `json:"compatible,omitempty"`
	SchemaVersion               string                     `json:"schema_version,omitempty"`
	SnapshotGatewayVersion      string                     `json:"snapshot_gateway_version,omitempty"`
	CurrentGatewayVersion       string                     `json:"current_gateway_version,omitempty"`
	Plan                        []RestorePlanItem          `json:"plan,omitempty"`
	Errors                      []string                   `json:"errors,omitempty"`
	Warnings                    []string                   `json:"warnings,omitempty"`
	Result                      string                     `json:"result,omitempty"`
	SnapshotGeneration          *uint64                    `json:"snapshot_generation,omitempty"`
	ExternalDependencies        []ExternalDependencyStatus `json:"external_dependencies,omitempty"`
	Persisted                   *bool                      `json:"persisted,omitempty"`
	PersistedGeneration         *uint64                    `json:"persisted_generation,omitempty"`
	PreRestoreSnapshotPersisted string                     `json:"pre_restore_snapshot_persisted,omitempty"`
}

// Restore modes and engine results, as the gateway spells them.
const (
	RestoreModeDryRun = "dry-run"
	RestoreModeCommit = "commit"

	RestoreResultOK             = "ok"
	RestoreResultRolledBack     = "rolled-back"
	RestoreResultRollbackFailed = "ROLLBACK-FAILED"

	// PersistResultOK is the only result a 200 persist may carry.
	PersistResultOK = "ok"
)

// LifecycleContract names how much of the config-lifecycle response contract
// the gateway that answered actually speaks. It exists so the CLI can say
// "this gateway did not report X" instead of printing a zero as if it were an
// answer.
type LifecycleContract string

const (
	// ContractDurable is the full durable-persist contract: identity
	// (schema version, lineage generation), declared coverage, the
	// recovery-dependency manifest and, for a committed restore, the
	// write-through disposition.
	ContractDurable LifecycleContract = "durable"
	// ContractLegacy is an older gateway that answers the same endpoints
	// with the pre-durability body. Everything the CLI cannot see, it must
	// not claim.
	ContractLegacy LifecycleContract = "legacy"
)

// Capabilities reports whether the persist response carries the durable
// contract's identity fields. Coverage (included/excluded domains) is not part
// of the test: a persist of a component subset legitimately reports narrow
// coverage, but no gateway that speaks the durable contract omits the
// document's schema version and lineage generation.
func (r *PersistResult) Capabilities() LifecycleContract {
	if r == nil {
		return ContractLegacy
	}
	if r.SchemaVersion != "" && r.Generation != nil {
		return ContractDurable
	}
	return ContractLegacy
}

// Capabilities reports whether the restore response carries the durable
// contract's fields. A dry-run never reaches the write-through, so the marker
// there is the document identity the pipeline echoes back; a commit is judged
// on the write-through disposition itself, which is the field automation
// actually depends on ("did the applied state become durable").
func (r *RestoreResult) Capabilities() LifecycleContract {
	if r == nil {
		return ContractLegacy
	}
	if r.Mode == RestoreModeCommit && r.Result == RestoreResultOK {
		if r.Persisted == nil {
			return ContractLegacy
		}
		return ContractDurable
	}
	if r.SchemaVersion != "" && r.Compatible != nil {
		return ContractDurable
	}
	return ContractLegacy
}

// DecodePersistResult strictly decodes a persist response body. A body the CLI
// cannot decode is a failure, never a shrug: the caller asked the gateway to
// make its configuration durable and has no evidence that it did.
func DecodePersistResult(body []byte) (*PersistResult, error) {
	var res PersistResult
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, &LifecycleError{
			Reason:  ReasonDecodeFailed,
			Message: fmt.Sprintf("cannot decode persist response: %v", err),
			Body:    string(body),
		}
	}
	return &res, nil
}

// DecodeRestoreResult strictly decodes a restore response body.
func DecodeRestoreResult(body []byte) (*RestoreResult, error) {
	var res RestoreResult
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, &LifecycleError{
			Reason:  ReasonDecodeFailed,
			Message: fmt.Sprintf("cannot decode restore response: %v", err),
			Body:    string(body),
		}
	}
	return &res, nil
}

// Verdict decides whether a decoded persist response is a success, from the
// body rather than from the status code. strict additionally refuses a legacy
// contract, so automation that depends on the persisted document's identity
// fails loudly against a gateway that cannot report it.
func (r *PersistResult) Verdict(strict bool) error {
	if r == nil {
		return &LifecycleError{Reason: ReasonDecodeFailed, Message: "empty persist response"}
	}
	if r.Result != PersistResultOK {
		msg := fmt.Sprintf("gateway reported result %q, want %q", r.Result, PersistResultOK)
		if r.Result == "" {
			msg = "gateway response carries no result field"
		}
		return &LifecycleError{Reason: ReasonResultNotOK, Message: msg}
	}
	if strict {
		if r.Capabilities() == ContractLegacy {
			return &LifecycleError{
				Reason:  ReasonContractLegacy,
				Message: "gateway did not report the persisted document's schema version and generation (--strict requires the durable-persist contract)",
			}
		}
		if r.Path == "" || r.Checksum == "" {
			return &LifecycleError{
				Reason:  ReasonContractLegacy,
				Message: "gateway did not report the persisted document's path and checksum (--strict requires them)",
			}
		}
	}
	return nil
}

// Verdict decides whether a decoded restore response is a success.
//
// Dry-run succeeds when the pipeline validated and planned: compatible, no
// errors. It deliberately does NOT require a result field — the engine leaves
// it empty when it stops before APPLY, which is exactly what a dry-run does.
//
// Commit succeeds only on result "ok" AND a write-through that did not fail.
// A committed restore whose write-through failed applied the configuration but
// did not make it durable: the next restart loses it. Reporting that as
// success is the failure mode this contract exists to remove.
func (r *RestoreResult) Verdict(strict bool) error {
	if r == nil {
		return &LifecycleError{Reason: ReasonDecodeFailed, Message: "empty restore response"}
	}
	if failed := r.FailedDependencies(); len(failed) > 0 {
		return &LifecycleError{
			Reason:  ReasonDependencyFailed,
			Message: "restore refused: " + strings.Join(failed, "; ") + joinDetail(r.Errors),
		}
	}
	if r.Compatible != nil && !*r.Compatible {
		return &LifecycleError{
			Reason:  ReasonIncompatible,
			Message: "snapshot is not compatible with this gateway" + joinDetail(r.Errors),
		}
	}
	switch r.Result {
	case RestoreResultRollbackFailed:
		return &LifecycleError{
			Reason:  ReasonRollbackFailed,
			Message: "restore failed AND its rollback failed - the gateway's running configuration is not the pre-restore configuration" + joinDetail(r.Errors),
		}
	case RestoreResultRolledBack:
		return &LifecycleError{
			Reason:  ReasonRolledBack,
			Message: "restore failed and was rolled back" + joinDetail(r.Errors),
		}
	}
	// The commit-specific verdicts come first, because a failed
	// write-through reports its detail in errors while result stays "ok":
	// judged by the generic errors branch it would be reported as a
	// validation failure, losing exactly the fact that matters - the
	// applied configuration is not durable.
	if r.Mode == RestoreModeCommit {
		if r.Result != RestoreResultOK {
			msg := fmt.Sprintf("gateway reported result %q, want %q", r.Result, RestoreResultOK)
			if r.Result == "" {
				msg = "committed restore carries no result field"
			}
			return &LifecycleError{Reason: ReasonResultNotOK, Message: msg}
		}
		if r.Persisted != nil && !*r.Persisted {
			return &LifecycleError{
				Reason:  ReasonNotPersisted,
				Message: "restore applied but was NOT persisted - the restored configuration will not survive a restart until a persist succeeds" + joinDetail(r.Errors),
			}
		}
	}
	if len(r.Errors) > 0 {
		return &LifecycleError{
			Reason:  ReasonValidationFailed,
			Message: "restore reported errors" + joinDetail(r.Errors),
		}
	}
	if strict && r.Capabilities() == ContractLegacy {
		what := "the document identity (schema version, compatibility)"
		if r.Mode == RestoreModeCommit {
			what = "the write-through disposition (persisted)"
		}
		return &LifecycleError{
			Reason:  ReasonContractLegacy,
			Message: "gateway did not report " + what + " (--strict requires the durable-persist contract)",
		}
	}
	return nil
}

// FailedDependencies lists the recovery dependencies this restore refused to
// proceed without, by identity. The restore engine verifies required entries
// before it plans, wipes or applies anything, so a failed entry means nothing
// was mutated.
func (r *RestoreResult) FailedDependencies() []string {
	var failed []string
	for _, dep := range r.ExternalDependencies {
		if dep.Status == DependencyStatusFailed {
			failed = append(failed, dep.String())
		}
	}
	return failed
}

// joinDetail appends server-supplied detail lines to a message, or nothing
// when the server sent none.
func joinDetail(detail []string) string {
	if len(detail) == 0 {
		return ""
	}
	return ": " + strings.Join(detail, "; ")
}
