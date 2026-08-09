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
package set

import (
	"fmt"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewSetDPUCmd(restOptions *api.RESTOptions) *cobra.Command {
	var action string
	var plugin string
	var mode string

	var setDPUCmd = &cobra.Command{
		Use:   "dpu --action <unregister|cb_force> [--plugin name] [--mode open|close]",
		Short: "Trigger a DPU offload debug action",
		Long: `Execute a DPU debug action (/config/dpu/debug). Supported actions:
  unregister  unload a DPU plugin by name (requires --plugin)
  cb_force    pin the offload circuit breaker open or closed (requires --mode)

Note: this is a raw-middleware endpoint; inspect state with 'get dpu'.

ex)
	loxicmd set dpu --action unregister --plugin doca
	loxicmd set dpu --action cb_force --mode open`,
		Run: func(cmd *cobra.Command, args []string) {
			switch action {
			case "unregister":
				if plugin == "" {
					fmt.Printf("Error: --plugin is required for --action unregister\n")
					return
				}
			case "cb_force":
				if mode != "open" && mode != "close" {
					fmt.Printf("Error: --mode must be open or close for --action cb_force\n")
					return
				}
			default:
				fmt.Printf("Error: --action must be unregister or cb_force\n")
				return
			}
			req := api.DPUDebugAction{Action: action, Plugin: plugin, Mode: mode}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			resp, err := client.DPU().SubResources([]string{"debug"}).Create(ctx, req)
			reportPost(resp, err, fmt.Sprintf("DPU debug action '%s' executed.", action))
		},
	}
	setDPUCmd.Flags().StringVar(&action, "action", "", "Debug action: unregister or cb_force (required)")
	setDPUCmd.Flags().StringVar(&plugin, "plugin", "", "Plugin name (required for unregister)")
	setDPUCmd.Flags().StringVar(&mode, "mode", "", "Circuit breaker mode: open or close (required for cb_force)")
	return setDPUCmd
}
