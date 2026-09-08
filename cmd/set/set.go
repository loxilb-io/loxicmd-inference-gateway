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
package set

import (
	"encoding/json"
	"fmt"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
	"io"
	"net/http"

	"github.com/spf13/cobra"
)

type SetResult struct {
	Result string `json:"result"`
}

type SetOptions struct {
	Provider string
}

// SetParamCmd represents the Set command
func SetParamCmd(restOptions *api.RESTOptions) *cobra.Command {
	SetParamCmd := &cobra.Command{
		Use:   "set",
		Short: "Set configurations",
		Long:  `Set the configuration like log-level or bfd session`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("set needs a subcommand")
			}
			return exitcode.Usagef("unknown command %q for \"loxicmd set\"", args[0])
		},
	}
	SetParamCmd.AddCommand(NewSetLogLevelCmd(restOptions))
	SetParamCmd.AddCommand(NewSetBFDCmd(restOptions))
	SetParamCmd.AddCommand(NewSetLogInCmd(restOptions))
	SetParamCmd.AddCommand(NewSetLogOutCmd(restOptions))
	SetParamCmd.AddCommand(NewSetRefreshTokenCmd(restOptions))
	SetParamCmd.AddCommand(NewSetAPIKeyCmd(restOptions))
	SetParamCmd.AddCommand(NewSetRateLimitCmd(restOptions))
	SetParamCmd.AddCommand(NewSetMetricsCmd(restOptions))
	SetParamCmd.AddCommand(NewSetGPUCmd(restOptions))
	SetParamCmd.AddCommand(NewSetPIICmd(restOptions))
	SetParamCmd.AddCommand(NewSetLlamaFirewallCmd(restOptions))
	SetParamCmd.AddCommand(NewSetTraceCmd(restOptions))
	SetParamCmd.AddCommand(NewSetL4TraceCmd(restOptions))
	SetParamCmd.AddCommand(NewSetOPACmd(restOptions))
	SetParamCmd.AddCommand(NewSetDPUCmd(restOptions))
	SetParamCmd.AddCommand(NewSetMaintenanceCmd(restOptions))

	return SetParamCmd
}

func PrintSetResult(resp *http.Response, o api.RESTOptions) {
	result := SetResult{}
	resultByte, err := io.ReadAll(resp.Body)
	//fmt.Printf("Debug: response.Body: %s\n", string(resultByte))

	if err != nil {
		fmt.Printf("Error: Failed to read HTTP response: (%s)\n", err.Error())
		return
	}
	if err := json.Unmarshal(resultByte, &result); err != nil {
		fmt.Printf("Error: Failed to unmarshal HTTP response: (%s)\n", err.Error())
		return
	}

	if o.PrintOption == "json" {
		// TODO: need to test MarshalIndent
		resultIndent, _ := json.MarshalIndent(resp.Body, "", "\t")
		fmt.Println(string(resultIndent))
		return
	}

	fmt.Printf("%s\n", result.Result)
}
