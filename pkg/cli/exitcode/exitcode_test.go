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

package exitcode

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/envelope"
)

// everyReason is the complete stable-reason surface of the api package.
// A reason added there without a taxonomy decision here must fail this
// suite: an unmapped reason silently classifying as 7 is exactly the
// "catch-all for laziness" the contract forbids.
var everyReason = []string{
	api.ReasonOK, api.ReasonInvalidArguments, api.ReasonFileRead,
	api.ReasonFileWrite, api.ReasonInvalidJSON, api.ReasonRequestFailed,
	api.ReasonUnauthorized, api.ReasonBusy, api.ReasonMaintenance,
	api.ReasonServerError, api.ReasonBadRequest, api.ReasonDecodeFailed,
	api.ReasonResultNotOK, api.ReasonIncompatible, api.ReasonValidationFailed,
	api.ReasonDependencyFailed, api.ReasonRolledBack, api.ReasonRollbackFailed,
	api.ReasonNotPersisted, api.ReasonChecksumMismatch, api.ReasonContractLegacy,
}

func TestEveryLifecycleReasonHasADeliberateRow(t *testing.T) {
	for _, reason := range everyReason {
		if _, ok := reasonCodes[reason]; !ok {
			t.Errorf("reason %q has no taxonomy decision", reason)
		}
	}
}

func TestClassifyLifecycleErrorPreservesOrigin(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &api.LifecycleError{
		Reason: api.ReasonRollbackFailed, Message: "rollback failed", HTTPStatus: 500,
	})
	ce := Classify(err)
	if ce.Code != Partial {
		t.Fatalf("rollback-failed must be PARTIAL(8), got %d", ce.Code)
	}
	if ce.Origin != "gateway" || ce.HTTPStatus != 500 || ce.ComponentCode != api.ReasonRollbackFailed {
		t.Fatalf("origin fields flattened: %+v", ce)
	}
}

func TestClassifySpotChecks(t *testing.T) {
	for reason, want := range map[string]Code{
		api.ReasonMaintenance:  Precondition,
		api.ReasonBusy:         Unavailable,
		api.ReasonUnauthorized: Auth,
		api.ReasonNotPersisted: Partial,
		"recovery-required":    Partial,
		"not-ready":            Precondition,
	} {
		ce := Classify(&api.LifecycleError{Reason: reason, Message: reason})
		if ce.Code != want {
			t.Errorf("reason %q -> %d, want %d", reason, ce.Code, want)
		}
	}
}

func TestClassifyUnknownErrorIsFailedNeverLegacy(t *testing.T) {
	ce := Classify(errors.New("something nobody classified"))
	if ce.Code != Failed {
		t.Fatalf("unknown error must be FAILED(7), got %d", ce.Code)
	}
}

func TestClassifyCobraUsageShapes(t *testing.T) {
	for _, msg := range []string{
		`unknown command "delte" for "loxicmd"`,
		"unknown flag: --frobnicate",
		"accepts 1 arg(s), received 0",
	} {
		ce := Classify(errors.New(msg))
		if ce.Code != InvalidArgument || !ce.ShowHelp {
			t.Errorf("%q -> %+v, want INVALID_ARGUMENT with help", msg, ce)
		}
	}
}

func TestClassifyPassesThroughCLIError(t *testing.T) {
	in := Usagef("missing <Vid>")
	if got := Classify(fmt.Errorf("wrap: %w", in)); got != in {
		t.Fatalf("classified error must pass through unchanged")
	}
}

func TestFromHTTPStatusTable(t *testing.T) {
	for status, want := range map[int]Code{
		http.StatusUnauthorized:        Auth,
		http.StatusForbidden:           Auth,
		http.StatusNotFound:            Precondition,
		http.StatusConflict:            Precondition,
		http.StatusTooManyRequests:     Unavailable,
		http.StatusServiceUnavailable:  Unavailable,
		http.StatusBadRequest:          ContractMismatch,
		http.StatusInternalServerError: Failed,
	} {
		if got := FromHTTPStatus("op", status).Code; got != want {
			t.Errorf("HTTP %d -> %d, want %d", status, got, want)
		}
	}
}

func TestLabelsMatchTheContractTable(t *testing.T) {
	for code, want := range map[Code]string{
		OK: "OK", InvalidArgument: "INVALID_ARGUMENT", Auth: "AUTH",
		Precondition: "PRECONDITION", Unavailable: "UNAVAILABLE",
		ContractMismatch: "CONTRACT_MISMATCH", Failed: "FAILED", Partial: "PARTIAL",
	} {
		if code.Label() != want {
			t.Errorf("Code %d label %q, want %q", code, code.Label(), want)
		}
	}
}

// TestEnvelopeCodeAgreement proves the two halves of the public contract can
// never disagree: for every taxonomy row, the envelope code string derived
// from it maps back to the same process exit status. A row added to one
// enum without the other fails here.
func TestEnvelopeCodeAgreement(t *testing.T) {
	for _, code := range []Code{OK, InvalidArgument, Auth, Precondition,
		Unavailable, ContractMismatch, Failed, Partial} {
		if got := envelope.Code(code.Label()).ExitCode(); got != int(code) {
			t.Errorf("envelope code %q exits %d, exit taxonomy says %d", code.Label(), got, int(code))
		}
	}
	// An unknown envelope code exits with the reserved legacy 1, never a
	// stolen taxonomy slot.
	if got := envelope.Code("NOT_A_CODE").ExitCode(); got != 1 {
		t.Errorf("unknown envelope code exits %d, want the reserved 1", got)
	}
}
