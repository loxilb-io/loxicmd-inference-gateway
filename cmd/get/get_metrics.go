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

func NewGetMetricsCmd(restOptions *api.RESTOptions) *cobra.Command {
	var getMetricsCmd = &cobra.Command{
		Use:   "metrics",
		Short: "Show whether Prometheus metrics are enabled",
		Long:  `Report the gateway's Prometheus metrics on/off state.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			resp, err := client.Metrics().Get(ctx)
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
			cfg := api.MetricsConfig{}
			if err := json.Unmarshal(body, &cfg); err != nil {
				fmt.Printf("Error: Failed to unmarshal HTTP response: (%s)\n", err.Error())
				return
			}
			if restOptions.PrintOption == "json" {
				indent, _ := json.MarshalIndent(cfg, "", "    ")
				fmt.Println(string(indent))
				return
			}
			state := "disabled"
			if cfg.Prometheus {
				state = "enabled"
			}
			fmt.Printf("prometheus metrics: %s\n", state)
		},
	}
	return getMetricsCmd
}
