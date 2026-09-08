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

type CreateRouteStaticOptions struct {
	StaticProto string
}

func NewCreateRouteCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := CreateRouteStaticOptions{}
	var createRouteCmd = &cobra.Command{
		Use:   "route <DestinationIPNet> <gateway> --proto=<protocol>",
		Short: "Create a Route",
		Long: `Create a Route using LoxiLB. It is working as "ip route add <DestinationIPNet> via <gateway> proto <protocol>"
	
ex) loxicmd create route 192.168.212.0/24 172.17.0.254 --proto=static
    loxicmd create route 192.168.212.0/24 172.17.0.254
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("create route needs its arguments")
			}
			var RouteMod api.Routev4Get
			// Make RouteMod
			if err := ReadCreateRouteOptions(&RouteMod, args, o); err != nil {
				return exitcode.Invalidf("%s", err.Error())
			}
			resp, err := RouteAPICall(restOptions, RouteMod)
			if err != nil {
				return exitcode.Unavailablef("create route: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				PrintCreateResult(resp, *restOptions)
				return nil
			}
			return exitcode.FromHTTPStatus("create route", resp.StatusCode)
		},
	}
	createRouteCmd.Flags().StringVarP(&o.StaticProto, "proto", "", "", "Proto static mode")
	return createRouteCmd
}

func ReadCreateRouteOptions(o *api.Routev4Get, args []string, opts CreateRouteStaticOptions) error {
	if len(args) > 3 {
		return errors.New("create Route command get so many args")
	} else if len(args) <= 1 {
		return errors.New("create Route need <DestinationIPNet> and <gateway> args")
	}

	if _, _, err := net.ParseCIDR(args[0]); err != nil {
		return fmt.Errorf("DestinationIPNet '%s' is invalid format", args[0])
	}
	o.Dst = args[0]

	if val := net.ParseIP(args[1]); val != nil {
		o.Gw = args[1]
	} else {
		return fmt.Errorf("gateway IP '%s' is invalid format", args[1])
	}

	if opts.StaticProto == "static" {
		o.Protocol = opts.StaticProto
	}

	return nil
}

func RouteAPICall(restOptions *api.RESTOptions, RouteModel api.Routev4Get) (*http.Response, error) {
	client := api.NewLoxiClient(restOptions)
	ctx := context.TODO()
	var cancel context.CancelFunc
	if restOptions.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
		defer cancel()
	}

	return client.Route().Create(ctx, RouteModel)
}
