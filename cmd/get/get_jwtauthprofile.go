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

func NewGetJWTAuthProfileCmd(restOptions *api.RESTOptions) *cobra.Command {
	var getJWTAuthProfileCmd = &cobra.Command{
		Use:     "jwtauthprofile [NAME]",
		Short:   "Get inference-gateway JWT auth profiles",
		Aliases: []string{"jwtauthprofiles", "jwtprofile"},
		Long: `List the JWT auth profiles (GET /config/ai/jwtauthprofile), as the
gateway stores them, or one of them by NAME. This is the desired
configuration only, not issuer reachability or keyset health. The table
shows the name, the issuer, the tenant claim and the priority class;
-o wide adds every other field, -o json the whole entry. A field the
gateway did not report prints "-".

ex)
	loxicmd get jwtauthprofile
	loxicmd get jwtauthprofile keycloak -o wide`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return exitcode.Usagef("get jwtauthprofile takes at most one NAME")
			}
			if err := featuregate.Require(restOptions, featuregate.JWTAuthProfile, "get jwtauthprofile", cmd.ErrOrStderr()); err != nil {
				return err
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.AIJWTAuthProfile().Get(ctx)
			if err != nil {
				return exitcode.Unavailablef("get jwtauthprofile: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				return featuregate.Classify("get jwtauthprofile", featuregate.JWTAuthProfile, resp.StatusCode, body)
			}
			list := api.JWTAuthProfileList{}
			if err := json.Unmarshal(body, &list); err != nil {
				return &exitcode.CLIError{Code: exitcode.ContractMismatch, Message: fmt.Sprintf("get jwtauthprofile: decode response: %v", err), Origin: "gateway", HTTPStatus: resp.StatusCode}
			}
			if len(args) == 1 {
				kept := list.Profiles[:0]
				for _, p := range list.Profiles {
					if p.Name == args[0] {
						kept = append(kept, p)
					}
				}
				if len(kept) == 0 {
					return &exitcode.CLIError{Code: exitcode.Precondition, Message: fmt.Sprintf("get jwtauthprofile: no profile named %q", args[0])}
				}
				list.Profiles = kept
			}
			if restOptions.PrintOption == "json" {
				indent, _ := json.MarshalIndent(list, "", "    ")
				fmt.Println(string(indent))
				return nil
			}
			table := TableInit()
			wide := restOptions.PrintOption == "wide"
			if wide {
				table.SetHeader(JWTAUTHPROFILE_WIDE_TITLE)
			} else {
				table.SetHeader(JWTAUTHPROFILE_TITLE)
			}
			var data [][]string
			for _, p := range list.Profiles {
				data = append(data, jwtAuthProfileRow(p, wide))
			}
			TableShow(data, table)
			return nil
		},
	}
	return getJWTAuthProfileCmd
}

// jwtAuthProfileRow renders one profile; a field the gateway did not
// report prints "-", a zero number included, since zero selects the
// gateway's default and is never a configured value.
func jwtAuthProfileRow(p api.JWTAuthProfileEntry, wide bool) []string {
	row := []string{p.Name, dashIfEmpty(p.Issuer), dashIfEmpty(p.TenantClaim), dashIfEmpty(p.PriorityClass)}
	if wide {
		row = append(row, dashIfEmpty(p.JwksURL), dashIfEmpty(strings.Join(p.Audiences, ",")), dashIfEmpty(strings.Join(p.Algs, ",")),
			dashIfZero(p.LeewaySec), dashIfZero(p.RefreshSec), dashIfEmpty(p.UserClaim), dashIfEmpty(p.ModelsClaim),
			dashIfEmpty(p.RolesClaim), dashIfEmpty(p.ModelRolePrefix), dashIfEmpty(p.UsernameClaim), dashIfEmpty(p.ModelAuthz),
			dashIfEmpty(p.DefaultTenant), fmt.Sprintf("%v", p.ForwardIdentity), fmt.Sprintf("%v", p.AuthorizationPassthrough))
	}
	return row
}

func dashIfZero(n int64) string {
	if n == 0 {
		return "-"
	}
	return fmt.Sprint(n)
}
