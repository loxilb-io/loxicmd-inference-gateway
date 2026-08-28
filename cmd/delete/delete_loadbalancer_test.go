/*
 * Copyright (c) 2026 LoxiLB Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
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
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

func TestLoadBalancerDeleteQueryPreservesModelRuleKey(t *testing.T) {
	query := loadBalancerDeleteQuery(true, 7, "/v1/chat completions", "exact", "llama/70b")
	want := map[string]string{
		"bgp":             "true",
		"block":           "7",
		"path_prefix":     "/v1/chat completions",
		"path_match_mode": "exact",
		"model_name":      "llama/70b",
	}
	for key, value := range want {
		if query[key] != value {
			t.Fatalf("query[%q] = %q, want %q", key, query[key], value)
		}
	}

	client := api.NewLoxiClient(&api.RESTOptions{ServerIP: "127.0.0.1", ServerPort: 11111, Protocol: "http"})
	url := client.LoadBalancer().Query(query).GetUrlString()
	for _, encoded := range []string{"model_name=llama%2F70b", "path_prefix=%2Fv1%2Fchat+completions"} {
		if !strings.Contains(url, encoded) {
			t.Fatalf("delete URL %q does not contain %q", url, encoded)
		}
	}
}

func TestLoadBalancerDeleteQueryOmitsEmptyModelFields(t *testing.T) {
	query := loadBalancerDeleteQuery(false, 0, "", "", "")
	for _, key := range []string{"path_prefix", "path_match_mode", "model_name"} {
		if _, exists := query[key]; exists {
			t.Fatalf("empty optional field %q was emitted", key)
		}
	}
}
