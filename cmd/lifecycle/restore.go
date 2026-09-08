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
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// RestoreOptions are the restore-specific inputs.
type RestoreOptions struct {
	// File is the client-local snapshot document to post.
	File string
	// Commit applies the document; the default is a dry-run that mutates
	// nothing.
	Commit bool
	// Components limits the restore to a subset of the document's declared
	// domains (default: everything the document covers).
	Components string
}

// Restore runs the staged restore pipeline on a snapshot document
// (POST /config/restore) and reports the outcome.
func Restore(restOptions *api.RESTOptions, out io.Writer, o Options, ro RestoreOptions) error {
	result, err := doRestore(restOptions, o, ro)
	report := &api.LifecycleReport{Restore: result}
	if result != nil {
		report.Contract = result.Capabilities()
		noteLegacy(report, legacyContractNote)
	}
	return render(out, o, "create.restore", report, humanRestore, err)
}

func doRestore(restOptions *api.RESTOptions, o Options, ro RestoreOptions) (*api.RestoreResult, error) {
	if ro.File == "" {
		return nil, &api.LifecycleError{
			Reason:  api.ReasonInvalidArguments,
			Message: "-f/--file is required (the snapshot document to restore)",
		}
	}
	data, err := os.ReadFile(ro.File)
	if err != nil {
		return nil, &api.LifecycleError{
			Reason:  api.ReasonFileRead,
			Message: fmt.Sprintf("cannot read the snapshot document: %v", err),
		}
	}
	if !json.Valid(data) {
		return nil, &api.LifecycleError{
			Reason:  api.ReasonInvalidJSON,
			Message: fmt.Sprintf("%s is not valid JSON", ro.File),
		}
	}

	mode := api.RestoreModeDryRun
	if ro.Commit {
		mode = api.RestoreModeCommit
	}
	query := map[string]string{"mode": mode}
	if ro.Components != "" {
		query["components"] = ro.Components
	}

	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.Restore().Query(query).Create(ctx, json.RawMessage(data))
	if err != nil {
		return nil, transportError("restore request failed", err)
	}
	defer resp.Body.Close()

	body, err := readBody(resp.Body, "restore")
	if err != nil {
		return nil, err
	}

	// The restore endpoint answers 200, 400 and 500 with a RestoreResult,
	// so the status code alone never decides the verdict. The other
	// documented statuses (401, 409, 503) carry an error envelope instead,
	// and so does a 400 that never reached the pipeline -- hence the
	// shape check rather than a status switch.
	result, decodeErr := api.DecodeRestoreResult(body)
	if decodeErr != nil || !looksLikeRestoreResult(result) {
		if resp.StatusCode/100 != 2 {
			return nil, api.NewStatusError(resp.StatusCode, body)
		}
		if decodeErr != nil {
			return nil, decodeErr
		}
		return nil, &api.LifecycleError{
			Reason:  api.ReasonDecodeFailed,
			Message: "gateway returned a body that is not a restore result",
			Body:    string(body),
		}
	}
	// The mode the CLI asked for is authoritative for the verdict: a
	// gateway that omits the field must still be judged by commit rules
	// when a commit was requested.
	if result.Mode == "" {
		result.Mode = mode
	}
	verdict := result.Verdict(o.Strict)
	if verdict == nil && resp.StatusCode/100 != 2 {
		// A non-2xx whose body reports no failure is itself a contract
		// violation -- report the status rather than call it success.
		return result, api.NewStatusError(resp.StatusCode, body)
	}
	if le, ok := verdict.(*api.LifecycleError); ok && le.HTTPStatus == 0 {
		le.HTTPStatus = resp.StatusCode
	}
	return result, verdict
}

// looksLikeRestoreResult reports whether a decoded body actually came from the
// restore pipeline. json.Unmarshal happily decodes an unrelated object into an
// all-zero struct, and an all-zero struct must never be read as a clean
// dry-run.
func looksLikeRestoreResult(r *api.RestoreResult) bool {
	if r == nil {
		return false
	}
	return r.Mode != "" || r.Result != "" || r.SchemaVersion != "" ||
		r.Compatible != nil || len(r.Plan) > 0 || len(r.Errors) > 0
}

func humanRestore(out io.Writer, report *api.LifecycleReport) error {
	res := report.Restore
	if res == nil {
		_, err := fmt.Fprintln(out, "Restore completed.")
		return err
	}
	switch res.Mode {
	case api.RestoreModeCommit:
		fmt.Fprintln(out, "Restore committed.")
	default:
		fmt.Fprintln(out, "Restore dry-run: nothing was changed.")
	}
	if res.SchemaVersion != "" {
		fmt.Fprintf(out, "  Document: schema %s", res.SchemaVersion)
		if res.SnapshotGeneration != nil {
			fmt.Fprintf(out, ", generation %d", *res.SnapshotGeneration)
		}
		if res.SnapshotGatewayVersion != "" {
			fmt.Fprintf(out, ", written by %s", res.SnapshotGatewayVersion)
		}
		fmt.Fprintln(out)
	}
	for _, item := range res.Plan {
		fmt.Fprintf(out, "  Plan %-24s delete %d, apply %d\n", item.Domain, item.ToDelete, item.ToApply)
	}
	for _, dep := range res.ExternalDependencies {
		fmt.Fprintf(out, "  Depends on: %s\n", dep.String())
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(out, "  Warning: %s\n", w)
	}
	if res.Mode == api.RestoreModeCommit {
		if res.Persisted != nil && *res.Persisted {
			fmt.Fprint(out, "  Persisted: yes")
			if res.PersistedGeneration != nil {
				fmt.Fprintf(out, " (generation %d)", *res.PersistedGeneration)
			}
			fmt.Fprintln(out)
		}
		if res.PreRestoreSnapshotPersisted != "" {
			fmt.Fprintf(out, "  Pre-restore snapshot: %s\n", res.PreRestoreSnapshotPersisted)
		}
	}
	writeNotes(out, report)
	return nil
}
