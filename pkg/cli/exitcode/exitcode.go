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

// Package exitcode implements the frozen exit-code taxonomy of
// contracts/exit-codes.md. Commands return a *CLIError (directly or by
// classifying a lower-layer error) and the root command's single exit
// point turns it into the process status — no command calls os.Exit for
// a failure itself, so the exit code and the reported failure can never
// disagree.
package exitcode

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// Code is one row of the frozen taxonomy table. The numbers are part of
// the released contract: never renumbered, never reused.
type Code int

const (
	// OK — success. Exit 1 is deliberately absent: it is reserved as the
	// unclassified legacy failure code, so automation seeing 1 knows it
	// is talking to a pre-taxonomy binary.
	OK Code = 0
	// InvalidArgument — invalid command, argument, or flag combination.
	InvalidArgument Code = 2
	// Auth — authentication failed or OS privilege insufficient.
	Auth Code = 3
	// Precondition — a required precondition is not met (state,
	// readiness, missing file, absent target).
	Precondition Code = 4
	// Unavailable — gateway, OAM, or host backend unreachable or
	// refusing service.
	Unavailable Code = 5
	// ContractMismatch — schema, version, or validation mismatch
	// between this CLI and its peer.
	ContractMismatch Code = 6
	// Failed — the operation failed and no state change is confirmed.
	Failed Code = 7
	// Partial — partial apply or recovery required; must never be
	// retried blindly.
	Partial Code = 8
)

// Label returns the envelope `code` string for the taxonomy row.
func (c Code) Label() string {
	switch c {
	case OK:
		return "OK"
	case InvalidArgument:
		return "INVALID_ARGUMENT"
	case Auth:
		return "AUTH"
	case Precondition:
		return "PRECONDITION"
	case Unavailable:
		return "UNAVAILABLE"
	case ContractMismatch:
		return "CONTRACT_MISMATCH"
	case Failed:
		return "FAILED"
	case Partial:
		return "PARTIAL"
	}
	return "FAILED"
}

// CLIError is a classified failure. It carries the downstream origin
// verbatim (contract rule: origin is never flattened into prose) so the
// envelope can expose data.origin/httpStatus/componentCode for automation
// to branch on.
type CLIError struct {
	Code Code
	// Message is the single human line the exit point prints to stderr.
	Message string
	// Origin names the component the failure came from ("gateway",
	// "cli", ...); empty when the CLI itself is the origin.
	Origin string
	// HTTPStatus is the downstream HTTP status when one exists.
	HTTPStatus int
	// ComponentCode is the downstream component's own stable code
	// (e.g. a LifecycleError reason), verbatim.
	ComponentCode string
	// ShowHelp asks the exit point to print the command's help to
	// stderr (the missing-argument case; contract rule 2).
	ShowHelp bool
}

func (e *CLIError) Error() string { return e.Message }

// Usagef builds the invalid-invocation failure (exit 2) with help
// requested on stderr — for arguments that are missing or unparseable
// before any request is built.
func Usagef(format string, args ...any) *CLIError {
	return &CLIError{Code: InvalidArgument, Message: fmt.Sprintf(format, args...), ShowHelp: true}
}

// Invalidf builds the invalid-input failure (exit 2) without a help dump
// — the invocation shape was fine, a value was not.
func Invalidf(format string, args ...any) *CLIError {
	return &CLIError{Code: InvalidArgument, Message: fmt.Sprintf(format, args...)}
}

// Unavailablef builds the transport failure (exit 5): the peer could not
// be reached at all, so no state change is confirmed and bounded-backoff
// retry is safe.
func Unavailablef(format string, args ...any) *CLIError {
	return &CLIError{Code: Unavailable, Message: fmt.Sprintf(format, args...), Origin: "gateway"}
}

// FromHTTPStatus classifies a non-2xx gateway answer for a simple
// (request/answer) command. op names the operation for the human line.
//
// The mapping follows the contract's most-specific-wins rule:
// 401/403 are the credential rows; 404 and 409 are state preconditions
// (the target is absent, or the current state refuses the transition);
// 429 and 503 are the peer refusing service right now; 400 means the
// peer's validation rejected a request this CLI built — a contract
// mismatch between the two, not a transport fault; everything else is a
// classified failure with no better category.
func FromHTTPStatus(op string, status int) *CLIError {
	e := &CLIError{Message: fmt.Sprintf("%s: gateway answered HTTP %d", op, status), Origin: "gateway", HTTPStatus: status}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		e.Code = Auth
	case http.StatusNotFound, http.StatusConflict:
		e.Code = Precondition
	case http.StatusTooManyRequests, http.StatusServiceUnavailable:
		e.Code = Unavailable
	case http.StatusBadRequest:
		e.Code = ContractMismatch
	default:
		e.Code = Failed
	}
	return e
}

