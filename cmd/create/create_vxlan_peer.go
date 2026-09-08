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
package create

import (
	"context"
	"errors"
	"fmt"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

func NewCreateVxlanPeerCmd(restOptions *api.RESTOptions) *cobra.Command {
	var createvxlanCmd = &cobra.Command{
		Use:   "vxlanpeer <Vnid> <PeerIP>",
		Short: "Create a vxlan",
		Long: `Create a vxlan using LoxiLB.

ex) loxicmd create vxlan-peer 100 30.1.3.1
`,
		Aliases: []string{"vxlanPeer", "vxlan-peer", "vxlan_peer"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("create vxlanpeer needs its arguments")
			}
			var vxlanMod api.VxlanPeerMod
			// Make vxlanMod
			if err := ReadCreateVxlanPeerOptions(&vxlanMod, args); err != nil {
				return exitcode.Invalidf("%s", err.Error())
			}
			url := fmt.Sprintf("/config/tunnel/vxlan/%s/peer", args[0])
			resp, err := VxlanPeerAPICall(restOptions, vxlanMod, url)
			if err != nil {
				return exitcode.Unavailablef("create vxlanpeer: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				PrintCreateResult(resp, *restOptions)
				return nil
			}
			return exitcode.FromHTTPStatus("create vxlanpeer", resp.StatusCode)
		},
	}

	return createvxlanCmd
}

func ReadCreateVxlanPeerOptions(o *api.VxlanPeerMod, args []string) error {
	if len(args) > 2 {
		return errors.New("create vxlan command get so many args")
	} else if len(args) < 1 {
		return errors.New("create vxlan need <MacAddress>  args")
	}

	if val := net.ParseIP(args[1]); val == nil {
		return fmt.Errorf("PeerIP '%s' is invalid format", args[1])
	}
	o.PeerIP = args[1]

	return nil
}

func VxlanPeerAPICall(restOptions *api.RESTOptions, vxlanModel api.VxlanPeerMod, url string) (*http.Response, error) {
	client := api.NewLoxiClient(restOptions)
	ctx := context.TODO()
	var cancel context.CancelFunc
	if restOptions.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
		defer cancel()
	}

	return client.Vxlan().SetUrl(url).Create(ctx, vxlanModel)
}
