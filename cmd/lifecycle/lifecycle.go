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

// Package lifecycle holds the configuration-lifecycle operations shared by the
// commands that drive them: 'create persist', 'create restore', 'get snapshot'
// and the 'save --api' compatibility alias.
//
// The operations live here rather than in the cobra command files for two
// reasons. First, 'save --api' must be a true alias of 'create persist' -- the
// same request, the same verdict, the same exit status -- and the surest way
// to keep two commands identical is to give them one implementation. Second,
// these functions are plain io.Writer/error functions, so every failure class
// is reachable from a test against a fake gateway rather than only from a live
// one.
package lifecycle

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/envelope"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
)

// Options are the parts of a lifecycle invocation that every operation shares.
type Options struct {
	// JSON selects the stable --output json envelope instead of prose.
	JSON bool
	// Strict refuses a gateway that answers with the pre-durability
	// contract, so automation never records a coverage claim the gateway
	// did not actually make.
	Strict bool
}

// OptionsFrom derives the shared options from the global REST options.
func OptionsFrom(restOptions *api.RESTOptions, strict bool) Options {
	return Options{JSON: restOptions.PrintOption == "json", Strict: strict}
}

// requestContext honors the global --timeout for a lifecycle call.
func requestContext(restOptions *api.RESTOptions) (context.Context, context.CancelFunc) {
	if restOptions.Timeout > 0 {
		return context.WithTimeout(context.Background(), time.Duration(restOptions.Timeout)*time.Second)
	}
	return context.Background(), func() {}
}

// transportError wraps a failure that never produced an HTTP response.
func transportError(what string, err error) *api.LifecycleError {
	return &api.LifecycleError{
		Reason:  api.ReasonRequestFailed,
		Message: fmt.Sprintf("%s: %v", what, err),
	}
}

// readBody reads a response body in full. A truncated read is a failure in its
// own right: the bytes that did arrive are not the answer the gateway sent.
func readBody(r io.Reader, what string) ([]byte, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, transportError("cannot read the "+what+" response", err)
	}
	return body, nil
}

// Notes a command records instead of claiming something the gateway never
// reported. They surface as envelope warnings under -o json and are printed
// after the human summary otherwise.
//
// The codes are a stable surface -- they identify the condition class, never
// the wording -- and a schema-constrained one: contracts/command-result.schema.json
// pins warnings[].code to ^[A-Z][A-Z0-9_]*$. They are therefore spelled in that
// shape, NOT in the lowercase-hyphen shape of the lifecycle reason codes
// (api.Reason*), which travel in data.componentCode: a different field, with a
// different contract, documented in docs/COMMANDS.md. The two happening to
// describe the same condition does not make them the same string.
var (
	legacyContractNote = api.Note{
		Code: "CONTRACT_LEGACY",
		Message: "this gateway answers with the older response contract; " +
			"the document's identity and coverage were not reported and are not claimed here",
	}
	uncheckedDocumentNote = api.Note{
		Code:    "UNCHECKED_DOCUMENT",
		Message: "this document carries no checksum, so its integrity was not verified",
	}
)

// noteLegacy records the note for a report whose gateway spoke the older
// contract, and returns the report so callers can chain.
func noteLegacy(report *api.LifecycleReport, note api.Note) *api.LifecycleReport {
	if report.Contract == api.ContractLegacy {
		report.Notes = append(report.Notes, note)
	}
	return report
}

// writeNotes prints whatever the operation refused to claim.
func writeNotes(out io.Writer, report *api.LifecycleReport) {
	for _, note := range report.Notes {
		fmt.Fprintf(out, "  Note: %s\n", note.Message)
	}
}

// envelopeFor builds the CommandResult document for a lifecycle outcome: the
// report becomes the data payload, notes become warnings, and on failure the
// classified verdict fills code/message plus the data failure triple. The
// correlationId carries the operation id the gateway reported, when it
// reported one.
func envelopeFor(command string, report *api.LifecycleReport, err error) *envelope.CommandResult {
	result := envelope.New(command)
	if report.Maintenance != nil {
		result.CorrelationID = report.Maintenance.OperationID
	}
	for _, note := range report.Notes {
		result.Warnings = append(result.Warnings, envelope.Warning{Code: note.Code, Message: note.Message})
	}
	if err != nil {
		ce := exitcode.Classify(err)
		result.Fail(envelope.Code(ce.Code.Label()), ce.Message)
		origin := ce.Origin
		if origin == "" {
			origin = "cli"
		}
		httpStatus, componentCode := ce.HTTPStatus, ce.ComponentCode
		report.Origin, report.HTTPStatus, report.ComponentCode = &origin, &httpStatus, &componentCode
	}
	result.Data = report
	return result
}

// render emits the outcome of a lifecycle operation and returns err unchanged,
// so the caller's only job is to hand the error back to cobra for the exit
// status. Under -o json the outcome is the CommandResult envelope on stdout —
// success and failure alike, so automation never parses prose. A human-mode
// failure prints nothing here: the root command's single exit point owns the
// one stderr line, so the message can never appear twice.
func render(out io.Writer, o Options, command string, report *api.LifecycleReport, human func(io.Writer, *api.LifecycleReport) error, err error) error {
	if o.JSON {
		if werr := envelopeFor(command, report, err).Write(out); err == nil {
			return werr
		}
		return err
	}
	if err != nil {
		return err
	}
	return human(out, report)
}

// WriteFailure emits the CommandResult failure envelope for a lifecycle
// command that failed before any operation ran (argument validation). It
// exists for callers outside this package that share the lifecycle contract —
// the 'save' compatibility surface.
func WriteFailure(out io.Writer, command string, err error) {
	_ = envelopeFor(command, &api.LifecycleReport{}, err).Write(out)
}
