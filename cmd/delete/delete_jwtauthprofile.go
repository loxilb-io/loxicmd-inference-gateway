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
package delete

import (
	"fmt"
	"io"
	"net/http"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/featuregate"

	"github.com/spf13/cobra"
)

func NewDeleteJWTAuthProfileCmd(restOptions *api.RESTOptions) *cobra.Command {
	var deleteCmd = &cobra.Command{
		Use:     "jwtauthprofile <NAME>",
		Short:   "Delete an inference-gateway JWT auth profile",
		Aliases: []string{"jwtauthprofiles", "jwtprofile"},
		Long: `Remove the named JWT auth profile (DELETE
/config/ai/jwtauthprofile/{name}). A profile referenced by any LB rule is
not deletable: the gateway answers 409 and this command exits 4
PRECONDITION with its message; detach it from every rule first.

ex)
	loxicmd delete jwtauthprofile <name>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exitcode.Usagef("delete jwtauthprofile needs <NAME> arg")
			}
			if err := featuregate.Require(restOptions, featuregate.JWTAuthProfile, "delete jwtauthprofile", cmd.ErrOrStderr()); err != nil {
				return err
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.AIJWTAuthProfile().SubResources([]string{args[0]}).Delete(ctx)
			if err != nil {
				return exitcode.Unavailablef("delete jwtauthprofile: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
				return featuregate.Classify("delete jwtauthprofile", featuregate.JWTAuthProfile, resp.StatusCode, body)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "JWT auth profile '%s' deleted.\n", args[0])
			return nil
		},
	}
	return deleteCmd
}
