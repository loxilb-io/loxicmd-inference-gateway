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
package get

import (
	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/lifecycle"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewGetDiagnosticsCmd(restOptions *api.RESTOptions) *cobra.Command {
	var getDiagnosticsCmd = &cobra.Command{
		Use:   "diagnostics",
		Short: "Show the gateway's secret-safe diagnostics",
		Long: `Show the gateway's bounded, allowlist-only diagnostics
(GET /diagnostics): build identity, served API contract, uptime, the
readiness verdict with reasons, operator maintenance state, per-interface
eBPF attachment, per-map utilization, external-dependency reachability
with a latency class, and the last configuration lifecycle outcomes.

By design this surface never carries credentials, connection strings,
request/response bodies, rule contents, or key material - the gateway
does not collect them for it. With -o json the gateway's body is printed
verbatim.

ex)
	loxicmd get diagnostics
	loxicmd get diagnostics -o json`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return lifecycle.DiagnosticsGet(restOptions, cmd.OutOrStdout(),
				restOptions.PrintOption == "json")
		},
	}
	return getDiagnosticsCmd
}
