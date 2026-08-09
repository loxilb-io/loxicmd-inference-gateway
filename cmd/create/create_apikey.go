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
package create

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

type CreateAPIKeyOptions struct {
	TenantID      string
	Name          string
	AllowedModels []string
	Rps           int64
	Burst         int64
	TokensPerMin  int64
	ExpiresAt     string
	Enabled       bool
}

func NewCreateAPIKeyCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := CreateAPIKeyOptions{}

	var createAPIKeyCmd = &cobra.Command{
		Use:   "apikey --tenant-id=<tenant> [--name=<name>] [--allowed-models=<m>,] [--rps=<n>] [--burst=<n>] [--tokens-per-min=<n>] [--expires-at=<RFC3339>] [--enabled]",
		Short: "Create an inference-gateway API key",
		Long: `Create a per-tenant inference-gateway API key.

The plaintext key (raw_key) is returned ONLY once, at creation time - store it now.

Note: API keys are control-plane CRUD today; data-plane enforcement
(401/403 on invalid key or disallowed model) is on the roadmap.

ex)
	loxicmd create apikey --tenant-id=tenant-a --name=key-1 --allowed-models=llama-70b,mistral-7b --rps=5 --burst=10 --tokens-per-min=1000`,
		Run: func(cmd *cobra.Command, args []string) {
			if o.TenantID == "" {
				fmt.Printf("Error: --tenant-id is required\n")
				return
			}
			enabled := o.Enabled
			req := api.AIApiKeyCreateRequest{
				TenantID:      o.TenantID,
				Name:          o.Name,
				AllowedModels: o.AllowedModels,
				RateLimitRps:  o.Rps,
				BurstSize:     o.Burst,
				TokensPerMin:  o.TokensPerMin,
				ExpiresAt:     o.ExpiresAt,
				Enabled:       &enabled,
			}

			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.AIApiKey().Create(ctx, req)
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				fmt.Printf("Error: %s\n", api.NewAPIError(resp.StatusCode, body).Error())
				return
			}

			result := api.AIApiKeyCreateResponse{}
			if err := json.Unmarshal(body, &result); err != nil {
				fmt.Printf("Error: Failed to unmarshal HTTP response: (%s)\n", err.Error())
				return
			}
			if restOptions.PrintOption == "json" {
				indent, _ := json.MarshalIndent(result, "", "    ")
				fmt.Println(string(indent))
				return
			}
			fmt.Printf("API key created.\n  key_id : %s\n  raw_key: %s\n", result.KeyID, result.RawKey)
			fmt.Printf("Store the raw_key now - it will not be shown again.\n")
		},
	}

	createAPIKeyCmd.Flags().StringVar(&o.TenantID, "tenant-id", "", "Owner tenant ID (required)")
	createAPIKeyCmd.Flags().StringVar(&o.Name, "name", "", "Key label")
	createAPIKeyCmd.Flags().StringSliceVar(&o.AllowedModels, "allowed-models", o.AllowedModels, "Allowed model names (empty = all)")
	createAPIKeyCmd.Flags().Int64Var(&o.Rps, "rps", 0, "Per-key rate limit (requests/sec)")
	createAPIKeyCmd.Flags().Int64Var(&o.Burst, "burst", 0, "Per-key burst size")
	createAPIKeyCmd.Flags().Int64Var(&o.TokensPerMin, "tokens-per-min", 0, "Per-key token budget per minute")
	createAPIKeyCmd.Flags().StringVar(&o.ExpiresAt, "expires-at", "", "Expiry timestamp (RFC3339)")
	createAPIKeyCmd.Flags().BoolVar(&o.Enabled, "enabled", true, "Whether the key is enabled")

	return createAPIKeyCmd
}

// aiContext builds a context with the standard REST timeout.
func aiContext(restOptions *api.RESTOptions) (context.Context, context.CancelFunc) {
	if restOptions.Timeout > 0 {
		return context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
	}
	return context.TODO(), nil
}
