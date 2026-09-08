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

// Package envelope is the Go form of contracts/command-result.schema.json:
// the single JSON document every command writes to stdout under -o json.
// The schema file is the contract; this package must never marshal a
// document the schema rejects, which is what its tests prove.
package envelope

import (
	"encoding/json"
	"fmt"
	"io"
)

// APIVersion and Kind identify the envelope contract. A breaking envelope
// change requires a new APIVersion; consumers reject an unknown major.
const (
	APIVersion = "loxilb.io/appliance/v1"
	Kind       = "CommandResult"
)

// Code is the machine verdict of an invocation. Codes correspond one-to-one
// with the process exit codes frozen in contracts/exit-codes.md, so the
// envelope and the exit status can never disagree.
type Code string

const (
	OK               Code = "OK"                // exit 0
	InvalidArgument  Code = "INVALID_ARGUMENT"  // exit 2
	Auth             Code = "AUTH"              // exit 3
	Precondition     Code = "PRECONDITION"      // exit 4
	Unavailable      Code = "UNAVAILABLE"       // exit 5
	ContractMismatch Code = "CONTRACT_MISMATCH" // exit 6
	Failed           Code = "FAILED"            // exit 7
	Partial          Code = "PARTIAL"           // exit 8
)

// ExitCode returns the process exit status frozen for this verdict. An
// unknown Code maps to the reserved legacy value 1 so a programming error
// surfaces as "pre-taxonomy binary" to automation rather than as a false
// success or a stolen taxonomy slot.
func (c Code) ExitCode() int {
	switch c {
	case OK:
		return 0
	case InvalidArgument:
		return 2
	case Auth:
		return 3
	case Precondition:
		return 4
	case Unavailable:
		return 5
	case ContractMismatch:
		return 6
	case Failed:
		return 7
	case Partial:
		return 8
	}
	return 1
}

// Warning is a condition that did not change the verdict but that an
// operator or automation should see.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// CommandResult is the envelope document. Every key is always present in the
// marshaled form: empty values are "", {} or [], never null — which is why
// Data and Warnings must stay non-nil (New takes care of it, and Write
// refuses a document where a later assignment broke the invariant).
type CommandResult struct {
	APIVersion    string    `json:"apiVersion"`
	Kind          string    `json:"kind"`
	Command       string    `json:"command"`
	Success       bool      `json:"success"`
	Code          Code      `json:"code"`
	Message       string    `json:"message"`
	CorrelationID string    `json:"correlationId"`
	Data          any       `json:"data"`
	Warnings      []Warning `json:"warnings"`
}

// New starts a successful envelope for the given dotted command path
// (e.g. "version", "appliance.status"). Callers replace Data with their
// command-specific payload and downgrade Code/Success on failure.
func New(command string) *CommandResult {
	return &CommandResult{
		APIVersion: APIVersion,
		Kind:       Kind,
		Command:    command,
		Success:    true,
		Code:       OK,
		Data:       map[string]any{},
		Warnings:   []Warning{},
	}
}

// Fail records a failure verdict in place and returns the envelope. Success
// is derived from the code rather than set independently, so the two cannot
// drift apart.
func (r *CommandResult) Fail(code Code, message string) *CommandResult {
	r.Code = code
	r.Success = code == OK
	r.Message = message
	return r
}

// Write emits the envelope as exactly one JSON document followed by one
// newline. It restores the always-present invariant for Data and Warnings
// before marshaling so a nil assigned by a caller can never surface as null
// on the wire.
func (r *CommandResult) Write(w io.Writer) error {
	if r.Data == nil {
		r.Data = map[string]any{}
	}
	if r.Warnings == nil {
		r.Warnings = []Warning{}
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal command result: %w", err)
	}
	if _, err := w.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("write command result: %w", err)
	}
	return nil
}
