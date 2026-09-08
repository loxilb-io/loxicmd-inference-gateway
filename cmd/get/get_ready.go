/*
 * Copyright (c) 2026 LoxiLB Authors
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
	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/lifecycle"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewGetReadyCmd(restOptions *api.RESTOptions) *cobra.Command {
	var getReadyCmd = &cobra.Command{
		Use:   "ready",
		Short: "Show the gateway's configuration readiness verdict",
		Long: `Show the configuration readiness verdict with its evidence
(GET /status/ready): the boot replay outcome, live external-dependency
probes, kernel-verified per-interface eBPF attachment, and the reasons
when not ready.

The gateway answers 200 (ready) and 503 (not ready) with the same body;
this command renders both identically and carries the verdict in its
exit status, so automation can probe readiness with the exit code alone.
With -o json the gateway's body is printed verbatim.

ex)
	loxicmd get ready
	loxicmd get ready -o json`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return lifecycle.ReadyGet(restOptions, cmd.OutOrStdout(),
				restOptions.PrintOption == "json")
		},
	}
	return getReadyCmd
}
