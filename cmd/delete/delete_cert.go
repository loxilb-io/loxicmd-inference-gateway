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

func NewDeleteCertCmd(restOptions *api.RESTOptions) *cobra.Command {
	var deleteCertCmd = &cobra.Command{
		Use:   "cert <CERT-ID>",
		Short: "Delete a TLS certificate by ID",
		Long: `Delete a TLS certificate.

ex)
	loxicmd delete cert web`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exitcode.Usagef("delete cert needs <CERT-ID> arg")
			}
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			resp, err := client.Cert().SubResources([]string{args[0]}).Delete(ctx)
			if err != nil {
				return exitcode.Unavailablef("delete cert: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
				ce := exitcode.FromHTTPStatus("delete cert", resp.StatusCode)
				ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
				return ce
			}
			fmt.Printf("Certificate '%s' deleted.\n", args[0])
			return nil
		},
	}
	return deleteCertCmd
}
