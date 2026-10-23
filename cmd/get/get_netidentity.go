/*
 * Copyright (c) 2026 LoxiLB Authors
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
package get

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/featuregate"

	"github.com/spf13/cobra"
)

func NewGetNetIdentityCmd(restOptions *api.RESTOptions) *cobra.Command {
	var getNetIdentityCmd = &cobra.Command{
		Use:     "netidentity [NAME]",
		Short:   "Get inference-gateway network-identity bindings",
		Aliases: []string{"netidentities", "netid"},
		Long: `List the network-identity bindings (GET /config/ai/netidentity), in name
order as the gateway stores them, or one of them by NAME. -o wide adds the
forwarded identity, the priority class, the allowed models and the
services.

ex)
	loxicmd get netidentity
	loxicmd get netidentity lab -o wide`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return exitcode.Usagef("get netidentity takes at most one NAME")
			}
			if err := featuregate.Require(restOptions, featuregate.NetIdentity, cmd.ErrOrStderr()); err != nil {
				return err
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.AINetIdentity().Get(ctx)
			if err != nil {
				return exitcode.Unavailablef("get netidentity: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				return featuregate.Classify("get netidentity", featuregate.NetIdentity, resp.StatusCode, body)
			}
			list := api.NetIdentityList{}
			if err := json.Unmarshal(body, &list); err != nil {
				return &exitcode.CLIError{Code: exitcode.ContractMismatch, Message: fmt.Sprintf("get netidentity: decode response: %v", err), Origin: "gateway", HTTPStatus: resp.StatusCode}
			}
			if len(args) == 1 {
				kept := list.Bindings[:0]
				for _, b := range list.Bindings {
					if b.Name == args[0] {
						kept = append(kept, b)
					}
				}
				if len(kept) == 0 {
					return &exitcode.CLIError{Code: exitcode.Precondition, Message: fmt.Sprintf("get netidentity: no binding named %q", args[0])}
				}
				list.Bindings = kept
			}
			if restOptions.PrintOption == "json" {
				indent, _ := json.MarshalIndent(list, "", "    ")
				fmt.Println(string(indent))
				return nil
			}
			table := TableInit()
			wide := restOptions.PrintOption == "wide"
			if wide {
				table.SetHeader(NETIDENTITY_WIDE_TITLE)
			} else {
				table.SetHeader(NETIDENTITY_TITLE)
			}
			var data [][]string
			for _, b := range list.Bindings {
				data = append(data, netIdentityRow(b, wide))
			}
			TableShow(data, table)
			return nil
		},
	}
	return getNetIdentityCmd
}

// netIdentityMatchCell renders the match as the field that is set
// ("cidr=10.0.0.0/8,10.1.0.0/16", "vlan=100"); every field the gateway
// reports is shown, "-" when none is.
func netIdentityMatchCell(m api.NetIdentityMatch) string {
	var parts []string
	if len(m.Cidr) > 0 {
		parts = append(parts, "cidr="+strings.Join(m.Cidr, ","))
	}
	if m.IngressIf != "" {
		parts = append(parts, "ingress_if="+m.IngressIf)
	}
	if m.Vlan != 0 {
		parts = append(parts, fmt.Sprintf("vlan=%d", m.Vlan))
	}
	if m.Vni != 0 {
		parts = append(parts, fmt.Sprintf("vni=%d", m.Vni))
	}
	if m.IpsecTunnel != "" {
		parts = append(parts, "ipsec_tunnel="+m.IpsecTunnel)
	}
	return dashIfEmpty(strings.Join(parts, " "))
}

// netIdentityRow renders one binding; a field the gateway did not report
// prints "-".
func netIdentityRow(b api.NetIdentityEntry, wide bool) []string {
	row := []string{b.Name, b.Tenant, netIdentityMatchCell(b.Match), dashIfEmpty(b.Enforce), dashIfEmpty(b.Credential), dashIfEmpty(b.FailMode)}
	if wide {
		row = append(row, fmt.Sprintf("%v", b.ForwardIdentity), dashIfEmpty(b.PriorityClass),
			dashIfEmpty(strings.Join(b.AllowedModels, ",")), dashIfEmpty(strings.Join(b.Services, ",")))
	}
	return row
}
