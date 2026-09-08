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
package set

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

func NewSetMetricsCmd(restOptions *api.RESTOptions) *cobra.Command {
	var enable bool
	var disable bool

	var setMetricsCmd = &cobra.Command{
		Use:   "metrics (--enable | --disable)",
		Short: "Enable or disable Prometheus metrics",
		Long: `Toggle the gateway's Prometheus metrics endpoint (/netlox/v1/metrics).

ex)
	loxicmd set metrics --enable
	loxicmd set metrics --disable`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if enable == disable {
				return exitcode.Usagef("specify exactly one of --enable or --disable")
			}
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}

			var resp *http.Response
			var err error
			if enable {
				resp, err = client.Metrics().Create(ctx, nil) // bodyless POST enables
			} else {
				resp, err = client.Metrics().Delete(ctx) // DELETE disables
			}
			if err != nil {
				return exitcode.Unavailablef("set metrics: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
				ce := exitcode.FromHTTPStatus("set metrics", resp.StatusCode)
				ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
				return ce
			}
			if enable {
				fmt.Printf("Prometheus metrics enabled.\n")
			} else {
				fmt.Printf("Prometheus metrics disabled.\n")
			}
			return nil
		},
	}
	setMetricsCmd.Flags().BoolVar(&enable, "enable", false, "Enable Prometheus metrics")
	setMetricsCmd.Flags().BoolVar(&disable, "disable", false, "Disable Prometheus metrics")
	return setMetricsCmd
}
