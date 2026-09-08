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
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

func NewGetSNICmd(restOptions *api.RESTOptions) *cobra.Command {
	var getSNICmd = &cobra.Command{
		Use:     "sni",
		Short:   "List SNI certificate mappings",
		Aliases: []string{"snicert", "snicerts"},
		Long:    `List hostname -> certificate-directory mappings in the SNI store.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			resp, err := client.SNICertificate().Get(ctx)
			if err != nil {
				return exitcode.Unavailablef("get sni: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				ce := exitcode.FromHTTPStatus("get sni", resp.StatusCode)
				ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
				return ce
			}
			list := api.SNICertificateListResponse{}
			if err := json.Unmarshal(body, &list); err != nil {
				return &exitcode.CLIError{
					Code:       exitcode.ContractMismatch,
					Message:    fmt.Sprintf("Failed to unmarshal HTTP response: (%s)", err.Error()),
					Origin:     "gateway",
					HTTPStatus: resp.StatusCode,
				}
			}
			if restOptions.PrintOption == "json" {
				indent, _ := json.MarshalIndent(list, "", "    ")
				fmt.Println(string(indent))
				return nil
			}
			table := TableInit()
			table.SetHeader(SNI_TITLE)
			var data [][]string
			for _, c := range list.Certificates {
				data = append(data, []string{c.Hostname, c.CertPath, fmt.Sprintf("%d", c.RefCount)})
			}
			TableShow(data, table)
			return nil
		},
	}
	return getSNICmd
}
