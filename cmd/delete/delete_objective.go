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

func NewDeleteObjectiveCmd(restOptions *api.RESTOptions) *cobra.Command {
	var deleteCmd = &cobra.Command{
		Use:     "objective <NAME>",
		Short:   "Delete an inference-gateway objective",
		Aliases: []string{"objectives"},
		Long: `Remove the named objective (DELETE /config/ai/objective/{name}). A tenant
or key that still names it keeps the name; a request that asks for it is
then treated as one that asked for an unknown objective.

ex)
	loxicmd delete objective <name>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exitcode.Usagef("delete objective needs <NAME> arg")
			}
			if err := featuregate.Require(restOptions, featuregate.Objectives, "delete objective", cmd.ErrOrStderr()); err != nil {
				return err
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.AIObjective().SubResources([]string{args[0]}).Delete(ctx)
			if err != nil {
				return exitcode.Unavailablef("delete objective: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
				return featuregate.Classify("delete objective", featuregate.Objectives, resp.StatusCode, body)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Objective '%s' deleted.\n", args[0])
			return nil
		},
	}
	return deleteCmd
}
