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

func NewSetL4TraceCmd(restOptions *api.RESTOptions) *cobra.Command {
	var enable bool
	var disable bool
	var resetStats bool
	var samplingRate int64

	var setL4TraceCmd = &cobra.Command{
		Use:   "l4trace (--enable | --disable | --sampling-rate N | --reset-stats)",
		Short: "Configure L4 connection tracing",
		Long: `Toggle L4 connection tracing (/config/l4trace/enable|disable), update the
sampling rate (/config/l4trace/sampling), or reset statistics
(/config/l4trace/stats/reset). --sampling-rate may accompany --enable, or be
used alone to update the rate while tracing is running.

ex)
	loxicmd set l4trace --enable --sampling-rate 100
	loxicmd set l4trace --sampling-rate 10
	loxicmd set l4trace --disable
	loxicmd set l4trace --reset-stats`,
		Run: func(cmd *cobra.Command, args []string) {
			rateChanged := cmd.Flags().Changed("sampling-rate")
			n := 0
			for _, b := range []bool{enable, disable, resetStats} {
				if b {
					n++
				}
			}
			// --sampling-rate alone is a valid action (update rate while running).
			if n == 0 && rateChanged {
				n = 1
			}
			if n != 1 {
				fmt.Printf("Error: specify exactly one of --enable, --disable, --sampling-rate, or --reset-stats\n")
				return
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			switch {
			case enable:
				req := api.L4TraceEnableRequest{}
				if rateChanged {
					req.SamplingRate = &samplingRate
				}
				resp, err := client.L4Trace().SubResources([]string{"enable"}).Create(ctx, req)
				reportPost(resp, err, "L4 connection tracing enabled.")
			case disable:
				resp, err := client.L4Trace().SubResources([]string{"disable"}).Create(ctx, nil)
				reportPost(resp, err, "L4 connection tracing disabled.")
			case resetStats:
				resp, err := client.L4Trace().SubResources([]string{"stats", "reset"}).Create(ctx, nil)
				reportPost(resp, err, "L4 tracing statistics reset.")
			default: // sampling-rate only → PUT /config/l4trace/sampling
				req := api.L4TraceSamplingRequest{SamplingRate: samplingRate}
				resp, err := client.L4Trace().SubResources([]string{"sampling"}).Put(ctx, req)
				reportPost(resp, err, fmt.Sprintf("L4 sampling rate updated to %d%%.", samplingRate))
			}
		},
	}
	f := setL4TraceCmd.Flags()
	f.BoolVar(&enable, "enable", false, "Enable L4 connection tracing")
	f.BoolVar(&disable, "disable", false, "Disable L4 connection tracing")
	f.BoolVar(&resetStats, "reset-stats", false, "Reset L4 tracing statistics")
	f.Int64Var(&samplingRate, "sampling-rate", 100, "Percentage of connections to trace (0-100)")
	return setL4TraceCmd
}
