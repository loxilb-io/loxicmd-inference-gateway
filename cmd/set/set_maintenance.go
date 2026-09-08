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
package set

import (
	"fmt"

	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/lifecycle"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewSetMaintenanceCmd(restOptions *api.RESTOptions) *cobra.Command {
	var drainTimeout uint32

	var setMaintenanceCmd = &cobra.Command{
		Use:   "maintenance on|off",
		Short: "Enter or leave operator maintenance",
		Long: `Enter or leave operator maintenance (PUT /maintenance). Both directions
are idempotent on the gateway: repeating 'on' keeps the running episode's
operation id, start time, and drain window; 'off' when already active is a
no-op.

While maintenance holds, the gateway refuses mutating configuration calls
except the snapshot/persist/restore lifecycle operations and the
maintenance endpoint itself. It does NOT drain data-path inference
traffic - the read-back reports that truthfully.

If the outcome of the change cannot be confirmed (timeout, broken
connection), the command fails with reason 'recovery-required' and never
claims success - verify with 'loxicmd get maintenance'.

ex)
	loxicmd set maintenance on
	loxicmd set maintenance on --drain-timeout 300
	loxicmd set maintenance off
	loxicmd set maintenance on -o json`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var enable bool
			switch args[0] {
			case "on":
				enable = true
			case "off":
				enable = false
			default:
				return &api.LifecycleError{
					Reason:  api.ReasonInvalidArguments,
					Message: fmt.Sprintf("maintenance takes on or off, not %q", args[0]),
				}
			}
			return lifecycle.MaintenanceSet(restOptions, cmd.OutOrStdout(), cmd.ErrOrStderr(),
				lifecycle.OptionsFrom(restOptions, false),
				lifecycle.MaintenanceSetOptions{Enable: enable, DrainTimeoutSeconds: drainTimeout})
		},
	}
	setMaintenanceCmd.Flags().Uint32Var(&drainTimeout, "drain-timeout", 0,
		"Drain window in seconds declared on enter (0 = no deadline; ignored on repeat enter and on off)")
	return setMaintenanceCmd
}
