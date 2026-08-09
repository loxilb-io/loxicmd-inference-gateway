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

	"github.com/spf13/cobra"
)

func NewGetSNICmd(restOptions *api.RESTOptions) *cobra.Command {
	var getSNICmd = &cobra.Command{
		Use:     "sni",
		Short:   "List SNI certificate mappings",
		Aliases: []string{"snicert", "snicerts"},
		Long:    `List hostname -> certificate-directory mappings in the SNI store.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			resp, err := client.SNICertificate().Get(ctx)
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				fmt.Printf("Error: %s\n", api.NewAPIError(resp.StatusCode, body).Error())
				return
			}
			list := api.SNICertificateListResponse{}
			if err := json.Unmarshal(body, &list); err != nil {
				fmt.Printf("Error: Failed to unmarshal HTTP response: (%s)\n", err.Error())
				return
			}
			if restOptions.PrintOption == "json" {
				indent, _ := json.MarshalIndent(list, "", "    ")
				fmt.Println(string(indent))
				return
			}
			table := TableInit()
			table.SetHeader(SNI_TITLE)
			var data [][]string
			for _, c := range list.Certificates {
				data = append(data, []string{c.Hostname, c.CertPath, fmt.Sprintf("%d", c.RefCount)})
			}
			TableShow(data, table)
		},
	}
	return getSNICmd
}
