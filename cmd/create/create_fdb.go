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

func NewCreateFDBCmd(restOptions *api.RESTOptions) *cobra.Command {
	var createFDBCmd = &cobra.Command{
		Use:   "fdb <MacAddress> <DeviceName>",
		Short: "Create a FDB",
		Long: `Create a FDB using LoxiLB. It is working as "bridge fdb add <MacAddress> dev <device>"
ex) loxicmd create fdb aa:aa:aa:aa:bb:bb eno7	
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("create fdb needs its arguments")
			}
			var FDBMod api.FDBMod
			// Make FDBMod
			if err := ReadCreateFDBOptions(&FDBMod, args); err != nil {
				return exitcode.Invalidf("%s", err.Error())
			}
			resp, err := FDBAPICall(restOptions, FDBMod)
			if err != nil {
				return exitcode.Unavailablef("create fdb: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				PrintCreateResult(resp, *restOptions)
				return nil
			}
			return exitcode.FromHTTPStatus("create fdb", resp.StatusCode)
		},
	}

	return createFDBCmd
}

func ReadCreateFDBOptions(o *api.FDBMod, args []string) error {
	if len(args) > 2 {
		return errors.New("create FDB command get so many args")
	} else if len(args) <= 1 {
		return errors.New("create FDB need <MacAddress>  args")
	}

	if _, err := net.ParseMAC(args[0]); err != nil {
		return fmt.Errorf("MacAddress '%s' is invalid format", args[0])
	}
	o.MacAddress = args[0]
	o.Dev = args[1]

	return nil
}

func FDBAPICall(restOptions *api.RESTOptions, FDBModel api.FDBMod) (*http.Response, error) {
	client := api.NewLoxiClient(restOptions)
	ctx := context.TODO()
	var cancel context.CancelFunc
	if restOptions.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
		defer cancel()
	}

	return client.FDB().Create(ctx, FDBModel)
}
