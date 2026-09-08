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
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

func DeleteNeighborsValidation(args []string) error {
	if len(args) > 3 {
		fmt.Println("delete Neighbors command get so many args")
	} else if len(args) <= 1 {
		return errors.New("delete IP Address need <DeviceIP> <device> args")
	}
	if val := net.ParseIP(args[0]); val == nil {
		return fmt.Errorf("DeviceIP '%s' is invalid format", args[0])
	}

	return nil
}

func NewDeleteNeighborsCmd(restOptions *api.RESTOptions) *cobra.Command {

	var deleteNeighborsCmd = &cobra.Command{
		Use:     "neighbor <DeviceIP> <device>",
		Short:   "Delete a Neighbors",
		Long:    `Delete a Neighbors using DeviceIP in the LoxiLB.`,
		Aliases: []string{"nei", "neigh"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("delete neighbor needs its arguments")
			}
			if err := DeleteNeighborsValidation(args); err != nil {
				return exitcode.Invalidf("not valid <DeviceIP>")
			}
			DeviceIP := args[0]
			Device := args[1]

			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			subResources := []string{
				DeviceIP, "dev", Device,
			}
			resp, err := client.Neighbor().SubResources(subResources).Delete(ctx)
			if err != nil {
				return exitcode.Unavailablef("Failed to delete Neighbors : %s (%v)", DeviceIP, err)
			}
			defer resp.Body.Close()
			fmt.Printf("Debug: response.StatusCode: %d\n", resp.StatusCode)
			if resp.StatusCode == http.StatusOK {
				PrintDeleteResult(resp, *restOptions)
				return nil
			}
			return exitcode.FromHTTPStatus("delete neighbor", resp.StatusCode)
		},
	}

	return deleteNeighborsCmd
}
