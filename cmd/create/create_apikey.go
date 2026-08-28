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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

type CreateAPIKeyOptions struct {
	TenantID      string
	Name          string
	AllowedModels []string
	Rps           int64
	Burst         int64
	TokensPerMin  int64
	ExpiresAt     string
	Enabled       bool
	APIKeyFile    string
	APIKeyStdin   bool
}

const (
	minImportedAPIKeyLength = 16
	maxImportedAPIKeyLength = 512
)

func NewCreateAPIKeyCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := CreateAPIKeyOptions{}

	var createAPIKeyCmd = &cobra.Command{
		Use:   "apikey --tenant-id=<tenant> [--name=<name>] [--api-key-file=<path>|--api-key-stdin] [--allowed-models=<m>,] [--rps=<n>] [--burst=<n>] [--tokens-per-min=<n>] [--expires-at=<RFC3339>] [--enabled]",
		Short: "Create an inference-gateway API key",
		Long: `Create a per-tenant inference-gateway API key.

Without an import option, the plaintext key (raw_key) is returned ONLY once at
creation time. Store it immediately. To register an existing credential without
exposing it in process arguments or shell history, use --api-key-file or pipe it
to --api-key-stdin.

Data-plane enforcement is enabled per load-balancer service with
--api-key-auth=required. Management Bearer authentication and data-plane
X-Api-Key credentials are separate identities.

ex)
	loxicmd create apikey --tenant-id=tenant-a --name=key-1 --allowed-models=llama-70b,mistral-7b --rps=5 --burst=10 --tokens-per-min=1000`,
		Run: func(cmd *cobra.Command, args []string) {
			if o.TenantID == "" {
				fmt.Printf("Error: --tenant-id is required\n")
				return
			}
			importedKey, imported, err := readImportedAPIKey(&o, cmd.InOrStdin())
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			enabled := o.Enabled
			req := api.AIApiKeyCreateRequest{
				TenantID:      o.TenantID,
				Name:          o.Name,
				APIKey:        importedKey,
				AllowedModels: o.AllowedModels,
				RateLimitRps:  o.Rps,
				BurstSize:     o.Burst,
				TokensPerMin:  o.TokensPerMin,
				ExpiresAt:     o.ExpiresAt,
				Enabled:       &enabled,
			}

			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.AIApiKey().Create(ctx, req)
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

			result := api.AIApiKeyCreateResponse{}
			if err := json.Unmarshal(body, &result); err != nil {
				fmt.Printf("Error: Failed to unmarshal HTTP response: (%s)\n", err.Error())
				return
			}
			if err := printCreateAPIKeyResult(cmd.OutOrStdout(), result, imported, restOptions.PrintOption); err != nil {
				fmt.Printf("Error: %s\n", err.Error())
			}
		},
	}

	createAPIKeyCmd.Flags().StringVar(&o.TenantID, "tenant-id", "", "Owner tenant ID (required)")
	createAPIKeyCmd.Flags().StringVar(&o.Name, "name", "", "Key label")
	createAPIKeyCmd.Flags().StringSliceVar(&o.AllowedModels, "allowed-models", o.AllowedModels, "Allowed model names (empty = all)")
	createAPIKeyCmd.Flags().Int64Var(&o.Rps, "rps", 0, "Per-key rate limit (requests/sec)")
	createAPIKeyCmd.Flags().Int64Var(&o.Burst, "burst", 0, "Per-key burst size")
	createAPIKeyCmd.Flags().Int64Var(&o.TokensPerMin, "tokens-per-min", 0, "Per-key token budget per minute")
	createAPIKeyCmd.Flags().StringVar(&o.ExpiresAt, "expires-at", "", "Expiry timestamp (RFC3339)")
	createAPIKeyCmd.Flags().BoolVar(&o.Enabled, "enabled", true, "Whether the key is enabled")
	createAPIKeyCmd.Flags().StringVar(&o.APIKeyFile, "api-key-file", "", "Read an existing API key from a file")
	createAPIKeyCmd.Flags().BoolVar(&o.APIKeyStdin, "api-key-stdin", false, "Read an existing API key from stdin")

	return createAPIKeyCmd
}

func readImportedAPIKey(o *CreateAPIKeyOptions, stdin io.Reader) (string, bool, error) {
	if o.APIKeyFile != "" && o.APIKeyStdin {
		return "", false, fmt.Errorf("--api-key-file and --api-key-stdin are mutually exclusive")
	}
	if o.APIKeyFile == "" && !o.APIKeyStdin {
		return "", false, nil
	}

	var reader io.Reader = stdin
	var file *os.File
	if o.APIKeyFile != "" {
		var err error
		file, err = os.Open(o.APIKeyFile)
		if err != nil {
			return "", false, fmt.Errorf("read API key file: %w", err)
		}
		defer file.Close()
		reader = file
	}

	b, err := io.ReadAll(io.LimitReader(reader, maxImportedAPIKeyLength+2))
	if err != nil {
		return "", false, fmt.Errorf("read imported API key: %w", err)
	}
	key := strings.TrimSpace(string(b))
	if len(key) < minImportedAPIKeyLength || len(key) > maxImportedAPIKeyLength {
		return "", false, fmt.Errorf("imported API key must be between %d and %d characters", minImportedAPIKeyLength, maxImportedAPIKeyLength)
	}
	for i := 0; i < len(key); i++ {
		if key[i] < 0x21 || key[i] > 0x7e {
			return "", false, fmt.Errorf("imported API key must contain only printable non-space ASCII characters")
		}
	}
	return key, true, nil
}

func printCreateAPIKeyResult(w io.Writer, result api.AIApiKeyCreateResponse, imported bool, printOption string) error {
	if imported {
		// Imported material is user-supplied and must never be echoed, even if a
		// server implementation includes it in the response.
		result.RawKey = ""
	}
	if printOption == "json" {
		indent, err := json.MarshalIndent(result, "", "    ")
		if err != nil {
			return fmt.Errorf("marshal API key response: %w", err)
		}
		_, err = fmt.Fprintln(w, string(indent))
		return err
	}
	if imported {
		_, err := fmt.Fprintf(w, "API key imported.\n  key_id : %s\n", result.KeyID)
		return err
	}
	_, err := fmt.Fprintf(w, "API key created.\n  key_id : %s\n  raw_key: %s\nStore the raw_key now - it will not be shown again.\n", result.KeyID, result.RawKey)
	return err
}

// aiContext builds a context with the standard REST timeout.
func aiContext(restOptions *api.RESTOptions) (context.Context, context.CancelFunc) {
	if restOptions.Timeout > 0 {
		return context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
	}
	return context.TODO(), nil
}
