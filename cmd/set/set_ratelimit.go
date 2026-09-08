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
	"strconv"
	"strings"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

type SetRateLimitOptions struct {
	TenantID     string
	Rps          int64
	TokensPerMin int64
	BurstPct     int64
	ModelLimits  []string
}

func NewSetRateLimitCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := SetRateLimitOptions{}

	var setRateLimitCmd = &cobra.Command{
		Use:   "ratelimit --tenant-id=<tenant> [--rps=<n>] [--tokens-per-min=<n>] [--burst-pct=<n>] [--model-limit=<model>=<tokens>]...",
		Short: "Set a tenant's inference-gateway rate limit",
		Long: `Create or update the rate limit for a tenant.

Tenant quotas are enforced for requests admitted through a load-balancer
service configured with --api-key-auth=required. A model token value of zero
removes that model-specific quota.

ex)
	loxicmd set ratelimit --tenant-id=tenant-a --rps=50 --tokens-per-min=2000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if o.TenantID == "" {
				return exitcode.Usagef("--tenant-id is required")
			}
			if strings.Contains(o.TenantID, "|") {
				return exitcode.Invalidf("--tenant-id must not contain '|'")
			}
			if o.Rps < 0 || o.TokensPerMin < 0 || o.BurstPct < 0 {
				return exitcode.Invalidf("--rps, --tokens-per-min, and --burst-pct must be non-negative")
			}
			modelLimits, err := parseTenantModelLimits(o.ModelLimits)
			if err != nil {
				return exitcode.Invalidf("%s", err.Error())
			}
			req := api.AITenantRateLimitMod{
				TenantID: o.TenantID, Rps: o.Rps, TokensPerMin: o.TokensPerMin,
				BurstPct: o.BurstPct, ModelLimits: modelLimits,
			}

			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			resp, err := client.AITenantRatelimit().Create(ctx, req)
			if err != nil {
				return exitcode.Unavailablef("set ratelimit: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
				ce := exitcode.FromHTTPStatus("set ratelimit", resp.StatusCode)
				ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
				return ce
			}
			fmt.Printf("Rate limit for tenant '%s' set.\n", o.TenantID)
			return nil
		},
	}
	setRateLimitCmd.Flags().StringVar(&o.TenantID, "tenant-id", "", "Tenant ID (required)")
	setRateLimitCmd.Flags().Int64Var(&o.Rps, "rps", 0, "Requests per second")
	setRateLimitCmd.Flags().Int64Var(&o.TokensPerMin, "tokens-per-min", 0, "Tokens per minute")
	setRateLimitCmd.Flags().Int64Var(&o.BurstPct, "burst-pct", 0, "Token-bucket capacity as a percent of tokens/min (0 = server default)")
	setRateLimitCmd.Flags().StringArrayVar(&o.ModelLimits, "model-limit", nil, "Per-model token quota MODEL=TOKENS; repeatable; TOKENS=0 removes")
	return setRateLimitCmd
}

func parseTenantModelLimits(values []string) ([]api.AITenantModelRateLimit, error) {
	limits := make([]api.AITenantModelRateLimit, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		model, rawTokens, ok := strings.Cut(value, "=")
		model = strings.TrimSpace(model)
		rawTokens = strings.TrimSpace(rawTokens)
		if !ok || model == "" || rawTokens == "" {
			return nil, fmt.Errorf("--model-limit %q must use MODEL=TOKENS", value)
		}
		if strings.Contains(model, "|") {
			return nil, fmt.Errorf("--model-limit model %q must not contain '|'", model)
		}
		if seen[model] {
			return nil, fmt.Errorf("--model-limit repeats model %q", model)
		}
		tokens, err := strconv.ParseInt(rawTokens, 10, 64)
		if err != nil || tokens < 0 {
			return nil, fmt.Errorf("--model-limit %q has invalid non-negative token quota", value)
		}
		seen[model] = true
		limits = append(limits, api.AITenantModelRateLimit{Model: model, TokensPerMin: tokens})
	}
	return limits, nil
}
