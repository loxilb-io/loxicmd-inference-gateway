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

func NewDeleteSNICmd(restOptions *api.RESTOptions) *cobra.Command {
	var hostname string

	var deleteSNICmd = &cobra.Command{
		Use:     "sni --hostname=<host>",
		Short:   "Delete an SNI certificate mapping",
		Aliases: []string{"snicert"},
		Long: `Delete a hostname's SNI certificate mapping.

ex)
	loxicmd delete sni --hostname=api.example.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if hostname == "" {
				return exitcode.Usagef("--hostname is required")
			}
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			// The SNI DELETE identifies the target in the request body.
			resp, err := client.SNICertificate().DeleteWithBody(ctx, api.SNICertificateDeleteRequest{Hostname: hostname})
			if err != nil {
				return exitcode.Unavailablef("delete sni: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
				ce := exitcode.FromHTTPStatus("delete sni", resp.StatusCode)
				ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
				return ce
			}
			fmt.Printf("SNI certificate for '%s' deleted.\n", hostname)
			return nil
		},
	}
	deleteSNICmd.Flags().StringVar(&hostname, "hostname", "", "SNI hostname (required)")
	return deleteSNICmd
}
