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
	"errors"
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
	applianceCmd.AddCommand(publicAddressCmd(restOptions))
	applianceCmd.AddCommand(gatewayCmd(restOptions))
	applianceCmd.AddCommand(credentialsCmd(restOptions))
	applianceCmd.AddCommand(diagnosticsCmd(restOptions))
	applianceCmd.AddCommand(logsCmd(restOptions))
	applianceCmd.AddCommand(backupCmd(restOptions))
	applianceCmd.AddCommand(restoreCmd(restOptions))
	applianceCmd.AddCommand(updateCmd(restOptions))
	applianceCmd.AddCommand(rollbackCmd(restOptions))
	applianceCmd.AddCommand(factoryResetCmd(restOptions))
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
			return dispatchReadOnly(cmd.OutOrStdout(), cmd.ErrOrStderr(), restOptions, "appliance.status", "status")
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
			return dispatchReadOnly(cmd.OutOrStdout(), cmd.ErrOrStderr(), restOptions, "appliance.network.validate", "network validate")
		},
	})
	return networkCmd
}

// applianceData is the data payload of the CommandResult envelope for a
// dispatched command: the backend's own JSON document, preserved without
// loss, plus the schema's failure triple when the invocation failed.
type applianceData struct {
	Backend       json.RawMessage `json:"backend,omitempty"`
	Origin        *string         `json:"origin,omitempty"`
	HTTPStatus    *int            `json:"httpStatus,omitempty"`
	ComponentCode *string         `json:"componentCode,omitempty"`
	// OperationID is the handle a caller recovers with. Present exactly
	// when the outcome is unknown, which is the only time recovery is a
	// question the caller has to answer.
	OperationID *string `json:"operationId,omitempty"`
}

// dispatchReadOnly invokes a read-only backend subcommand and renders the
// outcome. The backend package performs the contract-version handshake first;
// only a typed degraded-handshake class may continue to one best-effort read.
func dispatchReadOnly(out, errOut io.Writer, restOptions *api.RESTOptions, command, subcommand string) error {
	return dispatchReadOnlyRequest(out, errOut, restOptions, command, &backend.Request{Subcommand: subcommand})
}

func dispatchReadOnlyRequest(out, errOut io.Writer, restOptions *api.RESTOptions, command string, req *backend.Request) error {
	jsonOut := restOptions.PrintOption == "json"
	req.JSON = jsonOut
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	res, err := backend.InvokeReadOnly(ctx, req)
	var validated *backend.ValidatedPayload
	if err == nil {
		validated, err = resolveBackendOutcome(req.Subcommand, res, false, jsonOut)
	}
	if err != nil {
		if jsonOut {
			writeEnvelope(out, command, res, validated, err)
		}
		return err
	}
	if _, err := errOut.Write(res.Stderr); err != nil {
		return err
	}
	if !jsonOut {
		// Human mode: the backend's human output is the output.
		_, werr := out.Write(res.Stdout)
		return werr
	}
	writeEnvelope(out, command, res, validated, nil)
	return nil
}

// writeEnvelope emits the CommandResult document for a dispatched command.
// Backend bytes enter data.backend only through ValidatedPayload; json.Valid
// alone is deliberately not an authorization boundary.
func writeEnvelope(out io.Writer, command string, res *backend.Result, validated *backend.ValidatedPayload, err error) {
	result := envelope.New(command)
	data := &applianceData{}
	if res != nil {
		result.CorrelationID = res.CorrelationID
	}
	if validated != nil {
		data.Backend = append(json.RawMessage(nil), validated.Document...)
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
		// Only an unknown outcome needs a recovery handle. Publishing one on
		// every failure would suggest there is something to recover from
		// when the backend already said there is not.
		if ce.Code == exitcode.Partial && res != nil {
			// A signal-death path has no validated document, so its
			// correlation ID is the only trustworthy recovery handle.
			id := res.CorrelationID
			if validated != nil && validated.OperationID != "" {
				id = validated.OperationID
			}
			data.OperationID = &id
		}
	}
	result.Data = data
	_ = result.Write(out)
}

// resolveBackendOutcome is the CLI-WP03 dispatcher boundary. In JSON mode,
// every normally completed public-taxonomy outcome is validated against the
// exact command tuple before any backend bytes are exposed. Signal deaths are
// classified from execution context because their stdout may be truncated.
// Legacy/out-of-taxonomy normal exits keep the existing FAILED fallback.
func resolveBackendOutcome(subcommand string, res *backend.Result, mutating, jsonOut bool) (*backend.ValidatedPayload, error) {
	if jsonOut && !res.Signaled && (res.ExitCode == 0 || (res.ExitCode >= 2 && res.ExitCode <= 8)) {
		return validateJSONOutcome(subcommand, res)
	}
	return nil, backendOutcome(subcommand, res, mutating)
}