// reasonCodes maps every stable LifecycleError reason to its taxonomy
// row. Keys are the wire strings, not the api constants, so reasons a
// newer gateway/CLI slice introduces (recovery-required, not-ready)
// classify correctly the day they appear.
var reasonCodes = map[string]Code{
	api.ReasonOK:               OK,
	api.ReasonInvalidArguments: InvalidArgument,
	// An input file that cannot be read is a missing precondition; a
	// file this CLI failed to write is a failed operation.
	api.ReasonFileRead:  Precondition,
	api.ReasonFileWrite: Failed,
	// The local input file was not valid JSON: correct the input.
	api.ReasonInvalidJSON:   InvalidArgument,
	api.ReasonRequestFailed: Unavailable,
	api.ReasonUnauthorized:  Auth,
	// Busy and maintenance are the peer refusing service and a state
	// precondition respectively — retry semantics differ (backoff vs
	// check state first), which is exactly the 5/4 split.
	api.ReasonBusy:        Unavailable,
	api.ReasonMaintenance: Precondition,
	api.ReasonServerError: Failed,
	// The peer's validation rejected the request, or its answer could
	// not be decoded: the two directions of a contract mismatch.
	api.ReasonBadRequest:   ContractMismatch,
	api.ReasonDecodeFailed: ContractMismatch,
	api.ReasonResultNotOK:  Failed,
	api.ReasonIncompatible: ContractMismatch,
	// Gateway-side validation refused the document before anything was
	// planned or wiped.
	api.ReasonValidationFailed: ContractMismatch,
	// A declared external dependency was missing or divergent: a state
	// precondition, addressed then retried.
	api.ReasonDependencyFailed: Precondition,
	// Rolled back means the pre-state was restored: the operation
	// failed with no net state change. A failed rollback means state is
	// partially applied and must be recovered, never retried blindly.
	api.ReasonRolledBack:     Failed,
	api.ReasonRollbackFailed: Partial,
	// Applied but not durable: a state change happened and its
	// durability did not — the definition of partial.
	api.ReasonNotPersisted:     Partial,
	api.ReasonChecksumMismatch: ContractMismatch,
	api.ReasonContractLegacy:   ContractMismatch,
	// Wire literals from command slices that ship separately from this
	// package (see the key-string rationale above).
	"recovery-required": Partial,
	"not-ready":         Precondition,
}

// cobraUsageShapes are the argument/flag parse failures cobra returns
// from Execute before any RunE runs. Cobra offers no typed error for
// them, so the shapes are matched; every one is an invalid invocation.
var cobraUsageShapes = []string{
	"unknown command ",
	"unknown flag",
	"unknown shorthand flag",
	"flag needs an argument",
	"required flag",
	"invalid argument ",
	"accepts ",
	"requires at least ",
	"requires exactly ",
	"if any flags in the group",
	"were all set",
	"none of the others can be",
}

// Classify turns any error into its taxonomy row. Classified errors pass
// through; LifecycleErrors map by their stable reason with the origin
// preserved; cobra's own parse failures are invalid invocations; every
// other error is a classified failure with no better category (exit 7,
// per the contract's fallback rule — never the legacy 1).
func Classify(err error) *CLIError {
	var ce *CLIError
	if errors.As(err, &ce) {
		return ce
	}
	var le *api.LifecycleError
	if errors.As(err, &le) {
		code, known := reasonCodes[le.Reason]
		if !known {
			code = Failed
		}
		return &CLIError{
			Code:          code,
			Message:       le.Error(),
			Origin:        "gateway",
			HTTPStatus:    le.HTTPStatus,
			ComponentCode: le.Reason,
		}
	}
	msg := err.Error()
	for _, shape := range cobraUsageShapes {
		if strings.Contains(msg, shape) {
			return &CLIError{Code: InvalidArgument, Message: msg, ShowHelp: true}
		}
	}
	return &CLIError{Code: Failed, Message: msg}
}
