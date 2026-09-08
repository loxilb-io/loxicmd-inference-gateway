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
	"fmt"
	"net/http"
)

// Reason codes for the configuration-lifecycle commands (persist, restore,
// snapshot, save --api). They are a STABLE automation surface: scripts branch
// on them, so a code's meaning must never be repurposed and a code must never
// disappear from a failure it used to describe. New failure classes get new
// codes.
const (
	// ReasonOK marks a successful operation.
	ReasonOK = "ok"
	// ReasonInvalidArguments — the command line itself is wrong; the
	// gateway was never contacted.
	ReasonInvalidArguments = "invalid-arguments"
	// ReasonFileRead / ReasonFileWrite — a client-local file operation
	// failed (reading the snapshot to restore, writing the downloaded one).
	ReasonFileRead  = "file-read-failed"
	ReasonFileWrite = "file-write-failed"
	// ReasonInvalidJSON — a client-local file is not a JSON document.
	ReasonInvalidJSON = "invalid-json"
	// ReasonRequestFailed — the request never produced an HTTP response
	// (connection refused, TLS failure, timeout).
	ReasonRequestFailed = "request-failed"
	// ReasonUnauthorized — 401.
	ReasonUnauthorized = "unauthorized"
	// ReasonBusy — 409: another snapshot or restore holds the gate.
	ReasonBusy = "operation-in-progress"
	// ReasonMaintenance — 503: the gateway has not finished booting, or is
	// otherwise not accepting configuration operations.
	ReasonMaintenance = "maintenance-mode"
	// ReasonRecoveryRequired — a state-changing request failed in a way
	// that leaves the gateway's state unknown to this process (the
	// request may or may not have been applied). The command never
	// reports success from here; the operator must verify the actual
	// state before acting on any assumption about it.
	ReasonRecoveryRequired = "recovery-required"
	// ReasonServerError — a 5xx that carried no lifecycle body.
	ReasonServerError = "server-error"
	// ReasonBadRequest — a 4xx that carried no lifecycle body.
	ReasonBadRequest = "bad-request"
	// ReasonDecodeFailed — the response body could not be decoded into the
	// documented model. Never treated as success.
	ReasonDecodeFailed = "decode-failed"
	// ReasonResultNotOK — the body decoded but its result field is not the
	// documented success value.
	ReasonResultNotOK = "result-not-ok"
	// ReasonIncompatible — the snapshot document cannot be restored onto
	// this gateway.
	ReasonIncompatible = "incompatible-snapshot"
	// ReasonValidationFailed — the pipeline reported errors without
	// reaching (or needing) a rollback.
	ReasonValidationFailed = "validation-failed"
	// ReasonDependencyFailed — a required external recovery dependency
	// could not be verified; the restore stopped before mutating anything.
	ReasonDependencyFailed = "dependency-failed"
	// ReasonRolledBack — the restore failed and the pre-restore
	// configuration was restored.
	ReasonRolledBack = "rolled-back"
	// ReasonRollbackFailed — the restore failed AND its rollback failed.
	// The running configuration is neither the old nor the new one.
	ReasonRollbackFailed = "rollback-failed"
	// ReasonNotPersisted — a committed restore applied but its
	// write-through failed: the applied configuration is not durable.
	ReasonNotPersisted = "not-persisted"
	// ReasonChecksumMismatch — a downloaded snapshot document does not
	// match its own checksum. Nothing is written to the destination.
	ReasonChecksumMismatch = "checksum-mismatch"
	// ReasonContractLegacy — the gateway answered with the pre-durability
	// body and --strict was requested.
	ReasonContractLegacy = "contract-legacy"
)

// LifecycleError is a config-lifecycle failure carrying a stable reason code.
// Commands return it so the exit status, the human message and the --output
// json envelope all describe the same failure.
type LifecycleError struct {
	Reason     string
	Message    string
	HTTPStatus int
	// Body is the raw response body, kept for diagnostics when the
	// failure is precisely that the body could not be understood.
	Body string
}

