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

func NewGetMaintenanceCmd(restOptions *api.RESTOptions) *cobra.Command {
	var getMaintenanceCmd = &cobra.Command{
		Use:   "maintenance",
		Short: "Show the operator maintenance state and drain progress",
		Long: `Show whether an operator holds the gateway in maintenance
(GET /maintenance): what is being refused while it does, the in-flight
streaming-session count, and how far the drain has progressed against the
declared drain window. Every field is the gateway's observed truth.

ex)
	loxicmd get maintenance
	loxicmd get maintenance -o json`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return lifecycle.MaintenanceGet(restOptions, cmd.OutOrStdout(), cmd.ErrOrStderr(),
				lifecycle.OptionsFrom(restOptions, false))
		},
	}
	return getMaintenanceCmd
}
