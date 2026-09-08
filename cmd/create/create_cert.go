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
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

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
		RunE: func(cmd *cobra.Command, args []string) error {
			if o.CertFile == "" || o.KeyFile == "" {
				return exitcode.Usagef("--cert-file and --key-file are required")
			}
			// An unreadable input file is a missing precondition, per the
			// taxonomy's file-read row.
			certPem, err := os.ReadFile(o.CertFile)
			if err != nil {
				return &exitcode.CLIError{Code: exitcode.Precondition, Message: fmt.Sprintf("reading --cert-file: %v", err)}
			}
			keyPem, err := os.ReadFile(o.KeyFile)
			if err != nil {
				return &exitcode.CLIError{Code: exitcode.Precondition, Message: fmt.Sprintf("reading --key-file: %v", err)}
			}
			model := api.CertModel{CertID: o.CertID, CertPem: string(certPem), KeyPem: string(keyPem)}
			if o.ChainFile != "" {
				chainPem, err := os.ReadFile(o.ChainFile)
				if err != nil {
					return &exitcode.CLIError{Code: exitcode.Precondition, Message: fmt.Sprintf("reading --chain-file: %v", err)}
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
				return exitcode.Unavailablef("create cert: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				ce := exitcode.FromHTTPStatus("create cert", resp.StatusCode)
				ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
				return ce
			}
			if o.CertID != "" {
				fmt.Printf("Certificate '%s' created.\n", o.CertID)
			} else {
				fmt.Printf("Certificate created.\n")
			}
			return nil
		},
	}
	createCertCmd.Flags().StringVar(&o.CertID, "cert-id", "", "Certificate ID (optional; server mints one if omitted)")
	createCertCmd.Flags().StringVar(&o.CertFile, "cert-file", "", "Leaf certificate PEM file (required)")
	createCertCmd.Flags().StringVar(&o.KeyFile, "key-file", "", "Private key PEM file (required)")
	createCertCmd.Flags().StringVar(&o.ChainFile, "chain-file", "", "Intermediate chain PEM file")
	return createCertCmd
}
