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
	"fmt"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

func NewDeleteBFDCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := api.BFDSessionInfo{}

	var deleteBFDCmd = &cobra.Command{
		Use:   "bfd remoteIP [--instance=<instance>]",
		Short: "Delete a BFD session",
		Long: `Delete a BFD session for HA failover.

ex) loxicmd delete bfd 32.32.32.2 --instance=default"
		`,
		Aliases: []string{"bfd-session"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("delete bfd needs its arguments")
			}
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}

			if val := net.ParseIP(args[0]); val != nil {
				o.RemoteIP = args[0]
			} else {
				return exitcode.Invalidf("remoteIP '%s' is invalid format", args[0])
			}
			subResources := []string{
				"remoteIP", o.RemoteIP,
			}

			qmap := map[string]string{}
			qmap["instance"] = o.Instance

			resp, err := client.BFDSession().SubResources(subResources).Query(qmap).Delete(ctx)
			if err != nil {
				return exitcode.Unavailablef("Failed to delete bfd session (%v)", err)
			}
			defer resp.Body.Close()

			fmt.Printf("Debug: response.StatusCode: %d\n", resp.StatusCode)
			if resp.StatusCode == http.StatusOK {
				PrintDeleteResult(resp, *restOptions)
				return nil
			}
			return exitcode.FromHTTPStatus("delete bfd", resp.StatusCode)
		},
	}
	deleteBFDCmd.Flags().StringVarP(&o.Instance, "instance", "", "default", "Specify the cluster instance name")

	return deleteBFDCmd
}
