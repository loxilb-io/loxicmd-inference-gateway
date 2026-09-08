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

func NewGetPIICmd(restOptions *api.RESTOptions) *cobra.Command {
	var stats bool

	var getPIICmd = &cobra.Command{
		Use:   "pii",
		Short: "Show PII detection configuration or statistics",
		Long: `Report PII detection status (/config/pii/status), or detection
statistics with --stats (/config/pii/stats).

ex)
	loxicmd get pii
	loxicmd get pii --stats`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			sub := "status"
			what := "pii status"
			if stats {
				sub = "stats"
				what = "pii stats"
			}
			resp, err := client.PII().SubResources([]string{sub}).Get(ctx)
			if err != nil {
				return exitcode.Unavailablef("get pii: %v", err)
			}
			return printJSONResponse(resp, what)
		},
	}
	getPIICmd.Flags().BoolVar(&stats, "stats", false, "Show PII detection statistics instead of status")
	return getPIICmd
}
