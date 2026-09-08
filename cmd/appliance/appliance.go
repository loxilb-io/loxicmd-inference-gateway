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

// Package appliance is the host lifecycle namespace: a thin dispatcher over
// the host backend per contracts/host-backend-contract.md. Every command
// here runs host-locally against the backend executable and never requires
// a gateway API connection — the family must work while the gateway
// container is stopped or unhealthy.
package appliance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/backend"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/envelope"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

// ApplianceCmd builds the `loxicmd appliance` family. restOptions supplies
// only the global -o/--output selector; no command in this family opens a
// gateway API connection.
func ApplianceCmd(restOptions *api.RESTOptions) *cobra.Command {
	applianceCmd := &cobra.Command{
		Use:   "appliance",
		Short: "Read and drive the appliance host lifecycle (via the host backend)",
		Long: `Host lifecycle commands, dispatched to the appliance backend installed
with the product. These run host-locally: they do not need the gateway API
and keep working while the gateway container is stopped or unhealthy.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("appliance needs a subcommand")
			}
			return exitcode.Usagef("unknown command %q for \"loxicmd appliance\"", args[0])
		},
	}

	applianceCmd.AddCommand(statusCmd(restOptions))
	applianceCmd.AddCommand(networkCmd(restOptions))
	for _, unavailable := range []struct{ use, what string }{
		{"restore", "restoring the appliance from a backup"},
		{"update", "updating the appliance software"},
		{"rollback", "rolling back an appliance update"},
		{"factory-reset", "resetting the appliance to factory state"},
	} {
		applianceCmd.AddCommand(unavailableCmd(restOptions, unavailable.use, unavailable.what))
	}
	return applianceCmd
}

func statusCmd(restOptions *api.RESTOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Read the complete appliance lifecycle state (read-only)",
		Long: `Reads the appliance lifecycle state from the host backend: product
release, first-boot marker, per-plane liveness and readiness, network
profile and interface roles, and active operations. Read-only: no option
changes host state.

Examples:
  loxicmd appliance status
  loxicmd appliance status -o json`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return dispatchReadOnly(cmd.OutOrStdout(), restOptions, "appliance.status", "status")
		},
	}
}

func networkCmd(restOptions *api.RESTOptions) *cobra.Command {
	networkCmd := &cobra.Command{
		Use:   "network",
		Short: "Appliance network checks (read-only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("appliance network needs a subcommand")
			}
			return exitcode.Usagef("unknown command %q for \"loxicmd appliance network\"", args[0])
		},
	}
	networkCmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Validate interface roles and routing safety without changing anything",
		Long: `Validates the recorded interface role intent: interface existence,
duplicate roles, frontend/backend mapping, CIDR and default-route safety,
and MTU/rp_filter risks. Strictly read-only: it never changes addresses,
routes, MTU, sysctls, or data-plane attachment.

Examples:
  loxicmd appliance network validate
  loxicmd appliance network validate -o json`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return dispatchReadOnly(cmd.OutOrStdout(), restOptions, "appliance.network.validate", "network validate")
		},
	})
	return networkCmd
}

// unavailableCmd is a function the current release does not provide. Per
// the invocation contract it is visible in help as unavailable and fails
// with a contract mismatch when invoked — never a successful stub.
func unavailableCmd(restOptions *api.RESTOptions, use, what string) *cobra.Command {
	return &cobra.Command{
		Use:           use,
		Short:         "Not available in this release",
		Long:          fmt.Sprintf("Not available in this release: %s ships with a later appliance backend.", what),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			err := &exitcode.CLIError{
				Code:          exitcode.ContractMismatch,
				Message:       fmt.Sprintf("appliance %s is not available in this release", use),
				Origin:        "backend",
				ComponentCode: "command-unavailable",
			}
			if restOptions.PrintOption == "json" {
				writeEnvelope(cmd.OutOrStdout(), "appliance."+use, nil, err)
			}
			return err
		},
	}
}

// applianceData is the data payload of the CommandResult envelope for a
// dispatched command: the backend's own JSON document, preserved without
// loss, plus the schema's failure triple when the invocation failed.
type applianceData struct {
	Backend       json.RawMessage `json:"backend,omitempty"`
	Origin        *string         `json:"origin,omitempty"`
	HTTPStatus    *int            `json:"httpStatus,omitempty"`
	ComponentCode *string         `json:"componentCode,omitempty"`
}

// dispatchReadOnly invokes a read-only backend subcommand and renders the
// outcome. Read-only commands skip the contract-version handshake (it is
// required before mutating calls only): the invocation itself is the
// availability probe, and an absent backend classifies as unavailable.
func dispatchReadOnly(out io.Writer, restOptions *api.RESTOptions, command, subcommand string) error {
	jsonOut := restOptions.PrintOption == "json"
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	res, err := backend.Invoke(ctx, subcommand, jsonOut)
	if err == nil && res.ExitCode != 0 {
		// The backend ran and refused: its own verdict is preserved
		// verbatim — the CLI does not reinterpret a failure it cannot
		// see into, and never converts one into success.
		err = &exitcode.CLIError{
			Code:          exitcode.Failed,
			Message:       backendFailureMessage(subcommand, res),
			Origin:        "backend",
			ComponentCode: fmt.Sprintf("backend-exit-%d", res.ExitCode),
		}
	}
	if err != nil {
		if jsonOut {
			writeEnvelope(out, command, res, err)
		}
		return err
	}
	if !jsonOut {
		// Human mode: the backend's human output is the output.
		_, werr := out.Write(res.Stdout)
		return werr
	}
	// JSON mode: the backend emits exactly one JSON document, which
	// becomes the envelope's data payload. A backend answering -o json
	// with something else broke the invocation contract; that is a
	// contract error, never silently-wrapped prose.
	if !json.Valid(res.Stdout) {
		err = &exitcode.CLIError{
			Code:          exitcode.ContractMismatch,
			Message:       fmt.Sprintf("the backend's %s output is not a JSON document", subcommand),
			Origin:        "backend",
			ComponentCode: "contract-invalid",
		}
		writeEnvelope(out, command, res, err)
		return err
	}
	writeEnvelope(out, command, res, nil)
	return nil
}

// writeEnvelope emits the CommandResult document for a dispatched command.
func writeEnvelope(out io.Writer, command string, res *backend.Result, err error) {
	result := envelope.New(command)
	data := &applianceData{}
	if res != nil {
		result.CorrelationID = res.CorrelationID
		if json.Valid(res.Stdout) {
			data.Backend = json.RawMessage(res.Stdout)
		}
	}
	if err != nil {
		ce := exitcode.Classify(err)
		result.Fail(envelope.Code(ce.Code.Label()), ce.Message)
		origin := ce.Origin
		if origin == "" {
			origin = "cli"
		}
		httpStatus, componentCode := ce.HTTPStatus, ce.ComponentCode
		data.Origin, data.HTTPStatus, data.ComponentCode = &origin, &httpStatus, &componentCode
	}
	result.Data = data
	_ = result.Write(out)
}

// backendFailureMessage folds the backend's stderr into the one human line
// the exit point prints, trimmed to its first line so a stack trace cannot
// flood the terminal (the full streams stay available to -o json callers
// through the preserved document).
func backendFailureMessage(subcommand string, res *backend.Result) string {
	line := firstLine(res.Stderr)
	if line == "" {
		return fmt.Sprintf("the backend's %s failed (exit %d)", subcommand, res.ExitCode)
	}
	return fmt.Sprintf("the backend's %s failed (exit %d): %s", subcommand, res.ExitCode, line)
}

func firstLine(b []byte) string {
	for i, c := range b {
		if c == '\n' {
			return string(b[:i])
		}
	}
	return string(b)
}

// requestContext honors the global --timeout the same way the gateway
// commands do, so a wedged backend cannot hang automation forever.
func requestContext(restOptions *api.RESTOptions) (context.Context, context.CancelFunc) {
	if restOptions.Timeout > 0 {
		return context.WithTimeout(context.Background(), time.Duration(restOptions.Timeout)*time.Second)
	}
	return context.Background(), func() {}
}
