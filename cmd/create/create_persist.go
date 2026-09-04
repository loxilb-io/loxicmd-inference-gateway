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

func NewCreatePersistCmd(restOptions *api.RESTOptions) *cobra.Command {
	var strict bool

	var createPersistCmd = &cobra.Command{
		Use:   "persist",
		Short: "Persist the running configuration to disk",
		Long: `Dump the gateway's live configuration to {config-path}/snapshot.json so it
survives a daemon restart (/config/persist). This is the canonical boot-save
command; 'loxicmd save --api' is a compatibility alias for the same call.

The command exits non-zero unless the gateway reports that the configuration
was persisted, and prints the persisted document's identity (path, checksum,
schema version, lineage generation) and the domains it covers.

ex)
	loxicmd create persist
	loxicmd create persist -o json
	loxicmd create persist --strict`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return lifecycle.Persist(restOptions, cmd.OutOrStdout(), cmd.ErrOrStderr(),
				lifecycle.OptionsFrom(restOptions, strict), "create persist")
		},
	}
	createPersistCmd.Flags().BoolVar(&strict, "strict", false,
		"Fail unless the gateway reports the persisted document's identity and coverage (refuses older gateways rather than claiming what they did not report)")
	return createPersistCmd
}
