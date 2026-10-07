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
	"encoding/json"
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
	Usage     string
}

// certCreateBody checks the files a usage needs and builds the upload. A CA
// bundle has no key; a server or client certificate needs one.
func certCreateBody(o *CreateCertOptions, certPem, keyPem, chainPem string) (any, error) {
	switch o.Usage {
	case "", api.CertUsageServer, api.CertUsageClient:
		if o.KeyFile == "" {
			return nil, exitcode.Usagef("--cert-file and --key-file are required")
		}
		return api.CertModel{CertID: o.CertID, Usage: o.Usage, CertPem: certPem, KeyPem: keyPem, ChainPem: chainPem}, nil
	case api.CertUsageCA:
		if o.KeyFile != "" {
			return nil, exitcode.Usagef("--usage ca takes a certificate bundle and no --key-file")
		}
		return api.CACertModel{CertID: o.CertID, Usage: o.Usage, CertPem: certPem, ChainPem: chainPem}, nil
	}
	return nil, exitcode.Usagef("--usage must be one of server|ca|client")
}

// certCreatedID is the ID a created certificate is stored under: the one
// the gateway answers with, which is the only way to learn an ID it minted.
// A gateway that answers without a body leaves the ID the request named.
func certCreatedID(body []byte, requested string) string {
	var created struct {
		CertID string `json:"certId"`
	}
	if json.Unmarshal(body, &created) == nil && created.CertID != "" {
		return created.CertID
	}
	return requested
}

func NewCreateCertCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := CreateCertOptions{}

	var createCertCmd = &cobra.Command{
		Use:   "cert --cert-file=<pem> [--key-file=<pem>] [--chain-file=<pem>] [--cert-id=<id>] [--usage=<server|ca|client>]",
		Short: "Create (upload) a TLS certificate",
		Long: `Upload a TLS certificate (leaf + private key, optional chain) to the gateway.
If --cert-id is omitted the server mints one. The private key is never returned on get.

--usage says what the entry is for:
  server  (default) terminates TLS on a listener
  ca      a CA bundle a backend certificate is verified against; no --key-file
  client  the certificate the gateway presents to a backend

A load-balancer rule names a ca entry with --backend-ca-cert-id and a client
entry with --backend-client-cert-id.

ex)
	loxicmd create cert --cert-file=server.crt --key-file=server.key --chain-file=chain.pem --cert-id=web
	loxicmd create cert --usage=ca --cert-file=backend-ca.pem --cert-id=backend-ca
	loxicmd create cert --usage=client --cert-file=client.crt --key-file=client.key --cert-id=backend-client`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if o.CertFile == "" {
				return exitcode.Usagef("--cert-file is required")
			}
			if _, err := certCreateBody(&o, "", "", ""); err != nil {
				return err
			}
			// An unreadable input file is a missing precondition, per the
			// taxonomy's file-read row.
			certPem, err := os.ReadFile(o.CertFile)
			if err != nil {
				return &exitcode.CLIError{Code: exitcode.Precondition, Message: fmt.Sprintf("reading --cert-file: %v", err)}
			}
			var keyPem, chainPem []byte
			if o.KeyFile != "" {
				if keyPem, err = os.ReadFile(o.KeyFile); err != nil {
					return &exitcode.CLIError{Code: exitcode.Precondition, Message: fmt.Sprintf("reading --key-file: %v", err)}
				}
			}
			if o.ChainFile != "" {
				if chainPem, err = os.ReadFile(o.ChainFile); err != nil {
					return &exitcode.CLIError{Code: exitcode.Precondition, Message: fmt.Sprintf("reading --chain-file: %v", err)}
				}
			}
			model, err := certCreateBody(&o, string(certPem), string(keyPem), string(chainPem))
			if err != nil {
				return err
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
			if id := certCreatedID(body, o.CertID); id != "" {
				fmt.Printf("Certificate '%s' created.\n", id)
			} else {
				fmt.Printf("Certificate created.\n")
			}
			return nil
		},
	}
	createCertCmd.Flags().StringVar(&o.CertID, "cert-id", "", "Certificate ID (optional; server mints one if omitted)")
	createCertCmd.Flags().StringVar(&o.CertFile, "cert-file", "", "Leaf certificate PEM file (required)")
	createCertCmd.Flags().StringVar(&o.KeyFile, "key-file", "", "Private key PEM file (required, except with --usage ca)")
	createCertCmd.Flags().StringVar(&o.ChainFile, "chain-file", "", "Intermediate chain PEM file")
	createCertCmd.Flags().StringVar(&o.Usage, "usage", "", "What the entry is for: server (default), ca or client")
	return createCertCmd
}
