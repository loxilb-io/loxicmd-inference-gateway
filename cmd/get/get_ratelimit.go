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
	"sort"
	"strings"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewGetRateLimitCmd(restOptions *api.RESTOptions) *cobra.Command {
	var getRateLimitCmd = &cobra.Command{
		Use:     "ratelimit <TENANT-ID>",
		Short:   "Get a tenant's inference-gateway rate limit",
		Aliases: []string{"ratelimits", "quota"},
		Long: `Get the rate limit configured for a tenant.

Tenant quotas are enforced for requests admitted through a load-balancer
service configured with --api-key-auth=required.

ex)
	loxicmd get ratelimit tenant-a`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				fmt.Printf("Error: get ratelimit needs <TENANT-ID> arg\n")
				return
			}
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			resp, err := client.AITenantRatelimit().SubResources([]string{args[0]}).Get(ctx)
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

			entry := api.AITenantRateLimitEntry{}
			if err := json.Unmarshal(body, &entry); err != nil {
				fmt.Printf("Error: Failed to unmarshal HTTP response: (%s)\n", err.Error())
				return
			}
			if restOptions.PrintOption == "json" {
				indent, _ := json.MarshalIndent(entry, "", "    ")
				fmt.Println(string(indent))
				return
			}
			table := TableInit()
			wide := restOptions.PrintOption == "wide"
			if wide {
				table.SetHeader(RATELIMIT_WIDE_TITLE)
			} else {
				table.SetHeader(RATELIMIT_TITLE)
			}
			TableShow([][]string{rateLimitRow(entry, wide)}, table)
		},
	}
	return getRateLimitCmd
}

func rateLimitRow(entry api.AITenantRateLimitEntry, wide bool) []string {
	models := fmt.Sprintf("%d", len(entry.ModelLimits))
	if wide {
		models = formatTenantModelLimits(entry.ModelLimits)
	}
	return []string{
		entry.TenantID,
		fmt.Sprintf("%d", entry.Rps),
		fmt.Sprintf("%d", entry.TokensPerMin),
		fmt.Sprintf("%d", entry.BurstPct),
		models,
		entry.UpdatedAt,
	}
}

func formatTenantModelLimits(limits []api.AITenantModelRateLimit) string {
	values := make([]string, 0, len(limits))
	for _, limit := range limits {
		values = append(values, fmt.Sprintf("%s=%d", limit.Model, limit.TokensPerMin))
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}
