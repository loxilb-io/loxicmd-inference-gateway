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
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewSetRateLimitCmd(restOptions *api.RESTOptions) *cobra.Command {
	var tenantID string
	var rps int64
	var tokensPerMin int64

	var setRateLimitCmd = &cobra.Command{
		Use:   "ratelimit --tenant-id=<tenant> [--rps=<n>] [--tokens-per-min=<n>]",
		Short: "Set a tenant's inference-gateway rate limit",
		Long: `Create or update the rate limit for a tenant.

Note: tenant rate limits are control-plane CRUD today; data-plane enforcement
(429) is on the roadmap.

ex)
	loxicmd set ratelimit --tenant-id=tenant-a --rps=50 --tokens-per-min=2000`,
		Run: func(cmd *cobra.Command, args []string) {
			if tenantID == "" {
				fmt.Printf("Error: --tenant-id is required\n")
				return
			}
			req := api.AITenantRateLimitMod{TenantID: tenantID, Rps: rps, TokensPerMin: tokensPerMin}

			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			resp, err := client.AITenantRatelimit().Create(ctx, req)
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
				fmt.Printf("Error: %s\n", api.NewAPIError(resp.StatusCode, body).Error())
				return
			}
			fmt.Printf("Rate limit for tenant '%s' set.\n", tenantID)
		},
	}
	setRateLimitCmd.Flags().StringVar(&tenantID, "tenant-id", "", "Tenant ID (required)")
	setRateLimitCmd.Flags().Int64Var(&rps, "rps", 0, "Requests per second")
	setRateLimitCmd.Flags().Int64Var(&tokensPerMin, "tokens-per-min", 0, "Tokens per minute")
	return setRateLimitCmd
}
