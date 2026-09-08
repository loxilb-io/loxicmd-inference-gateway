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
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

func NewGetTraceCmd(restOptions *api.RESTOptions) *cobra.Command {
	var otlp bool
	var parsers bool

	var getTraceCmd = &cobra.Command{
		Use:   "trace",
		Short: "Show HTTP/HTTPS request tracing status",
		Long: `Report request-tracing status (/config/trace/status), the current OTLP
exporter configuration with --otlp (/config/trace/otlp), or the available
trace parsers with --parsers (/config/trace/parsers).

ex)
	loxicmd get trace
	loxicmd get trace --otlp
	loxicmd get trace --parsers`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			sub := "status"
			what := "trace status"
			switch {
			case otlp:
				sub = "otlp"
				what = "trace otlp config"
			case parsers:
				sub = "parsers"
				what = "trace parsers"
			}
			resp, err := client.Trace().SubResources([]string{sub}).Get(ctx)
			if err != nil {
				return exitcode.Unavailablef("get trace: %v", err)
			}
			return printJSONResponse(resp, what)
		},
	}
	getTraceCmd.Flags().BoolVar(&otlp, "otlp", false, "Show the OTLP exporter configuration")
	getTraceCmd.Flags().BoolVar(&parsers, "parsers", false, "List the available trace parsers")
	return getTraceCmd
}
