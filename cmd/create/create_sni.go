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
	"fmt"
	"io"
	"net/http"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewCreateSNICmd(restOptions *api.RESTOptions) *cobra.Command {
	var hostname string
	var certPath string

	var createSNICmd = &cobra.Command{
		Use:     "sni --hostname=<host> [--cert-path=<dir>]",
		Short:   "Register an SNI certificate mapping",
		Aliases: []string{"snicert"},
		Long: `Register a hostname -> certificate-directory mapping in the SNI store.
The directory must contain server.crt, server.key (and optional rootCA.crt for mTLS).
If --cert-path is omitted the gateway uses /opt/loxilb/cert/<hostname>.

ex)
	loxicmd create sni --hostname=api.example.com --cert-path=/opt/loxilb/cert`,
		Run: func(cmd *cobra.Command, args []string) {
			if hostname == "" {
				fmt.Printf("Error: --hostname is required\n")
				return
			}
			req := api.SNICertificateEntry{Hostname: hostname, CertPath: certPath}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.SNICertificate().Create(ctx, req)
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				fmt.Printf("Error: %s\n", api.NewAPIError(resp.StatusCode, body).Error())
				return
			}
			fmt.Printf("SNI certificate registered for '%s'.\n", hostname)
		},
	}
	createSNICmd.Flags().StringVar(&hostname, "hostname", "", "SNI hostname (required)")
	createSNICmd.Flags().StringVar(&certPath, "cert-path", "", "Certificate directory (default /opt/loxilb/cert/<hostname>)")
	return createSNICmd
}
