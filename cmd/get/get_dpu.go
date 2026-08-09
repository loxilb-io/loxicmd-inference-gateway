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
	"strconv"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewGetDPUCmd(restOptions *api.RESTOptions) *cobra.Command {
	var hwcounters bool
	var flows bool
	var pipe string
	var svc string
	var ep string
	var limit int

	var getDPUCmd = &cobra.Command{
		Use:   "dpu",
		Short: "Show DPU offload debug state and hardware counters",
		Long: `Report DPU offload status, aggregate counters, and per-pipe counters
(/config/dpu/debug). Use --flows for per-flow/FDB/route/ACL counter arrays, or
--pipe/--svc/--ep/--limit for filtered per-entry detail. --hwcounters returns
per-flow hardware counters (/config/dpu/hwcounters).

Note: this is a raw-middleware endpoint; trigger debug actions with 'set dpu'.

ex)
	loxicmd get dpu
	loxicmd get dpu --flows
	loxicmd get dpu --pipe ct_fwd_5tuple --limit 100
	loxicmd get dpu --hwcounters`,
		Run: func(cmd *cobra.Command, args []string) {
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			if hwcounters {
				resp, err := client.DPU().SubResources([]string{"hwcounters"}).Get(ctx)
				if err != nil {
					fmt.Printf("Error: %s\n", err.Error())
					return
				}
				printJSONResponse(resp, "dpu hwcounters")
				return
			}

			query := map[string]string{}
			if flows {
				query["flows"] = "1"
			}
			if pipe != "" {
				query["pipe"] = pipe
			}
			if svc != "" {
				query["svc"] = svc
			}
			if ep != "" {
				query["ep"] = ep
			}
			if cmd.Flags().Changed("limit") {
				query["limit"] = strconv.Itoa(limit)
			}

			dpu := client.DPU().SubResources([]string{"debug"})
			if len(query) > 0 {
				dpu = dpu.Query(query)
			}
			resp, err := dpu.Get(ctx)
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			printJSONResponse(resp, "dpu debug")
		},
	}
	getDPUCmd.Flags().BoolVar(&hwcounters, "hwcounters", false, "Show per-flow hardware counters")
	getDPUCmd.Flags().BoolVar(&flows, "flows", false, "Include per-flow/FDB/route/ACL counter arrays")
	getDPUCmd.Flags().StringVar(&pipe, "pipe", "", "Restrict filtered entry query to one hardware pipe")
	getDPUCmd.Flags().StringVar(&svc, "svc", "", "Filter entries by service name")
	getDPUCmd.Flags().StringVar(&ep, "ep", "", "Filter entries by endpoint (addr:port)")
	getDPUCmd.Flags().IntVar(&limit, "limit", 200, "Maximum entries returned by the filtered query (max 2000)")
	return getDPUCmd
}
