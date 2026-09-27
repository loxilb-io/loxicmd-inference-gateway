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

func NewGetAuditSinkCmd(restOptions *api.RESTOptions) *cobra.Command {
	var getAuditSinkCmd = &cobra.Command{
		Use:   "audit-sink",
		Short: "Show the remote audit sink's configuration and session",
		Long: `Show the remote audit sink's configuration and what is known about its
current session (GET /audit/sink): the receiver, the bundle its certificate
is verified against, the syslog facility and frame cap, and the submission
counters.

Certificate material is named by path and never served: the paths reported
are on the gateway's own filesystem, not this host's. The submitted count is
writes to the socket, not deliveries - syslog over TLS carries no
acknowledgement, so the gateway cannot know more than that.

With -o json the gateway's body is printed verbatim.

ex)
	loxicmd get audit-sink
	loxicmd get audit-sink -o json`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return lifecycle.AuditSinkGet(restOptions, cmd.OutOrStdout(),
				restOptions.PrintOption == "json")
		},
	}
	return getAuditSinkCmd
}
