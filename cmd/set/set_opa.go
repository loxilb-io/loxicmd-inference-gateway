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
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

func NewSetOPACmd(restOptions *api.RESTOptions) *cobra.Command {
	var opaURL string
	var policyPath string
	var pollIntervalSec int
	var failOpen bool

	var setOPACmd = &cobra.Command{
		Use:   "opa --opa-url <url> [--policy-path p] [--poll-interval-sec n] [--fail-open]",
		Short: "Configure and start the OPA L4 policy watcher",
		Long: `Create (or replace) the OPA L4 policy watcher and start polling the given
OPA server (/config/opa/watcher). Stop it with 'delete opa'.

Note: this is a raw-middleware endpoint. URLs resolving to private/reserved IP
ranges are rejected server-side (SSRF protection).

ex)
	loxicmd set opa --opa-url http://opa.example.com:8181 --policy-path loxilb/l4 --poll-interval-sec 30`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opaURL == "" {
				return exitcode.Usagef("--opa-url is required")
			}
			req := api.OPAWatcherConfig{
				OpaURL:          opaURL,
				PolicyPath:      policyPath,
				PollIntervalSec: pollIntervalSec,
				FailOpen:        failOpen,
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			resp, err := client.OPA().Create(ctx, req)
			return reportPost(resp, err, "OPA policy watcher configured and started.")
		},
	}
	setOPACmd.Flags().StringVar(&opaURL, "opa-url", "", "Base URL of the OPA server (required)")
	setOPACmd.Flags().StringVar(&policyPath, "policy-path", "", "OPA policy path to poll (server default: loxilb/l4)")
	setOPACmd.Flags().IntVar(&pollIntervalSec, "poll-interval-sec", 0, "Poll interval in seconds (server default: 30)")
	setOPACmd.Flags().BoolVar(&failOpen, "fail-open", false, "Allow traffic when OPA is unreachable")
	return setOPACmd
}
