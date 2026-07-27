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
package delete

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewDeleteAPIKeyCmd(restOptions *api.RESTOptions) *cobra.Command {
	var deleteAPIKeyCmd = &cobra.Command{
		Use:   "apikey <KEY-ID>",
		Short: "Delete (revoke) an inference-gateway API key",
		Long: `Delete a per-tenant inference-gateway API key by its key ID.

ex)
	loxicmd delete apikey lxb_abc123`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				fmt.Printf("Error: delete apikey needs <KEY-ID> arg\n")
				return
			}
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			resp, err := client.AIApiKey().SubResources([]string{args[0]}).Delete(ctx)
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
			fmt.Printf("API key '%s' deleted.\n", args[0])
		},
	}
	return deleteAPIKeyCmd
}
