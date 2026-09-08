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
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

func NewGetLlamaFirewallCmd(restOptions *api.RESTOptions) *cobra.Command {
	var stats bool

	var getCmd = &cobra.Command{
		Use:   "llamafirewall",
		Short: "Show LlamaFirewall AI-security status or statistics",
		Long: `Report LlamaFirewall status and enabled scanners
(/config/llamafirewall/status), or scanning statistics with --stats
(/config/llamafirewall/stats).

ex)
	loxicmd get llamafirewall
	loxicmd get llamafirewall --stats`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			sub := "status"
			what := "llamafirewall status"
			if stats {
				sub = "stats"
				what = "llamafirewall stats"
			}
			resp, err := client.LlamaFirewall().SubResources([]string{sub}).Get(ctx)
			if err != nil {
				return exitcode.Unavailablef("get llamafirewall: %v", err)
			}
			return printJSONResponse(resp, what)
		},
	}
	getCmd.Flags().BoolVar(&stats, "stats", false, "Show scanning statistics instead of status")
	return getCmd
}
