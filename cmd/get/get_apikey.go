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
	"strings"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

func NewGetAPIKeyCmd(restOptions *api.RESTOptions) *cobra.Command {
	var tenantID string

	var getAPIKeyCmd = &cobra.Command{
		Use:     "apikey [KEY-ID] [--tenant-id=<tenant>]",
		Short:   "Get inference-gateway API keys",
		Aliases: []string{"apikeys"},
		Long: `List API keys for a tenant, or get a single key by ID.

The raw key is never shown here; it is only returned once at creation.

ex)
	loxicmd get apikey --tenant-id=tenant-a
	loxicmd get apikey lxb_abc123`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}

			var resp *http.Response
			var err error
			single := len(args) == 1
			if single {
				resp, err = client.AIApiKey().SubResources([]string{args[0]}).Get(ctx)
			} else {
				q := map[string]string{}
				if tenantID != "" {
					q["tenant_id"] = tenantID
				}
				resp, err = client.AIApiKey().Query(q).Get(ctx)
			}
			if err != nil {
				return exitcode.Unavailablef("get apikey: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				ce := exitcode.FromHTTPStatus("get apikey", resp.StatusCode)
				ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
				return ce
			}
			PrintGetAPIKeyResult(body, *restOptions, single)
			return nil
		},
	}
	getAPIKeyCmd.Flags().StringVar(&tenantID, "tenant-id", "", "Filter keys by tenant ID")
	return getAPIKeyCmd
}

func PrintGetAPIKeyResult(body []byte, o api.RESTOptions, single bool) {
	var keys []api.AIApiKeySummary
	if single {
		var one api.AIApiKeySummary
		if err := json.Unmarshal(body, &one); err != nil {
			fmt.Printf("Error: Failed to unmarshal HTTP response: (%s)\n", err.Error())
			return
		}
		keys = []api.AIApiKeySummary{one}
	} else {
		if err := json.Unmarshal(body, &keys); err != nil {
			fmt.Printf("Error: Failed to unmarshal HTTP response: (%s)\n", err.Error())
			return
		}
	}

	if o.PrintOption == "json" {
		indent, _ := json.MarshalIndent(keys, "", "    ")
		fmt.Println(string(indent))
		return
	}

	table := TableInit()
	wide := o.PrintOption == "wide"
	if wide {
		table.SetHeader(APIKEY_WIDE_TITLE)
	} else {
		table.SetHeader(APIKEY_TITLE)
	}
	var data [][]string
	for _, k := range keys {
		row := []string{
			k.KeyID, k.TenantID, k.Name, strings.Join(k.AllowedModels, ","),
			fmt.Sprintf("%d", k.RateLimitRps), fmt.Sprintf("%d", k.BurstSize),
			fmt.Sprintf("%d", k.TokensPerMin), fmt.Sprintf("%v", k.Enabled),
		}
		if wide {
			row = append(row, k.CreatedAt, k.ExpiresAt)
		}
		data = append(data, row)
	}
	TableShow(data, table)
}