func validateJSONOutcome(subcommand string, res *backend.Result) (*backend.ValidatedPayload, error) {
	selected, ok := backend.PayloadTupleForCommand(subcommand)
	if !ok {
		return nil, payloadExecutionError(backend.PayloadTuple{Command: subcommand}, subcommand,
			"the command has no registered JSON payload tuple", res, "")
	}
	validated, err := backend.ValidatePayload(selected, res.Stdout)
	if err != nil {
		var payloadErr *backend.PayloadValidationError
		if errors.As(err, &payloadErr) {
			return nil, payloadErr.WithExecutionEvidence(res.ExitCode, res.CorrelationID, "")
		}
		return nil, err
	}

	if res.ExitCode == 0 {
		if validated.OperationError != nil {
			return nil, payloadExecutionError(validated.Tuple, validated.Command,
				"an operation-error document accompanied process exit 0", res, validated.OperationID)
		}
		return validated, nil
	}
	if validated.OperationError == nil {
		return nil, payloadExecutionError(validated.Tuple, validated.Command,
			fmt.Sprintf("a success document accompanied process exit %d", res.ExitCode), res, validated.OperationID)
	}
	opErr := validated.OperationError
	if opErr.Exit != res.ExitCode {
		return nil, payloadExecutionError(validated.Tuple, validated.Command,
			fmt.Sprintf("document exit %d differs from process exit %d", opErr.Exit, res.ExitCode), res, validated.OperationID)
	}
	if opErr.CorrelationID != "" && opErr.CorrelationID != res.CorrelationID {
		return nil, payloadExecutionError(validated.Tuple, validated.Command,
			"document correlationId differs from the invocation correlationId", res, validated.OperationID)
	}
	message := opErr.Message
	if message == "" {
		message = backendFailureMessage(subcommand, res)
	}
	return validated, &exitcode.CLIError{
		Code:          exitcode.Code(opErr.Exit),
		Message:       message,
		Origin:        opErr.Origin,
		ComponentCode: opErr.ComponentCode,
	}
}

func payloadExecutionError(tuple backend.PayloadTuple, command, reason string, res *backend.Result, operationID string) error {
	return (&backend.PayloadValidationError{
		Tuple:   tuple,
		Command: command,
		Reason:  reason,
	}).WithExecutionEvidence(res.ExitCode, res.CorrelationID, operationID)
}

// backendOutcome turns a completed invocation into a verdict, or nil when the
// backend exited 0.
//
// The split that matters is between a failure the backend DECIDED and one
// nobody decided. A public-taxonomy exit status is preserved even in human
// mode; JSON mode reaches this function only for signal deaths or
// out-of-taxonomy legacy exits, because structured outcomes are handled by
// resolveBackendOutcome. A process that was killed never reported anything -- our own
// --timeout fired, a signal arrived, the OOM killer chose it -- so what it had
// done by then is unknown.
//
// The exit-code taxonomy resolves that ambiguity downward to safety: an
// unknown outcome is PARTIAL when a state change may have occurred, and only
// 5/7 when it is confirmed none did. A mutating subcommand killed mid-flight
// is the first case exactly, and PARTIAL is the code automation must never
// retry blindly -- half a backup or half an applied address is what retrying
// would compound. A read-only subcommand cannot have changed anything, so the
// same death is reported as the backend being unresponsive, which a bounded
// retry may legitimately ride out.
func backendOutcome(subcommand string, res *backend.Result, mutating bool) error {
	if res.ExitCode == 0 && !res.Signaled {
		return nil
	}
	if !res.Signaled {
		code := exitcode.Failed
		if res.ExitCode >= 2 && res.ExitCode <= 8 {
			code = exitcode.Code(res.ExitCode)
		}
		return &exitcode.CLIError{
			Code:          code,
			Message:       backendFailureMessage(subcommand, res),
			Origin:        "backend",
			ComponentCode: fmt.Sprintf("backend-exit-%d", res.ExitCode),
		}
	}

	cause, code := "was killed before it reported an outcome", "backend-killed"
	if res.TimedOut {
		cause, code = "did not finish within the timeout and was stopped", "backend-timeout"
	}
	if !mutating {
		return &exitcode.CLIError{
			Code: exitcode.Unavailable,
			Message: fmt.Sprintf("the backend's %s %s; it read nothing and changed nothing, so this is safe to retry",
				subcommand, cause),
			Origin:        "backend",
			ComponentCode: code,
		}
	}
	return &exitcode.CLIError{
		Code: exitcode.Partial,
		Message: fmt.Sprintf(
			"the backend's %s %s, so whether it took effect is UNKNOWN; do not retry blindly — "+
				"determine the host's actual state first, using operation id %s",
			subcommand, cause, res.OperationID()),
		Origin:        "backend",
		ComponentCode: code,
	}
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
