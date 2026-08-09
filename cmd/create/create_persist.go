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
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewCreatePersistCmd(restOptions *api.RESTOptions) *cobra.Command {
	var createPersistCmd = &cobra.Command{
		Use:   "persist",
		Short: "Persist the running configuration to disk",
		Long: `Dump the gateway's live configuration to {config-path}/snapshot.json so it
survives a daemon restart (/config/persist). This is the API-side "save".

ex)
	loxicmd create persist`,
		Run: func(cmd *cobra.Command, args []string) {
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}

			resp, err := client.Persist().Create(ctx, nil)
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
			res := api.PersistResult{}
			if err := json.Unmarshal(body, &res); err != nil {
				fmt.Printf("Configuration persisted.\n")
				return
			}
			if res.Path != "" {
				fmt.Printf("Configuration persisted to %s (checksum %s).\n", res.Path, res.Checksum)
			} else {
				fmt.Printf("Configuration persisted.\n")
			}
		},
	}
	return createPersistCmd
}