func (e *LifecycleError) Error() string {
	if e.HTTPStatus != 0 {
		return fmt.Sprintf("%s (%s, HTTP %d)", e.Message, e.Reason, e.HTTPStatus)
	}
	return fmt.Sprintf("%s (%s)", e.Message, e.Reason)
}

// ReasonOf returns the stable reason code of any error a lifecycle command
// produces. Errors from outside this package (transport failures, for
// instance) map to ReasonRequestFailed rather than to an empty string, so the
// automation surface never reports a failure without a code.
func ReasonOf(err error) string {
	if err == nil {
		return ReasonOK
	}
	if le, ok := err.(*LifecycleError); ok {
		return le.Reason
	}
	return ReasonRequestFailed
}

// HTTPStatusOf returns the HTTP status a lifecycle error carries, or 0.
func HTTPStatusOf(err error) int {
	if le, ok := err.(*LifecycleError); ok {
		return le.HTTPStatus
	}
	return 0
}

// reasonForStatus maps a non-2xx status the gateway returned without a
// lifecycle body onto a stable reason code. The lifecycle endpoints document
// 401/409/503 explicitly; everything else splits on the status class.
func reasonForStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return ReasonUnauthorized
	case http.StatusConflict:
		return ReasonBusy
	case http.StatusServiceUnavailable:
		return ReasonMaintenance
	}
	if status >= 500 {
		return ReasonServerError
	}
	return ReasonBadRequest
}

// NewStatusError builds a LifecycleError from a non-2xx response that carried
// an error envelope rather than a lifecycle result body.
func NewStatusError(status int, body []byte) *LifecycleError {
	return &LifecycleError{
		Reason:     reasonForStatus(status),
		Message:    NewAPIError(status, body).Error(),
		HTTPStatus: status,
		Body:       string(body),
	}
}

// SnapshotFileResult is the identity of a snapshot document the CLI
// downloaded — what was verified, and where (if anywhere) it was stored.
type SnapshotFileResult struct {
	Path          string `json:"path,omitempty"`
	Bytes         int    `json:"bytes"`
	Checksum      string `json:"checksum,omitempty"`
	SchemaVersion string `json:"schema_version,omitempty"`
	Generation    uint64 `json:"generation,omitempty"`
	// ChecksumVerified is false only for a document that carries no
	// checksum at all (a gateway older than the checksummed document
	// format). A document that carries one and fails verification is an
	// error, never a report.
	ChecksumVerified bool `json:"checksum_verified"`
}

// Note is a condition a lifecycle command recorded instead of claiming
// something the gateway never reported. Code is a stable identifier for the
// condition class; Message is the human sentence.
type Note struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// LifecycleReport is the data payload of the CommandResult envelope
// (contracts/command-result.schema.json) for the configuration-lifecycle
// commands. The verdict itself — command, success, code, message — lives on
// the envelope; this document carries what the operation observed, and on a
// failure the origin/httpStatus/componentCode triple the schema requires so
// automation never parses message strings.
type LifecycleReport struct {
	Contract LifecycleContract   `json:"contract,omitempty"`
	Notes    []Note              `json:"-"`
	Persist  *PersistResult      `json:"persist,omitempty"`
	Restore  *RestoreResult      `json:"restore,omitempty"`
	Snapshot *SnapshotFileResult `json:"snapshot,omitempty"`
	// Maintenance carries the gateway's maintenance state as reported by
	// GET/PUT /maintenance. On a recovery-required failure it is absent:
	// the CLI has no state it can honestly report.
	Maintenance *MaintenanceState `json:"maintenance,omitempty"`
	// Origin, HTTPStatus and ComponentCode are the failure triple of the
	// envelope's data contract: present on every failure (pointers so a
	// success omits them entirely), absent on success. ComponentCode is
	// the downstream component's own stable code — for lifecycle failures,
	// the LifecycleError reason — verbatim.
	Origin        *string `json:"origin,omitempty"`
	HTTPStatus    *int    `json:"httpStatus,omitempty"`
	ComponentCode *string `json:"componentCode,omitempty"`
}
