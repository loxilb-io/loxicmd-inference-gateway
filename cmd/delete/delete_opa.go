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
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

func NewDeleteOPACmd(restOptions *api.RESTOptions) *cobra.Command {
	var deleteOPACmd = &cobra.Command{
		Use:   "opa",
		Short: "Stop and remove the OPA L4 policy watcher",
		Long: `Stop the OPA L4 policy watcher if running and remove it
(/config/opa/watcher). Succeeds even when no watcher is configured.

Note: this is a raw-middleware endpoint; configure it with 'set opa'.

ex)
	loxicmd delete opa`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}

			resp, err := client.OPA().Delete(ctx)
			if err != nil {
				return exitcode.Unavailablef("delete opa: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
				ce := exitcode.FromHTTPStatus("delete opa", resp.StatusCode)
				ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
				return ce
			}
			fmt.Printf("OPA policy watcher stopped.\n")
			return nil
		},
	}
	return deleteOPACmd
}
