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
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

type DeleteEndPoint struct {
	Host      string
	Name      string
	ProbeType string
	ProbePort int
}

func NewDeleteEndPointCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := DeleteEndPoint{}

	var deleteEndPointCmd = &cobra.Command{
		Use:   "endpoint IP [--name=<id>] [--probetype=<probetype>] [--probeport=<port>]",
		Short: "Delete a LB EndPoint from monitoring",
		Long: `Delete a LB EndPoint from monitoring in the LoxiLB.

ex) loxicmd delete endpoint 31.31.31.31 --name=31.31.31.31_http_8080 --probetype=http --probeport=8080"
		`,
		Aliases: []string{"EndPoint", "ep", "endpoints"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("delete endpoint needs its arguments")
			}
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}

			if val := net.ParseIP(args[0]); val != nil {
				o.Host = args[0]
			} else {
				return exitcode.Invalidf("HOSTIP '%s' is invalid format", args[0])
			}

			if o.ProbeType != "http" && o.ProbeType != "https" && o.ProbeType != "ping" &&
				o.ProbeType != "tcp" && o.ProbeType != "udp" &&
				o.ProbeType != "sctp" && o.ProbeType != "none" {
				return exitcode.Invalidf("probetype '%s' is invalid", o.ProbeType)
			}

			if o.ProbeType == "http" || o.ProbeType == "https" || o.ProbeType == "tcp" ||
				o.ProbeType == "udp" || o.ProbeType == "sctp" {
				if o.ProbePort == 0 {
					return exitcode.Invalidf("probeport cant be 0 for '%s' probes", o.ProbeType)
				}
			}

			if o.ProbeType == "ping" && o.ProbePort != 0 {
				return exitcode.Invalidf("probeport should be 0 for '%s' probes", o.ProbeType)
			}

			/*		subResources := []string{
						"epipaddress", o.Host,
						"name", o.Name,
						"probetype", o.ProbeType,
						"probeport", strconv.Itoa(o.ProbePort),
					}
					resp, err := client.EndPoint().SubResources(subResources).Delete(ctx)
			*/
			subResources := []string{
				"epipaddress", o.Host,
			}

			qmap := map[string]string{}
			qmap["name"] = o.Name
			qmap["probe_type"] = o.ProbeType
			qmap["probe_port"] = strconv.Itoa(o.ProbePort)

			resp, err := client.EndPoint().SubResources(subResources).Query(qmap).Delete(ctx)

			if err != nil {
				return exitcode.Unavailablef("Failed to delete EndPoint (%v)", err)
			}
			defer resp.Body.Close()
			fmt.Printf("Debug: response.StatusCode: %d\n", resp.StatusCode)
			if resp.StatusCode == http.StatusOK {
				PrintDeleteResult(resp, *restOptions)
				return nil
			}
			return exitcode.FromHTTPStatus("delete endpoint", resp.StatusCode)
		},
	}
	deleteEndPointCmd.Flags().StringVar(&o.Name, "name", "", "Endpoint Identifier")
	deleteEndPointCmd.Flags().StringVar(&o.ProbeType, "probetype", "ping", "Probe-type:ping,http,https,udp,tcp,sctp,none")
	deleteEndPointCmd.Flags().IntVar(&o.ProbePort, "probeport", 0, "If probe is http,https,tcp,udp,sctp one can specify custom l4port to use")
	return deleteEndPointCmd
}
