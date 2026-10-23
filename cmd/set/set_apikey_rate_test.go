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
package set

import (
	"encoding/json"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// A rate flag given goes on the wire with its value, zero included; one
// not given stays off the wire. The other patchable fields ride the same
// rule.
func TestAPIKeyPatchFromFlags(t *testing.T) {
	for name, c := range map[string]struct {
		args []string
		want string
	}{
		"three given":       {[]string{"--rate-limit-rps=20", "--burst-size=40", "--tokens-per-min=5000"}, `{"rate_limit_rps":20,"burst_size":40,"tokens_per_min":5000}`},
		"zero is a value":   {[]string{"--rate-limit-rps=0", "--tokens-per-min=0"}, `{"rate_limit_rps":0,"tokens_per_min":0}`},
		"one given":         {[]string{"--burst-size=8"}, `{"burst_size":8}`},
		"beside the others": {[]string{"--enabled=false", "--rate-limit-rps=5", "--priority-class="}, `{"enabled":false,"rate_limit_rps":5,"priority_class":""}`},
		"none given":        {nil, `{}`},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := NewSetAPIKeyCmd(&api.RESTOptions{})
			if err := cmd.Flags().Parse(c.args); err != nil {
				t.Fatal(err)
			}
			// The options struct is bound to the flag set inside the constructor;
			// read the values back through it.
			o := SetAPIKeyOptions{}
			o.RateLimitRps, _ = cmd.Flags().GetInt64("rate-limit-rps")
			o.BurstSize, _ = cmd.Flags().GetInt64("burst-size")
			o.TokensPerMin, _ = cmd.Flags().GetInt64("tokens-per-min")
			o.Enabled, _ = cmd.Flags().GetBool("enabled")
			o.PriorityClass, _ = cmd.Flags().GetString("priority-class")
			b, _ := json.Marshal(apiKeyPatch(cmd.Flags(), &o))
			if string(b) != c.want {
				t.Fatalf("body %s, want %s", b, c.want)
			}
		})
	}
	t.Run("not an integer is a flag error", func(t *testing.T) {
		if err := NewSetAPIKeyCmd(&api.RESTOptions{}).Flags().Parse([]string{"--rate-limit-rps=many"}); err == nil {
			t.Fatal("accepted")
		}
	})
	t.Run("the flags are registered", func(t *testing.T) {
		flags := NewSetAPIKeyCmd(&api.RESTOptions{}).Flags()
		for _, name := range []string{"rate-limit-rps", "burst-size", "tokens-per-min"} {
			if f := flags.Lookup(name); f == nil || f.Value.Type() != "int64" {
				t.Fatalf("--%s not registered as int64", name)
			}
		}
	})
}
