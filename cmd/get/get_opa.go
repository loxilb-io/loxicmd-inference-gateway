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

func NewGetOPACmd(restOptions *api.RESTOptions) *cobra.Command {
	var getOPACmd = &cobra.Command{
		Use:   "opa",
		Short: "Show the OPA L4 policy watcher status",
		Long: `Report the configuration and runtime status of the OPA L4 policy
watcher (/config/opa/watcher). When no watcher is configured the status is
"not_configured".

Note: this is a raw-middleware endpoint; configure it with 'set opa' and stop
it with 'delete opa'.

ex)
	loxicmd get opa`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			resp, err := client.OPA().Get(ctx)
			if err != nil {
				return exitcode.Unavailablef("get opa: %v", err)
			}
			return printJSONResponse(resp, "opa watcher")
		},
	}
	return getOPACmd
}
