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
package get

import (
	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/lifecycle"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewGetSnapshotCmd(restOptions *api.RESTOptions) *cobra.Command {
	var opts lifecycle.SnapshotOptions
	var strict bool

	var getSnapshotCmd = &cobra.Command{
		Use:   "snapshot",
		Short: "Download a complete instance configuration snapshot",
		Long: `Download the versioned, checksummed snapshot document covering all v1
configuration domains (/config/snapshot). Restore it later with
'loxicmd create restore -f <file>'.

The document is checked against its own checksum before anything is stored,
and -f writes it through a temporary file that is renamed into place with
mode 0600 - a failed or truncated download can never overwrite the snapshot
already at that path.

ex)
	loxicmd get snapshot -f snapshot.json
	loxicmd get snapshot --components loadbalancer,endpoint
	loxicmd get snapshot -f snapshot.json -o json`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return lifecycle.Snapshot(restOptions, cmd.OutOrStdout(), cmd.ErrOrStderr(),
				lifecycle.OptionsFrom(restOptions, strict), opts)
		},
	}
	getSnapshotCmd.Flags().StringVar(&opts.Components, "components", "", "Comma-separated v1 domains to capture (default: all)")
	getSnapshotCmd.Flags().StringVarP(&opts.File, "file", "f", "", "Write the snapshot to this file instead of stdout")
	getSnapshotCmd.Flags().BoolVar(&strict, "strict", false,
		"Fail unless the downloaded document carries a checksum to verify it against")
	return getSnapshotCmd
}
