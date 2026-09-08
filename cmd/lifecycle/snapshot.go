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
package lifecycle

import (
	"fmt"
	"io"
	"net/http"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// snapshotChecksumHeader is the response header the gateway stamps with the
// document's checksum, so a client can cross-check the body it received
// against what the server says it sent.
const snapshotChecksumHeader = "X-Snapshot-Checksum"

// SnapshotOptions are the snapshot-download inputs.
type SnapshotOptions struct {
	// Components limits the capture to a subset of the v1 domains.
	Components string
	// File, when set, stores the document instead of printing it.
	File string
}

// Snapshot downloads a configuration snapshot document
// (GET /config/snapshot), verifies it against its own checksum, and either
// prints it or stores it.
//
// Verification happens before anything durable: a corrupt or truncated
// download must not overwrite the last good snapshot sitting at the
// destination path.
func Snapshot(restOptions *api.RESTOptions, out io.Writer, o Options, so SnapshotOptions) error {
	document, result, err := doSnapshot(restOptions, o, so)
	report := &api.LifecycleReport{Snapshot: result}
	if result != nil {
		if result.ChecksumVerified {
			report.Contract = api.ContractDurable
		} else {
			report.Contract = api.ContractLegacy
		}
		noteLegacy(report, uncheckedDocumentNote)
	}
	if err != nil {
		return render(out, o, "get.snapshot", report, nil, err)
	}
	// Printing the document to stdout and reporting on it are mutually
	// exclusive: a caller redirecting stdout into a file must receive the
	// document there, not a report about it.
	if so.File == "" && !o.JSON {
		_, werr := out.Write(append(document, '\n'))
		return werr
	}
	return render(out, o, "get.snapshot", report, humanSnapshot, nil)
}

func doSnapshot(restOptions *api.RESTOptions, o Options, so SnapshotOptions) ([]byte, *api.SnapshotFileResult, error) {
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	snap := &client.Snapshot().CommonAPI
	if so.Components != "" {
		snap = snap.Query(map[string]string{"components": so.Components})
	}
	resp, err := snap.Get(ctx)
	if err != nil {
		return nil, nil, transportError("snapshot request failed", err)
	}
	defer resp.Body.Close()

	body, err := readBody(resp.Body, "snapshot")
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, api.NewStatusError(resp.StatusCode, body)
	}

	document, result, err := api.VerifySnapshotChecksum(body, resp.Header.Get(snapshotChecksumHeader))
	if err != nil {
		return nil, nil, err
	}
	if o.Strict && !result.ChecksumVerified {
		return nil, result, &api.LifecycleError{
			Reason:  api.ReasonContractLegacy,
			Message: "gateway returned a snapshot document without a checksum, so its integrity cannot be verified (--strict requires one)",
		}
	}
	if so.File != "" {
		if err := api.WriteSnapshotFile(so.File, document); err != nil {
			return nil, result, err
		}
		result.Path = so.File
	}
	return document, result, nil
}

func humanSnapshot(out io.Writer, report *api.LifecycleReport) error {
	res := report.Snapshot
	if res == nil {
		return nil
	}
	if res.Path != "" {
		fmt.Fprintf(out, "Snapshot written to %s (%d bytes).\n", res.Path, res.Bytes)
	} else {
		fmt.Fprintf(out, "Snapshot downloaded (%d bytes).\n", res.Bytes)
	}
	if res.SchemaVersion != "" {
		fmt.Fprintf(out, "  Document: schema %s", res.SchemaVersion)
		if res.Generation != 0 {
			fmt.Fprintf(out, ", generation %d", res.Generation)
		}
		fmt.Fprintln(out)
	}
	if res.ChecksumVerified {
		fmt.Fprintf(out, "  Checksum verified: %s\n", res.Checksum)
	}
	writeNotes(out, report)
	return nil
}
