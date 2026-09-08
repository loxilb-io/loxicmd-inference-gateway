/*
 * Copyright (c) 2022 NetLOX Inc
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
	"fmt"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

func DeleteCmd(restOptions *api.RESTOptions) *cobra.Command {

	var NormalConfigFile string
	var deleteCmd = &cobra.Command{
		Use:   "delete",
		Short: "Delete a Load balance features in the LoxiLB.",
		Long: `Delete a Load balance features in the LoxiLB. 
Delete - Service type external load-balancer, Vlan, Vxlan, Qos Policies,
	 Endpoint client,FDB, IPaddress, Neighbor, Route,Firewall, Mirror, Session, UlCl
		`,
		// One RunE only. The previous version defined Run AND RunE; cobra
		// runs just the RunE, so the -f config-file path below was dead and
		// an unknown subcommand exited 0 after printing an error.
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(NormalConfigFile) > 0 {
				if err := DeleteFileConfig(NormalConfigFile, restOptions); err != nil {
					return exitcode.Classify(fmt.Errorf("configuration failed - %s: %w", NormalConfigFile, err))
				}
				fmt.Printf("Configuration applied - %s\n", NormalConfigFile)
				return nil
			}
			if len(args) == 0 {
				return exitcode.Usagef("delete needs a subcommand or --file")
			}
			return exitcode.Usagef("unknown command %q for \"loxicmd delete\"", args[0])
		},
	}
	deleteCmd.AddCommand(NewDeleteLoadBalancerCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteSessionCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteSessionUlClCmd(restOptions))
	deleteCmd.AddCommand(NewDeletePolicyCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteRouteCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteIPv4AddressCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteNeighborsCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteFDBCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteVlanBridgeCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteVlanMemberCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteVxlanBridgeCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteVxlanPeerCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteMirrorCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteFirewallCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteEndPointCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteBGPNeighborCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteBFDCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteAPIKeyCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteCertCmd(restOptions))
	deleteCmd.AddCommand(NewDeleteSNICmd(restOptions))
	deleteCmd.AddCommand(NewDeleteOPACmd(restOptions))

	deleteCmd.Flags().StringVarP(&NormalConfigFile, "file", "f", "", "Config file to apply as like K8s")
	return deleteCmd
}
