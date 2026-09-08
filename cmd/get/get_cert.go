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

func NewGetCertCmd(restOptions *api.RESTOptions) *cobra.Command {
	var getCertCmd = &cobra.Command{
		Use:   "cert <CERT-ID>",
		Short: "Get a TLS certificate by ID",
		Long: `Show a TLS certificate's metadata (the private key is never returned).
There is no list endpoint; a certificate ID is required.

ex)
	loxicmd get cert web`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exitcode.Usagef("get cert needs <CERT-ID> arg")
			}
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			resp, err := client.Cert().SubResources([]string{args[0]}).Get(ctx)
			if err != nil {
				return exitcode.Unavailablef("get cert: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				ce := exitcode.FromHTTPStatus("get cert", resp.StatusCode)
				ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
				return ce
			}
			cert := api.CertModel{}
			if err := json.Unmarshal(body, &cert); err != nil {
				return &exitcode.CLIError{
					Code:       exitcode.ContractMismatch,
					Message:    fmt.Sprintf("Failed to unmarshal HTTP response: (%s)", err.Error()),
					Origin:     "gateway",
					HTTPStatus: resp.StatusCode,
				}
			}
			if restOptions.PrintOption == "json" {
				indent, _ := json.MarshalIndent(cert, "", "    ")
				fmt.Println(string(indent))
				return nil
			}
			table := TableInit()
			table.SetHeader(CERT_TITLE)
			TableShow([][]string{{cert.CertID, strings.Join(cert.Hostnames, ",")}}, table)
			return nil
		},
	}
	return getCertCmd
}
