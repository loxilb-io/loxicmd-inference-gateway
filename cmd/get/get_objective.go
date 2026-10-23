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

func NewGetObjectiveCmd(restOptions *api.RESTOptions) *cobra.Command {
	var getObjectiveCmd = &cobra.Command{
		Use:     "objective [NAME]",
		Short:   "Get inference-gateway objectives (named request priorities)",
		Aliases: []string{"objectives"},
		Long: `List the objectives (GET /config/ai/objective), in name order as the
gateway stores them, or one of them by NAME. The line above the table says
where the list came from: the AI key store, or the last-known-good file
this gateway started from while the store has not answered since.

ex)
	loxicmd get objective
	loxicmd get objective interactive -o json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return exitcode.Usagef("get objective takes at most one NAME")
			}
			if err := featuregate.Require(restOptions, featuregate.Objectives, cmd.ErrOrStderr()); err != nil {
				return err
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.AIObjective().Get(ctx)
			if err != nil {
				return exitcode.Unavailablef("get objective: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				return featuregate.Classify("get objective", featuregate.Objectives, resp.StatusCode, body)
			}
			list := api.AIObjectiveList{}
			if err := json.Unmarshal(body, &list); err != nil {
				return &exitcode.CLIError{Code: exitcode.ContractMismatch, Message: fmt.Sprintf("get objective: decode response: %v", err), Origin: "gateway", HTTPStatus: resp.StatusCode}
			}
			if len(args) == 1 {
				kept := list.Objectives[:0]
				for _, o := range list.Objectives {
					if o.Name == args[0] {
						kept = append(kept, o)
					}
				}
				if len(kept) == 0 {
					return &exitcode.CLIError{Code: exitcode.Precondition, Message: fmt.Sprintf("get objective: no objective named %q", args[0])}
				}
				list.Objectives = kept
			}
			if restOptions.PrintOption == "json" {
				indent, _ := json.MarshalIndent(list, "", "    ")
				fmt.Println(string(indent))
				return nil
			}
			fmt.Println(objectiveSourceLine(list))
			table := TableInit()
			table.SetHeader(OBJECTIVE_TITLE)
			var data [][]string
			for _, o := range list.Objectives {
				data = append(data, objectiveRow(o))
			}
			TableShow(data, table)
			return nil
		},
	}
	return getObjectiveCmd
}

// objectiveSourceLine says where the list came from and what the key
// store answered; a value the gateway did not report prints "-".
func objectiveSourceLine(list api.AIObjectiveList) string {
	reachable := "false"
	if list.StoreReachable {
		reachable = "true"
	}
	return fmt.Sprintf("source: %s  source time: %s  store reachable: %s  store last read: %s",
		dashIfEmpty(list.Source), dashIfEmpty(list.SourceTimestamp), reachable, dashIfEmpty(list.StoreLastRead))
}

func objectiveRow(o api.AIObjectiveEntry) []string {
	return []string{o.Name, fmt.Sprintf("%d", o.Priority), dashIfEmpty(strings.Join(o.Services, ",")), dashIfEmpty(o.UpdatedAt)}
}
