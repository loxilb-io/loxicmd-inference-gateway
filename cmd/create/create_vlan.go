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
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
	"net/http"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

func NewCreateVlanBridgeCmd(restOptions *api.RESTOptions) *cobra.Command {
	var createvlanCmd = &cobra.Command{
		Use:   "vlan <Vid>",
		Short: "Create a vlan",
		Long: `Create a vlan using LoxiLB. It is working as "brctl addbr vlan$<Vid>"

ex) loxicmd create vlan 100
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("create vlan needs its arguments")
			}
			var vlanMod api.VlanBridgeMod
			// Make vlanMod
			if err := ReadCreateVlanBridgeOptions(&vlanMod, args); err != nil {
				return exitcode.Invalidf("%s", err.Error())
			}
			resp, err := VlanBridgeAPICall(restOptions, vlanMod)
			if err != nil {
				return exitcode.Unavailablef("create vlan: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				PrintCreateResult(resp, *restOptions)
				return nil
			}
			return exitcode.FromHTTPStatus("create vlan", resp.StatusCode)
		},
	}

	return createvlanCmd
}

func ReadCreateVlanBridgeOptions(o *api.VlanBridgeMod, args []string) error {
	if len(args) > 1 {
		return errors.New("create vlan command get so many args")
	}
	Vid, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	o.Vid = Vid
	return nil
}

func VlanBridgeAPICall(restOptions *api.RESTOptions, vlanModel api.VlanBridgeMod) (*http.Response, error) {
	client := api.NewLoxiClient(restOptions)
	ctx := context.TODO()
	var cancel context.CancelFunc
	if restOptions.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
		defer cancel()
	}

	return client.Vlan().Create(ctx, vlanModel)
}
