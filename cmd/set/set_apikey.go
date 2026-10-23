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
	"github.com/spf13/pflag"
)

// SetAPIKeyOptions holds the values of the set apikey flags; which of them
// were given is read from the flag set, since every one is optional and an
// omitted flag leaves the stored value.
type SetAPIKeyOptions struct {
	AllowedModels     []string
	Enabled           bool
	RateLimitRps      int64
	BurstSize         int64
	TokensPerMin      int64
	PriorityClass     string
	DefaultObjective  string
	AllowedObjectives []string
}

// setAPIKeyFlags are the patchable fields, in the swagger's order.
var setAPIKeyFlags = []string{"allowed-models", "enabled", "rate-limit-rps", "burst-size", "tokens-per-min", "priority-class", "default-objective", "allowed-objectives"}

func NewSetAPIKeyCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := SetAPIKeyOptions{}

	var setAPIKeyCmd = &cobra.Command{
		Use:   "apikey <KEY-ID> [--allowed-models=<m>,] [--enabled] [--rate-limit-rps=<n>] [--burst-size=<n>] [--tokens-per-min=<n>] [--priority-class=<class>] [--default-objective=<name>] [--allowed-objectives=<name>,]",
		Short: "Update an inference-gateway API key",
		Long: `Update the allowed model list, enabled state, rate limits and/or priority
of an API key (PATCH /config/ai/apikey/{key_id}). Only the flags you set
are changed; an empty --priority-class, --default-objective or
--allowed-objectives clears the stored value. A rate flag given as 0 is
sent as 0, which the gateway reads as no per-key request limit, a burst
equal to one second's allowance, or no per-key token quota; a rate flag
not given keeps the stored value. The data plane rebuilds the key's bucket
when the limit changes, so the next request is decided against the new
value.

ex)
	loxicmd set apikey lxb_abc123 --allowed-models=mistral-7b
	loxicmd set apikey lxb_abc123 --enabled=false
	loxicmd set apikey lxb_abc123 --rate-limit-rps=20 --burst-size=40 --tokens-per-min=5000
	loxicmd set apikey lxb_abc123 --priority-class=critical --default-objective=batch`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exitcode.Usagef("set apikey needs <KEY-ID> arg")
			}
			given := false
			for _, name := range setAPIKeyFlags {
				given = given || cmd.Flags().Changed(name)
			}
			if !given {
				return exitcode.Usagef("set at least one of --%s", strings.Join(setAPIKeyFlags, ", --"))
			}
			if cmd.Flags().Changed("priority-class") && o.PriorityClass != "" && !api.IsPriorityClass(o.PriorityClass) {
				return exitcode.Invalidf("--priority-class must be one of %s, or empty to clear it", strings.Join(api.PriorityClasses, "|"))
			}
			if cmd.Flags().Changed("priority-class") || cmd.Flags().Changed("default-objective") || cmd.Flags().Changed("allowed-objectives") {
				if err := featuregate.Require(restOptions, featuregate.Objectives, "set apikey", cmd.ErrOrStderr()); err != nil {
					return err
				}
			}
			req := apiKeyPatch(cmd.Flags(), &o)

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
	f := setAPIKeyCmd.Flags()
	f.StringSliceVar(&o.AllowedModels, "allowed-models", nil, "Replacement allowed model list")
	f.BoolVar(&o.Enabled, "enabled", true, "Enable or disable the key")
	f.Int64Var(&o.RateLimitRps, "rate-limit-rps", 0, "Requests per second allowed on this key; 0 is an explicit no per-key request limit, not unset")
	f.Int64Var(&o.BurstSize, "burst-size", 0, "Bucket capacity for --rate-limit-rps; 0 means the enforcement path falls back to the rate as the capacity, so burst equals one second's allowance")
	f.Int64Var(&o.TokensPerMin, "tokens-per-min", 0, "LLM tokens per minute allowed on this key; 0 is an explicit no per-key token quota. Enforced post-hoc: a charge that puts the bucket in debt denies the next request, not the one in flight")
	f.StringVar(&o.PriorityClass, "priority-class", "", "The highest priority class a request with this key may have: standard, critical or sheddable; empty clears it (no cap from the key)")
	f.StringVar(&o.DefaultObjective, "default-objective", "", "The objective a request with this key gets when it names none; empty clears it")
	f.StringSliceVar(&o.AllowedObjectives, "allowed-objectives", nil, "The objectives a request with this key may ask for by header, out of the ones its tenant allows; at most 64; empty clears the list")
	return setAPIKeyCmd
}

// apiKeyPatch builds the PATCH body from the flags that were given: each
// one goes on the wire with its value, a zero included; a flag not given
// stays off the wire and the gateway keeps the stored value.
func apiKeyPatch(flags *pflag.FlagSet, o *SetAPIKeyOptions) api.AIApiKeyPatchRequest {
	req := api.AIApiKeyPatchRequest{}
	if flags.Changed("allowed-models") {
		req.AllowedModels = o.AllowedModels
	}
	if flags.Changed("enabled") {
		e := o.Enabled
		req.Enabled = &e
	}
	if flags.Changed("rate-limit-rps") {
		v := o.RateLimitRps
		req.RateLimitRps = &v
	}
	if flags.Changed("burst-size") {
		v := o.BurstSize
		req.BurstSize = &v
	}
	if flags.Changed("tokens-per-min") {
		v := o.TokensPerMin
		req.TokensPerMin = &v
	}
	if flags.Changed("priority-class") {
		v := o.PriorityClass
		req.PriorityClass = &v
	}
	if flags.Changed("default-objective") {
		v := o.DefaultObjective
		req.DefaultObjective = &v
	}
	if flags.Changed("allowed-objectives") {
		list := append([]string{}, o.AllowedObjectives...)
		req.AllowedObjectives = &list
	}
	return req
}
