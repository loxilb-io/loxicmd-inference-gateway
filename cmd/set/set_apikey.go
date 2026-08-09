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

func NewSetAPIKeyCmd(restOptions *api.RESTOptions) *cobra.Command {
	var allowedModels []string
	var enabled bool

	var setAPIKeyCmd = &cobra.Command{
		Use:   "apikey <KEY-ID> [--allowed-models=<m>,] [--enabled]",
		Short: "Update an inference-gateway API key",
		Long: `Update the allowed model list and/or enabled state of an API key (PATCH).
Only the flags you set are changed.

ex)
	loxicmd set apikey lxb_abc123 --allowed-models=mistral-7b
	loxicmd set apikey lxb_abc123 --enabled=false`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				fmt.Printf("Error: set apikey needs <KEY-ID> arg\n")
				return
			}
			if !cmd.Flags().Changed("allowed-models") && !cmd.Flags().Changed("enabled") {
				fmt.Printf("Error: set at least one of --allowed-models or --enabled\n")
				return
			}
			req := api.AIApiKeyPatchRequest{}
			if cmd.Flags().Changed("allowed-models") {
				req.AllowedModels = allowedModels
			}
			if cmd.Flags().Changed("enabled") {
				e := enabled
				req.Enabled = &e
			}

			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			resp, err := client.AIApiKey().SubResources([]string{args[0]}).Update(ctx, req)
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
			fmt.Printf("API key '%s' updated.\n", args[0])
		},
	}
	setAPIKeyCmd.Flags().StringSliceVar(&allowedModels, "allowed-models", allowedModels, "Replacement allowed model list")
	setAPIKeyCmd.Flags().BoolVar(&enabled, "enabled", true, "Enable or disable the key")
	return setAPIKeyCmd
}
