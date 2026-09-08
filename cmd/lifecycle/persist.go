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
	"strings"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// Persist asks the gateway to dump its running configuration to
// {config-path}/snapshot.json (POST /config/persist) and reports what was
// written. command names the invocation in the JSON envelope so the
// compatibility alias is distinguishable from the canonical command.
func Persist(restOptions *api.RESTOptions, out io.Writer, o Options, command string) error {
	result, err := doPersist(restOptions, o)
	report := &api.LifecycleReport{Persist: result}
	if result != nil {
		report.Contract = result.Capabilities()
		noteLegacy(report, legacyContractNote)
	}
	return render(out, o, command, report, humanPersist, err)
}

func doPersist(restOptions *api.RESTOptions, o Options) (*api.PersistResult, error) {
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.Persist().Create(ctx, nil)
	if err != nil {
		return nil, transportError("persist request failed", err)
	}
	defer resp.Body.Close()

	body, err := readBody(resp.Body, "persist")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, api.NewStatusError(resp.StatusCode, body)
	}
	result, err := api.DecodePersistResult(body)
	if err != nil {
		return nil, err
	}
	// The result is returned alongside a failing verdict on purpose: the
	// JSON envelope should carry the body the gateway actually sent, which
	// is the evidence for the verdict.
	return result, result.Verdict(o.Strict)
}

func humanPersist(out io.Writer, report *api.LifecycleReport) error {
	res := report.Persist
	if res == nil {
		_, err := fmt.Fprintln(out, "Configuration persisted.")
		return err
	}
	if res.Path != "" {
		fmt.Fprintf(out, "Configuration persisted to %s", res.Path)
		if res.Checksum != "" {
			fmt.Fprintf(out, " (checksum %s)", res.Checksum)
		}
		fmt.Fprintln(out, ".")
	} else {
		fmt.Fprintln(out, "Configuration persisted.")
	}
	if res.SchemaVersion != "" || res.Generation != nil {
		fmt.Fprintf(out, "  Document: schema %s", orUnreported(res.SchemaVersion))
		if res.Generation != nil {
			fmt.Fprintf(out, ", generation %d", *res.Generation)
		}
		fmt.Fprintln(out)
	}
	if len(res.IncludedDomains) > 0 {
		fmt.Fprintf(out, "  Captured: %s\n", strings.Join(res.IncludedDomains, ", "))
	}
	// Excluded domains are an honesty marker, not noise: they are the
	// configuration a restore of this document will NOT bring back.
	if len(res.ExcludedDomains) > 0 {
		fmt.Fprintf(out, "  Not captured: %s\n", strings.Join(res.ExcludedDomains, ", "))
	}
	for _, dep := range res.ExternalDependencies {
		fmt.Fprintf(out, "  Depends on: %s\n", dep.String())
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(out, "  Warning: %s\n", w)
	}
	writeNotes(out, report)
	return nil
}

func orUnreported(s string) string {
	if s == "" {
		return "unreported"
	}
	return s
}
