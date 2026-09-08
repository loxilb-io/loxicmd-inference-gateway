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

// MaintenanceSetOptions are the state-change inputs of 'set maintenance'.
type MaintenanceSetOptions struct {
	// Enable true enters maintenance, false leaves it.
	Enable bool
	// DrainTimeoutSeconds declares the drain window on enter.
	DrainTimeoutSeconds uint32
}

// MaintenanceGet reads the operator maintenance state with its drain
// read-back (GET /maintenance).
func MaintenanceGet(restOptions *api.RESTOptions, out, errOut io.Writer, o Options) error {
	state, err := doMaintenanceGet(restOptions)
	report := &api.LifecycleReport{Command: "get maintenance", Maintenance: state}
	if err != nil {
		return render(out, errOut, o, report, nil, err)
	}
	return render(out, errOut, o, report, humanMaintenance, nil)
}

// MaintenanceSet enters or leaves operator maintenance (PUT /maintenance).
//
// The one contract clause that matters more than any other here: a
// state-CHANGING request whose outcome this process cannot know -- the
// request timed out, the connection broke, the response never arrived --
// is reported as recovery-required, never as success and never as a plain
// request failure that automation might retry blindly. The gateway may or
// may not be in maintenance at that point; only 'get maintenance' can say.
func MaintenanceSet(restOptions *api.RESTOptions, out, errOut io.Writer, o Options, so MaintenanceSetOptions) error {
	command := "set maintenance off"
	if so.Enable {
		command = "set maintenance on"
	}
	state, err := doMaintenanceSet(restOptions, so)
	report := &api.LifecycleReport{Command: command, Maintenance: state}
	if err != nil {
		return render(out, errOut, o, report, nil, err)
	}
	return render(out, errOut, o, report, humanMaintenance, nil)
}

func doMaintenanceGet(restOptions *api.RESTOptions) (*api.MaintenanceState, error) {
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.Maintenance().Get(ctx)
	if err != nil {
		return nil, transportError("maintenance request failed", err)
	}
	defer resp.Body.Close()
	return decodeMaintenance(resp)
}

func doMaintenanceSet(restOptions *api.RESTOptions, so MaintenanceSetOptions) (*api.MaintenanceState, error) {
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.Maintenance().Put(ctx, api.MaintenanceRequest{
		Enabled:             so.Enable,
		DrainTimeoutSeconds: so.DrainTimeoutSeconds,
	})
	if err != nil {
		// The request may have reached the gateway and been applied
		// before whatever broke, broke. Unknown state is never success.
		return nil, &api.LifecycleError{
			Reason: api.ReasonRecoveryRequired,
			Message: fmt.Sprintf(
				"the maintenance change could not be confirmed and may or may not have been applied: %v; "+
					"verify the actual state with 'loxicmd get maintenance' before acting on it", err),
		}
	}
	defer resp.Body.Close()
	return decodeMaintenance(resp)
}

// decodeMaintenance turns a maintenance response into state or a coded
// failure. A 2xx whose body does not decode is a decode failure, not a
// guess at what the gateway meant.
func decodeMaintenance(resp *http.Response) (*api.MaintenanceState, error) {
	body, err := readBody(resp.Body, "maintenance")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, api.NewStatusError(resp.StatusCode, body)
	}
	var state api.MaintenanceState
	if derr := json.Unmarshal(body, &state); derr != nil || state.State == "" {
		return nil, &api.LifecycleError{
			Reason:  api.ReasonDecodeFailed,
			Message: "the gateway's maintenance response could not be decoded",
			Body:    string(body),
		}
	}
	return &state, nil
}

func humanMaintenance(out io.Writer, report *api.LifecycleReport) error {
	st := report.Maintenance
	if st == nil {
		return nil
	}
	fmt.Fprintf(out, "Maintenance: %s\n", st.State)
	if st.OperationID != "" {
		fmt.Fprintf(out, "  Operation: %s\n", st.OperationID)
	}
	fmt.Fprintf(out, "  Refusing new config: %v\n", st.RefusingNewConfig)
	fmt.Fprintf(out, "  Refusing new inference: %v\n", st.RefusingNewInference)
	fmt.Fprintf(out, "  In-flight streams: %d\n", st.InFlightStreams)
	if st.State == "maintenance" {
		if st.DrainTimeoutSeconds > 0 {
			fmt.Fprintf(out, "  Elapsed: %ds of a %ds drain window\n", st.ElapsedSeconds, st.DrainTimeoutSeconds)
			if st.DrainDeadlineExceeded {
				fmt.Fprintln(out, "  Drain window EXCEEDED - the gateway stays in maintenance until told otherwise")
			}
		} else {
			fmt.Fprintf(out, "  Elapsed: %ds (no drain window declared)\n", st.ElapsedSeconds)
		}
		if st.EnteredAt != "" {
			fmt.Fprintf(out, "  Entered at: %s\n", st.EnteredAt)
		}
	}
	writeNotes(out, report)
	return nil
}
