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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewGetKvInventoryCmd(restOptions *api.RESTOptions) *cobra.Command {
	var serviceID int64
	var epIdx int64
	var epIdxSet bool

	var getKvInventoryCmd = &cobra.Command{
		Use:     "kvinventory --service-id=<id> --ep-idx=<idx>",
		Short:   "Get the KV-cache block-hash inventory for a service endpoint",
		Aliases: []string{"kv-inventory"},
		Long: `Dump the per-endpoint KV-cache block-hash inventory the gateway's KV
subscriber has ingested (read-only observability).

ex)
	loxicmd get kvinventory --service-id=3 --ep-idx=0`,
		Run: func(cmd *cobra.Command, args []string) {
			if !cmd.Flags().Changed("service-id") || !epIdxSet {
				fmt.Printf("Error: --service-id and --ep-idx are required\n")
				return
			}
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			q := map[string]string{
				"service_id": fmt.Sprintf("%d", serviceID),
				"ep_idx":     fmt.Sprintf("%d", epIdx),
			}
			resp, err := client.AIKvInventory().Query(q).Get(ctx)
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				fmt.Printf("Error: %s\n", api.NewAPIError(resp.StatusCode, body).Error())
				return
			}

			inv := api.AIKvInventoryResponse{}
			if err := json.Unmarshal(body, &inv); err != nil {
				fmt.Printf("Error: Failed to unmarshal HTTP response: (%s)\n", err.Error())
				return
			}
			if restOptions.PrintOption == "json" {
				indent, _ := json.MarshalIndent(inv, "", "    ")
				fmt.Println(string(indent))
				return
			}
			fmt.Printf("service_id=%d ep_idx=%d hash_algo=%s total=%d\n", inv.ServiceID, inv.EpIdx, inv.HashAlgo, inv.Total)
			table := TableInit()
			table.SetHeader(KVINVENTORY_TITLE)
			var data [][]string
			for _, b := range inv.Blocks {
				data = append(data, []string{fmt.Sprintf("%d", b.BlockIdx), fmt.Sprintf("%d", b.HashUint64)})
			}
			TableShow(data, table)
		},
	}
	getKvInventoryCmd.Flags().Int64Var(&serviceID, "service-id", 0, "Service ID (required)")
	getKvInventoryCmd.Flags().Int64Var(&epIdx, "ep-idx", 0, "Endpoint index (required)")
	getKvInventoryCmd.PreRun = func(cmd *cobra.Command, args []string) {
		epIdxSet = cmd.Flags().Changed("ep-idx")
	}
	return getKvInventoryCmd
}
