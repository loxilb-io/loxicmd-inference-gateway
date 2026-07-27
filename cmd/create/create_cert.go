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
	"os"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

type CreateCertOptions struct {
	CertID    string
	CertFile  string
	KeyFile   string
	ChainFile string
}

func NewCreateCertCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := CreateCertOptions{}

	var createCertCmd = &cobra.Command{
		Use:   "cert --cert-file=<pem> --key-file=<pem> [--chain-file=<pem>] [--cert-id=<id>]",
		Short: "Create (upload) a TLS certificate",
		Long: `Upload a TLS certificate (leaf + private key, optional chain) to the gateway.
If --cert-id is omitted the server mints one. The private key is never returned on get.

ex)
	loxicmd create cert --cert-file=server.crt --key-file=server.key --chain-file=chain.pem --cert-id=web`,
		Run: func(cmd *cobra.Command, args []string) {
			if o.CertFile == "" || o.KeyFile == "" {
				fmt.Printf("Error: --cert-file and --key-file are required\n")
				return
			}
			certPem, err := os.ReadFile(o.CertFile)
			if err != nil {
				fmt.Printf("Error: reading --cert-file: %s\n", err.Error())
				return
			}
			keyPem, err := os.ReadFile(o.KeyFile)
			if err != nil {
				fmt.Printf("Error: reading --key-file: %s\n", err.Error())
				return
			}
			model := api.CertModel{CertID: o.CertID, CertPem: string(certPem), KeyPem: string(keyPem)}
			if o.ChainFile != "" {
				chainPem, err := os.ReadFile(o.ChainFile)
				if err != nil {
					fmt.Printf("Error: reading --chain-file: %s\n", err.Error())
					return
				}
				model.ChainPem = string(chainPem)
			}

			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.Cert().Create(ctx, model)
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				fmt.Printf("Error: %s\n", api.NewAPIError(resp.StatusCode, body).Error())
				return
			}
			if o.CertID != "" {
				fmt.Printf("Certificate '%s' created.\n", o.CertID)
			} else {
				fmt.Printf("Certificate created.\n")
			}
		},
	}
	createCertCmd.Flags().StringVar(&o.CertID, "cert-id", "", "Certificate ID (optional; server mints one if omitted)")
	createCertCmd.Flags().StringVar(&o.CertFile, "cert-file", "", "Leaf certificate PEM file (required)")
	createCertCmd.Flags().StringVar(&o.KeyFile, "key-file", "", "Private key PEM file (required)")
	createCertCmd.Flags().StringVar(&o.ChainFile, "chain-file", "", "Intermediate chain PEM file")
	return createCertCmd
}
