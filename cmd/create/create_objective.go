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
package create

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

type CreateObjectiveOptions struct {
	Priority int32
	Services []string
}

func NewCreateObjectiveCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := CreateObjectiveOptions{}

	var createObjectiveCmd = &cobra.Command{
		Use:     "objective <NAME> [--priority=<int>] [--services=<VIP:port>,]",
		Short:   "Create or replace an inference-gateway objective (a named request priority)",
		Aliases: []string{"objectives"},
		Long: `Create or replace an objective: a named priority a request can ask for
with the x-llm-d-inference-objective header, or get as its tenant's or
key's default (POST /config/ai/objective). Posting an existing name
replaces the objective.

NAME is 1 to 63 characters of letters, digits, '-', '_' and '.', and must
not begin with tier:. A positive --priority is the critical class, a
negative one sheddable, zero (the default) standard. --services limits the
objective to the listed services ("VIP:port"); omitted means every
service. What the gateway refuses is printed as it answers.

ex)
	loxicmd create objective interactive --priority=10
	loxicmd create objective batch --priority=-5 --services=192.0.2.20:8000`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exitcode.Usagef("create objective needs <NAME> arg")
			}
			entry := api.AIObjectiveEntry{Name: args[0], Priority: o.Priority, Services: o.Services}

			if err := featuregate.Require(restOptions, featuregate.Objectives, cmd.ErrOrStderr()); err != nil {
				return err
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.AIObjective().Create(ctx, entry)
			if err != nil {
				return exitcode.Unavailablef("create objective: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				return featuregate.Classify("create objective", featuregate.Objectives, resp.StatusCode, body)
			}
			return printOperationResult(cmd.OutOrStdout(), body, restOptions.PrintOption)
		},
	}
	createObjectiveCmd.Flags().Int32Var(&o.Priority, "priority", 0, "The priority a request with this objective has: positive is the critical class, negative sheddable, zero standard")
	createObjectiveCmd.Flags().StringSliceVar(&o.Services, "services", nil, "Services the objective applies to, as VIP:port; at most 256; empty means all")
	return createObjectiveCmd
}

// printOperationResult prints the gateway's OperationResult as answered:
// its result line, which may carry a warning after the status, or the
// whole body under -o json.
func printOperationResult(w io.Writer, body []byte, printOption string) error {
	if printOption == "json" {
		var doc any
		if err := json.Unmarshal(body, &doc); err != nil {
			return &exitcode.CLIError{Code: exitcode.ContractMismatch, Message: fmt.Sprintf("decode response: %v", err), Origin: "gateway"}
		}
		indent, _ := json.MarshalIndent(doc, "", "    ")
		_, err := fmt.Fprintln(w, string(indent))
		return err
	}
	var result struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Result == "" {
		_, err := fmt.Fprintln(w, strings.TrimSpace(string(body)))
		return err
	}
	_, err := fmt.Fprintln(w, result.Result)
	return err
}
