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
	"fmt"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewGetGPUCmd(restOptions *api.RESTOptions) *cobra.Command {
	var workers bool

	var getGPUCmd = &cobra.Command{
		Use:   "gpu",
		Short: "Show GPU-aware load balancing status",
		Long: `Report GPU-aware routing status (/config/gpu/status), or the metrics
for all tracked workers with --workers (/config/worker/metrics).

ex)
	loxicmd get gpu
	loxicmd get gpu --workers`,
		Run: func(cmd *cobra.Command, args []string) {
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			if workers {
				resp, err := client.WorkerMetrics().Get(ctx)
				if err != nil {
					fmt.Printf("Error: %s\n", err.Error())
					return
				}
				printJSONResponse(resp, "worker metrics")
				return
			}
			resp, err := client.GPU().SubResources([]string{"status"}).Get(ctx)
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			printJSONResponse(resp, "gpu status")
		},
	}
	getGPUCmd.Flags().BoolVar(&workers, "workers", false, "Show per-worker GPU metrics")
	return getGPUCmd
}
