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

func NewGetL4TraceCmd(restOptions *api.RESTOptions) *cobra.Command {
	var getL4TraceCmd = &cobra.Command{
		Use:   "l4trace",
		Short: "Show L4 connection tracing status and statistics",
		Long: `Report L4 connection-tracing status, sampling rate, and event
statistics (/config/l4trace/status).

ex)
	loxicmd get l4trace`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			resp, err := client.L4Trace().SubResources([]string{"status"}).Get(ctx)
			if err != nil {
				return exitcode.Unavailablef("get l4trace: %v", err)
			}
			return printJSONResponse(resp, "l4trace status")
		},
	}
	return getL4TraceCmd
}
