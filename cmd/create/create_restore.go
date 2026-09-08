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
package create

import (
	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/lifecycle"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewCreateRestoreCmd(restOptions *api.RESTOptions) *cobra.Command {
	var opts lifecycle.RestoreOptions
	var strict bool

	var createRestoreCmd = &cobra.Command{
		Use:   "restore -f <snapshot-file> [--commit]",
		Short: "Restore an instance configuration snapshot",
		Long: `Run the staged restore pipeline on a snapshot document produced by
'loxicmd get snapshot' (/config/restore). The default mode is dry-run, which
validates and returns the plan without mutating anything; pass --commit to
apply the snapshot (with automatic rollback on failure).

The command exits non-zero when the pipeline reports a failure of any kind:
an incompatible document, an unverifiable required recovery dependency, a
rollback, and - for a committed restore - a write-through that failed, which
means the restored configuration would not survive the next restart.

ex)
	loxicmd create restore -f snapshot.json
	loxicmd create restore -f snapshot.json --commit
	loxicmd create restore -f snapshot.json --components loadbalancer,endpoint
	loxicmd create restore -f snapshot.json --commit -o json`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return lifecycle.Restore(restOptions, cmd.OutOrStdout(),
				lifecycle.OptionsFrom(restOptions, strict), opts)
		},
	}
	createRestoreCmd.Flags().StringVarP(&opts.File, "file", "f", "", "Snapshot document to restore (required)")
	createRestoreCmd.Flags().BoolVar(&opts.Commit, "commit", false, "Apply the snapshot (default is dry-run)")
	createRestoreCmd.Flags().StringVar(&opts.Components, "components", "",
		"Comma-separated domains to restore (default: every domain the document declares)")
	createRestoreCmd.Flags().BoolVar(&strict, "strict", false,
		"Fail unless the gateway reports the durable-restore contract (for a commit, whether the restored configuration was persisted)")
	return createRestoreCmd
}
