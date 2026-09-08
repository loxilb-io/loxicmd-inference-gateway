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

func NewCreateBFDCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := api.BFDSessionInfo{}

	var createBFDCmd = &cobra.Command{
		Use:   "bfd remoteIP [--instance=<instance>] [--sourceIP=<source-IP>] [--interval=<interval>] [--retryCount=<count>]",
		Short: "Create a BFD session",
		Long: `Create a BFD session for HA failover

ex) loxicmd create bfd 32.32.32.2 --instance=default --sourceIP=32.32.32.1 --interval=200000 --retryCount=3`,
		Aliases: []string{"bfd-session"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("create bfd needs remoteIP args")
			}

			// Make bfdMod
			if err := ReadCreateBfdOptions(&o, args); err != nil {
				return exitcode.Invalidf("%s", err.Error())
			}
			resp, err := CreateBFDAPICall(restOptions, o)
			if err != nil {
				return exitcode.Unavailablef("create bfd: %v", err)
			}
			defer resp.Body.Close()

			//fmt.Printf("Debug: response.StatusCode: %d\n", resp.StatusCode)
			if resp.StatusCode == http.StatusOK {
				PrintCreateResult(resp, *restOptions)
				return nil
			}
			return exitcode.FromHTTPStatus("create bfd", resp.StatusCode)
		},
	}

	createBFDCmd.Flags().StringVarP(&o.Instance, "instance", "", "default", "Specify the cluster instance name")
	createBFDCmd.Flags().Uint64VarP(&o.Interval, "interval", "", 200000, "Specify the BFD packet tx interval (in microseconds)")
	createBFDCmd.Flags().Uint8VarP(&o.RetryCount, "retryCount", "", 3, "Specify the number of reties")
	createBFDCmd.Flags().StringVar(&o.SourceIP, "sourceIP", "", "Specify the source IP for the session")

	return createBFDCmd
}

func ReadCreateBfdOptions(o *api.BFDSessionInfo, args []string) error {
	if len(args) > 1 {
		return errors.New("create bfd command get so many args")
	} else if len(args) < 1 {
		return errors.New("create bfd need <RemoteIP> args")
	}

	if val := net.ParseIP(args[0]); val != nil {
		o.RemoteIP = args[0]
	} else {
		return fmt.Errorf("remote IP '%s' is invalid format", args[0])
	}

	return nil
}

func CreateBFDAPICall(restOptions *api.RESTOptions, bfdModel api.BFDSessionInfo) (*http.Response, error) {
	client := api.NewLoxiClient(restOptions)
	ctx := context.TODO()
	var cancel context.CancelFunc
	if restOptions.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
		defer cancel()
	}

	return client.BFDSession().Create(ctx, bfdModel)
}
