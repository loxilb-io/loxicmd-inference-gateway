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
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

func DeleteBGPNeighborValidation(args []string) error {
	if len(args) > 3 {
		fmt.Println("delete BGPNeighbor command get so many args")
	} else if len(args) <= 1 {
		return errors.New("delete IP Address need <MacAddress> <device> args")
	}
	if val := net.ParseIP(args[0]); val == nil {
		return fmt.Errorf("peer IP '%s' is invalid format", args[0])
	}
	if val, err := strconv.Atoi(args[1]); err != nil || val > 65535 || 0 > val {
		return fmt.Errorf("remoteAS '%s' is invalid format", args[1])
	}
	return nil
}

func NewDeleteBGPNeighborCmd(restOptions *api.RESTOptions) *cobra.Command {

	var deleteBGPNeighborCmd = &cobra.Command{
		Use:     "bgpneighbor <PeerIP> <RemoteAS>",
		Short:   "Delete a BGP Neighbor peer information",
		Long:    `Delete a BGP Neighbor peer information in the LoxiLB.`,
		Aliases: []string{"bgpnei", "bgpneigh"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("delete bgpneighbor needs its arguments")
			}
			if err := DeleteBGPNeighborValidation(args); err != nil {
				return exitcode.Invalidf("not valid <PeerIP> or <RemoteAs>: %v", err)
			}
			PeerIP := args[0]
			RemoteAS := args[1]
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			subResources := []string{
				PeerIP,
			}
			qmap := map[string]string{}
			qmap["remoteAs"] = fmt.Sprintf("%v", RemoteAS)
			resp, err := client.BGPNeighbor().SubResources(subResources).Query(qmap).Delete(ctx)
			if err != nil {
				return exitcode.Unavailablef("Failed to delete BGPNeighbor : %s (%v)", PeerIP, err)
			}
			defer resp.Body.Close()
			fmt.Printf("Debug: response.StatusCode: %d\n", resp.StatusCode)
			if resp.StatusCode == http.StatusOK {
				PrintDeleteResult(resp, *restOptions)
				return nil
			}
			return exitcode.FromHTTPStatus("delete bgpneighbor", resp.StatusCode)
		},
	}

	return deleteBGPNeighborCmd
}
