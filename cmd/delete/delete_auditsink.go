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
package delete

import (
	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/lifecycle"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewDeleteAuditSinkCmd(restOptions *api.RESTOptions) *cobra.Command {
	var deleteAuditSinkCmd = &cobra.Command{
		Use:   "audit-sink <NAME>",
		Short: "Remove a secondary audit sink",
		Long: `Remove a secondary audit sink (DELETE /audit/sinks/NAME).

The sink stops. The gateway keeps its place in the trail and its export
sequence, so a sink set again under the same name continues both and no
export sequence number is used twice. The local trail and the other sinks
are not touched.

The compliance sink has no name and is not removed here: use
'loxicmd set audit-sink --disable'.

If the outcome cannot be confirmed (timeout, broken connection), the command
fails with reason 'recovery-required' and never claims success - verify with
'loxicmd get audit-sink NAME'.

ex)
	loxicmd delete audit-sink siem2`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return lifecycle.AuditNamedSinkDelete(restOptions, cmd.OutOrStdout(),
				restOptions.PrintOption == "json", args[0])
		},
	}
	return deleteAuditSinkCmd
}
