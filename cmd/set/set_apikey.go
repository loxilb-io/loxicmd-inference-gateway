/*
 * Copyright (c) 2025 LoxiLB Authors
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
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/featuregate"

	"github.com/spf13/cobra"
)

func NewSetAPIKeyCmd(restOptions *api.RESTOptions) *cobra.Command {
	var allowedModels []string
	var enabled bool
	var priorityClass, defaultObjective string
	var allowedObjectives []string

	var setAPIKeyCmd = &cobra.Command{
		Use:   "apikey <KEY-ID> [--allowed-models=<m>,] [--enabled] [--priority-class=<class>] [--default-objective=<name>] [--allowed-objectives=<name>,]",
		Short: "Update an inference-gateway API key",
		Long: `Update the allowed model list, enabled state and/or priority of an API key
(PATCH). Only the flags you set are changed; an empty --priority-class,
--default-objective or --allowed-objectives clears the stored value.

ex)
	loxicmd set apikey lxb_abc123 --allowed-models=mistral-7b
	loxicmd set apikey lxb_abc123 --enabled=false
	loxicmd set apikey lxb_abc123 --priority-class=critical --default-objective=batch`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exitcode.Usagef("set apikey needs <KEY-ID> arg")
			}
			if !cmd.Flags().Changed("allowed-models") && !cmd.Flags().Changed("enabled") &&
				!cmd.Flags().Changed("priority-class") && !cmd.Flags().Changed("default-objective") &&
				!cmd.Flags().Changed("allowed-objectives") {
				return exitcode.Usagef("set at least one of --allowed-models, --enabled, --priority-class, --default-objective or --allowed-objectives")
			}
			if cmd.Flags().Changed("priority-class") && priorityClass != "" && !api.IsPriorityClass(priorityClass) {
				return exitcode.Invalidf("--priority-class must be one of %s, or empty to clear it", strings.Join(api.PriorityClasses, "|"))
			}
			if cmd.Flags().Changed("priority-class") || cmd.Flags().Changed("default-objective") || cmd.Flags().Changed("allowed-objectives") {
				if err := featuregate.Require(restOptions, featuregate.Objectives, "set apikey", cmd.ErrOrStderr()); err != nil {
					return err
				}
			}
			req := api.AIApiKeyPatchRequest{}
			if cmd.Flags().Changed("allowed-models") {
				req.AllowedModels = allowedModels
			}
			if cmd.Flags().Changed("enabled") {
				e := enabled
				req.Enabled = &e
			}
			if cmd.Flags().Changed("priority-class") {
				req.PriorityClass = &priorityClass
			}
			if cmd.Flags().Changed("default-objective") {
				req.DefaultObjective = &defaultObjective
			}
			if cmd.Flags().Changed("allowed-objectives") {
				list := append([]string{}, allowedObjectives...)
				req.AllowedObjectives = &list
			}

			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			resp, err := client.AIApiKey().SubResources([]string{args[0]}).Update(ctx, req)
			if err != nil {
				return exitcode.Unavailablef("set apikey: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
				ce := exitcode.FromHTTPStatus("set apikey", resp.StatusCode)
				ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
				if resp.StatusCode == http.StatusBadRequest {
					ce.Message = api.NewAPIError(resp.StatusCode, body).Verbatim()
				}
				return ce
			}
			fmt.Printf("API key '%s' updated.\n", args[0])
			return nil
		},
	}
	setAPIKeyCmd.Flags().StringSliceVar(&allowedModels, "allowed-models", allowedModels, "Replacement allowed model list")
	setAPIKeyCmd.Flags().BoolVar(&enabled, "enabled", true, "Enable or disable the key")
	setAPIKeyCmd.Flags().StringVar(&priorityClass, "priority-class", "", "The highest priority class a request with this key may have: standard, critical or sheddable; empty clears it (no cap from the key)")
	setAPIKeyCmd.Flags().StringVar(&defaultObjective, "default-objective", "", "The objective a request with this key gets when it names none; empty clears it")
	setAPIKeyCmd.Flags().StringSliceVar(&allowedObjectives, "allowed-objectives", nil, "The objectives a request with this key may ask for by header, out of the ones its tenant allows; at most 64; empty clears the list")
	return setAPIKeyCmd
}
