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
	"strconv"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewSetGPUCmd(restOptions *api.RESTOptions) *cobra.Command {
	var enable bool
	var disable bool
	var cleanup bool
	var maxAgeHours int

	var setGPUCmd = &cobra.Command{
		Use:   "gpu (--enable | --disable | --cleanup)",
		Short: "Enable, disable, or clean up GPU-aware load balancing",
		Long: `Toggle GPU-aware routing (/config/gpu/enable|disable) or run a manual
conversation cleanup (/config/gpu/conversations/cleanup).

ex)
	loxicmd set gpu --enable
	loxicmd set gpu --disable
	loxicmd set gpu --cleanup --max-age-hours 2`,
		Run: func(cmd *cobra.Command, args []string) {
			n := 0
			for _, b := range []bool{enable, disable, cleanup} {
				if b {
					n++
				}
			}
			if n != 1 {
				fmt.Printf("Error: specify exactly one of --enable, --disable, or --cleanup\n")
				return
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			switch {
			case enable:
				resp, err := client.GPU().SubResources([]string{"enable"}).Create(ctx, nil)
				reportPost(resp, err, "GPU-aware load balancing enabled.")
			case disable:
				resp, err := client.GPU().SubResources([]string{"disable"}).Create(ctx, nil)
				reportPost(resp, err, "GPU-aware load balancing disabled.")
			case cleanup:
				gpu := client.GPU().SubResources([]string{"conversations", "cleanup"})
				if cmd.Flags().Changed("max-age-hours") {
					gpu = gpu.Query(map[string]string{"max_age_hours": strconv.Itoa(maxAgeHours)})
				}
				resp, err := gpu.Create(ctx, nil)
				reportPost(resp, err, "Conversation cleanup completed.")
			}
		},
	}
	setGPUCmd.Flags().BoolVar(&enable, "enable", false, "Enable GPU-aware load balancing")
	setGPUCmd.Flags().BoolVar(&disable, "disable", false, "Disable GPU-aware load balancing")
	setGPUCmd.Flags().BoolVar(&cleanup, "cleanup", false, "Run a manual stale-conversation cleanup")
	setGPUCmd.Flags().IntVar(&maxAgeHours, "max-age-hours", 1, "Max age in hours to keep during --cleanup")
	return setGPUCmd
}
