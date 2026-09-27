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

func NewGetAuditStatusCmd(restOptions *api.RESTOptions) *cobra.Command {
	var getAuditStatusCmd = &cobra.Command{
		Use:   "audit-status",
		Short: "Show the state of the management audit trail",
		Long: `Show the state of the management audit trail (GET /audit/status):
whether a writer is running, records accepted and dropped per stream, write
and sync failures, the active segment, the retention policy with the
retention it projects, and the management intents of the previous boot that
never received a result.

This is status, not content. The audit trail itself is deliberately not
served over the management API and there is no command that reads it: the
records exist to watch the same role that holds the management credential.
Read the trail on the gateway's filesystem instead.

A gateway whose audit directory was unusable at start still answers, with
availability false, which is the explanation for every management call it
then refuses. The read is not itself audited, so it is safe to poll. With
-o json the gateway's body is printed verbatim.

ex)
	loxicmd get audit-status
	loxicmd get audit-status -o json`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return lifecycle.AuditStatusGet(restOptions, cmd.OutOrStdout(),
				restOptions.PrintOption == "json")
		},
	}
	return getAuditStatusCmd
}
