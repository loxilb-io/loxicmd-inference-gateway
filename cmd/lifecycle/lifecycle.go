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
// reported. They are carried in the JSON envelope and printed after the human
// summary.
const (
	legacyContractNote = "this gateway answers with the older response contract; " +
		"the document's identity and coverage were not reported and are not claimed here"
	uncheckedDocumentNote = "this document carries no checksum, so its integrity was not verified"
)

// noteLegacy records the note for a report whose gateway spoke the older
// contract, and returns the report so callers can chain.
func noteLegacy(report *api.LifecycleReport, note string) *api.LifecycleReport {
	if report.Contract == api.ContractLegacy {
		report.Notes = append(report.Notes, note)
	}
	return report
}

// writeNotes prints whatever the operation refused to claim.
func writeNotes(out io.Writer, report *api.LifecycleReport) {
	for _, note := range report.Notes {
		fmt.Fprintf(out, "  Note: %s\n", note)
	}
}

// render emits the outcome of a lifecycle operation and returns err unchanged,
// so the caller's only job is to hand the error back to cobra for the exit
// status. Failures go to errOut as prose, or to out as the JSON envelope --
// automation asking for JSON gets a machine-readable failure, not prose on a
// stream it is not reading.
func render(out, errOut io.Writer, o Options, report *api.LifecycleReport, human func(io.Writer, *api.LifecycleReport) error, err error) error {
	if err != nil {
		report.Result = "error"
		report.Reason = api.ReasonOf(err)
		report.Message = err.Error()
		report.HTTPStatus = api.HTTPStatusOf(err)
		if o.JSON {
			_ = api.WriteLifecycleReport(out, report)
		} else {
			fmt.Fprintf(errOut, "Error: %s\n", err.Error())
		}
		return err
	}
	report.Result = "ok"
	report.Reason = api.ReasonOK
	if o.JSON {
		return api.WriteLifecycleReport(out, report)
	}
	return human(out, report)
}
