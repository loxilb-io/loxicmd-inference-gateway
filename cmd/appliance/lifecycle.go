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
package appliance

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/backend"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
	"github.com/spf13/cobra"
)

var (
	planHashShape  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	challengeShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,127}$`)
	identifierArg  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:+-]{0,127}$`)
)

func lifecycleParent(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("appliance %s needs a subcommand", use)
			}
			return exitcode.Usagef("unknown command %q for \"loxicmd appliance %s\"", args[0], use)
		},
	}
}

// existingInputFile validates references without opening or copying their
// contents. Archive, bundle, and key values remain references only; no secret
// material enters argv, the environment, or CLI output.
func existingInputFile(label, path string) error {
	if !filepath.IsAbs(path) {
		return exitcode.Invalidf("%s must be an absolute path, got %q", label, path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return &exitcode.CLIError{Code: exitcode.Precondition, Message: fmt.Sprintf("%s cannot be inspected: %v", label, err), ComponentCode: "input-file-refused"}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return &exitcode.CLIError{Code: exitcode.Precondition, Message: fmt.Sprintf("%s must be a regular file and must not be a symlink", label), ComponentCode: "input-file-refused"}
	}
	return nil
}

func validateIdentifier(label, value string) error {
	if !identifierArg.MatchString(value) {
		return exitcode.Invalidf("%s must be a non-flag identifier of at most 128 characters", label)
	}
	return nil
}

func lifecycleExecuteCmd(restOptions *api.RESTOptions, family string) *cobra.Command {
	var planHash, confirm string
	command := &cobra.Command{
		Use:   "execute --plan-hash SHA256 --confirm CHALLENGE",
		Short: "Execute the exact unexpired plan after one-time confirmation",
		Long: `Dispatches only the plan selected by --plan-hash. --confirm is the
one-time, plan-bound challenge issued by the appliance control plane. This CLI
does not implement host mutation and does not accept credentials or secret
values in argv or the environment.`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !planHashShape.MatchString(planHash) {
				return exitcode.Invalidf("--plan-hash must be exactly 64 lowercase hexadecimal characters")
			}
			if !challengeShape.MatchString(confirm) {
				return exitcode.Invalidf("--confirm must be the 8 to 128 character one-time challenge bound to the plan")
			}
			req := &backend.Request{Subcommand: family + " execute", Args: []string{"--plan-hash", planHash, "--confirm", confirm}}
			return dispatchMutating(cmd.OutOrStdout(), cmd.ErrOrStderr(), restOptions,
				"appliance."+family+".execute", req)
		},
	}
	command.Flags().StringVar(&planHash, "plan-hash", "", "SHA-256 of the exact unexpired plan to execute")
	command.Flags().StringVar(&confirm, "confirm", "", "One-time challenge bound to the plan hash (not a password or bearer credential)")
	_ = command.MarkFlagRequired("plan-hash")
	_ = command.MarkFlagRequired("confirm")
	return command
}

func restoreCmd(restOptions *api.RESTOptions) *cobra.Command {
	parent := lifecycleParent("restore", "Plan and execute an authenticated appliance restore")
	var keyFile string
	plan := &cobra.Command{
		Use:           "plan ARCHIVE --key-file PATH",
		Short:         "Validate a restore and return a stable, expiring plan (read-only)",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := existingInputFile("archive", args[0]); err != nil {
				return err
			}
			if keyFile == "" {
				return exitcode.Usagef("--key-file is required")
			}
			if err := backend.CheckSecretPath("key file", keyFile); err != nil {
				return err
			}
			req := &backend.Request{Subcommand: "restore plan", Args: []string{args[0], "--key-file", keyFile}}
			return dispatchReadOnlyRequest(cmd.OutOrStdout(), cmd.ErrOrStderr(), restOptions, "appliance.restore.plan", req)
		},
	}
	plan.Flags().StringVar(&keyFile, "key-file", "", "Absolute root-only regular file holding the backup key; symlinks are refused")
	parent.AddCommand(plan, lifecycleExecuteCmd(restOptions, "restore"))
	return parent
}

func updateCmd(restOptions *api.RESTOptions) *cobra.Command {
	parent := lifecycleParent("update", "Plan, execute, and inspect a signed appliance update")
	plan := &cobra.Command{
		Use:           "plan BUNDLE",
		Short:         "Verify a signed bundle and return a stable, expiring plan (read-only)",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := existingInputFile("bundle", args[0]); err != nil {
				return err
			}
			req := &backend.Request{Subcommand: "update plan", Args: []string{args[0]}}
			return dispatchReadOnlyRequest(cmd.OutOrStdout(), cmd.ErrOrStderr(), restOptions, "appliance.update.plan", req)
		},
	}
	parent.AddCommand(plan, lifecycleExecuteCmd(restOptions, "update"), lifecycleStatusCmd(restOptions, "update"))
	return parent
}

func rollbackCmd(restOptions *api.RESTOptions) *cobra.Command {
	parent := lifecycleParent("rollback", "Plan, execute, and inspect a governed appliance rollback")
	var archive, keyFile string
	plan := &cobra.Command{
		Use:           "plan RELEASE --archive PATH --key-file PATH",
		Short:         "Validate an approved release and recovery point (read-only)",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateIdentifier("release", args[0]); err != nil {
				return err
			}
			if archive == "" || keyFile == "" {
				return exitcode.Usagef("--archive and --key-file are both required")
			}
			if err := existingInputFile("archive", archive); err != nil {
				return err
			}
			if err := backend.CheckSecretPath("key file", keyFile); err != nil {
				return err
			}
			req := &backend.Request{Subcommand: "rollback plan", Args: []string{args[0], "--archive", archive, "--key-file", keyFile}}
			return dispatchReadOnlyRequest(cmd.OutOrStdout(), cmd.ErrOrStderr(), restOptions, "appliance.rollback.plan", req)
		},
	}
	plan.Flags().StringVar(&archive, "archive", "", "Absolute regular-file path of the approved pre-update backup")
	plan.Flags().StringVar(&keyFile, "key-file", "", "Absolute root-only regular file holding the backup key; symlinks are refused")
	parent.AddCommand(plan, lifecycleExecuteCmd(restOptions, "rollback"), lifecycleStatusCmd(restOptions, "rollback"))
	return parent
}

func factoryResetCmd(restOptions *api.RESTOptions) *cobra.Command {
	parent := lifecycleParent("factory-reset", "Plan and execute a safety-preserving factory reset")
	plan := &cobra.Command{
		Use:           "plan",
		Short:         "Return delete/preserve lists and backup gates (read-only)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			req := &backend.Request{Subcommand: "factory-reset plan"}
			return dispatchReadOnlyRequest(cmd.OutOrStdout(), cmd.ErrOrStderr(), restOptions, "appliance.factory-reset.plan", req)
		},
	}
	parent.AddCommand(plan, lifecycleExecuteCmd(restOptions, "factory-reset"))
	return parent
}

func lifecycleStatusCmd(restOptions *api.RESTOptions, family string) *cobra.Command {
	return &cobra.Command{
		Use:           "status OPERATION_ID",
		Short:         "Read durable lifecycle operation status (read-only)",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateIdentifier("operation ID", args[0]); err != nil {
				return err
			}
			req := &backend.Request{Subcommand: family + " status", Args: []string{args[0]}}
			return dispatchReadOnlyRequest(cmd.OutOrStdout(), cmd.ErrOrStderr(), restOptions, "appliance."+family+".status", req)
		},
	}
}
