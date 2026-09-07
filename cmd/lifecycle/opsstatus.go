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
package lifecycle

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// ReadyGet reads the configuration-readiness verdict (GET /status/ready).
//
// The gateway serves the same body on 200 (ready) and 503 (not ready), so
// both are rendered identically; the exit status carries the verdict. In
// JSON mode the gateway's body is printed verbatim - the contract body IS
// the machine interface here, so no envelope is wrapped around it.
func ReadyGet(restOptions *api.RESTOptions, out, errOut io.Writer, jsonOut bool) error {
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.StatusReady().Get(ctx)
	if err != nil {
		return renderOpsFailure(errOut, transportError("readiness request failed", err))
	}
	defer resp.Body.Close()
	body, err := readBody(resp.Body, "readiness")
	if err != nil {
		return renderOpsFailure(errOut, err)
	}

	var state api.ReadyState
	decodable := json.Unmarshal(body, &state) == nil && state.Ready != nil
	if (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable) || !decodable {
		return renderOpsFailure(errOut, api.NewStatusError(resp.StatusCode, body))
	}

	if jsonOut {
		_, _ = out.Write(append(body, '\n'))
	} else {
		humanReady(out, &state)
	}
	if !*state.Ready {
		// The body was already delivered on stdout; the error exists to
		// carry the non-zero exit and a one-line verdict on stderr.
		return &api.LifecycleError{
			Reason:     api.ReasonResultNotOK,
			Message:    fmt.Sprintf("gateway is not ready (%d reason(s))", len(state.Reasons)),
			HTTPStatus: resp.StatusCode,
		}
	}
	return nil
}

// DiagnosticsGet reads the secret-safe diagnostics assembly
// (GET /diagnostics). JSON mode prints the gateway's body verbatim.
func DiagnosticsGet(restOptions *api.RESTOptions, out, errOut io.Writer, jsonOut bool) error {
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.Diagnostics().Get(ctx)
	if err != nil {
		return renderOpsFailure(errOut, transportError("diagnostics request failed", err))
	}
	defer resp.Body.Close()
	body, err := readBody(resp.Body, "diagnostics")
	if err != nil {
		return renderOpsFailure(errOut, err)
	}
	if resp.StatusCode != http.StatusOK {
		return renderOpsFailure(errOut, api.NewStatusError(resp.StatusCode, body))
	}
	var state api.DiagnosticsState
	if json.Unmarshal(body, &state) != nil || state.Version == "" {
		return renderOpsFailure(errOut, &api.LifecycleError{
			Reason:  api.ReasonDecodeFailed,
			Message: "the gateway's diagnostics response could not be decoded",
			Body:    string(body),
		})
	}
	if jsonOut {
		_, _ = out.Write(append(body, '\n'))
		return nil
	}
	humanDiagnostics(out, &state)
	return nil
}

// renderOpsFailure prints a read failure as prose on stderr and returns
// the error for cobra's exit status. These are reads: there is no
// unknown-state problem and no envelope contract, so prose is the whole
// failure surface.
func renderOpsFailure(errOut io.Writer, err error) error {
	fmt.Fprintf(errOut, "Error: %s\n", err.Error())
	return err
}

func humanAttachments(out io.Writer, attachments []api.EbpfAttachment) {
	for _, a := range attachments {
		state := "attached"
		if !a.Attached {
			state = "NOT ATTACHED"
		}
		fmt.Fprintf(out, "  %s (%s): %s\n", a.Name, a.Mode, state)
	}
}

func humanReady(out io.Writer, st *api.ReadyState) {
	if *st.Ready {
		fmt.Fprintln(out, "Ready: true")
	} else {
		fmt.Fprintln(out, "Ready: FALSE")
		for _, r := range st.Reasons {
			fmt.Fprintf(out, "  Reason: %s\n", r)
		}
	}
	if len(st.EbpfAttachments) > 0 {
		fmt.Fprintln(out, "eBPF attachment (kernel-verified):")
		humanAttachments(out, st.EbpfAttachments)
	}
}

func humanDiagnostics(out io.Writer, st *api.DiagnosticsState) {
	fmt.Fprintf(out, "Gateway: %s", st.Version)
	if st.BuildInfo != "" {
		fmt.Fprintf(out, " (%s)", st.BuildInfo)
	}
	fmt.Fprintln(out)
	if st.APIVersion != "" {
		fmt.Fprintf(out, "  API contract: %s\n", st.APIVersion)
	}
	fmt.Fprintf(out, "  Uptime: %ds\n", st.UptimeSeconds)
	if st.Ready != nil {
		fmt.Fprintf(out, "  Ready: %v\n", *st.Ready)
		for _, r := range st.ReadyReasons {
			fmt.Fprintf(out, "    Reason: %s\n", r)
		}
	}
	fmt.Fprintf(out, "  Maintenance: %s\n", st.MaintenanceState)
	if len(st.EbpfAttachments) > 0 {
		fmt.Fprintln(out, "  eBPF attachment (kernel-verified):")
		for _, a := range st.EbpfAttachments {
			state := "attached"
			if !a.Attached {
				state = "NOT ATTACHED"
			}
			fmt.Fprintf(out, "    %s (%s): %s\n", a.Name, a.Mode, state)
		}
	}
	for _, m := range st.Maps {
		fmt.Fprintf(out, "  Map %s: %d of %d entries\n", m.Name, m.Count, m.Capacity)
	}
	for _, d := range st.ExternalDependencies {
		fmt.Fprintf(out, "  Dependency %s: %s (%s)\n", d.Type, d.Status, d.LatencyClass)
	}
}
