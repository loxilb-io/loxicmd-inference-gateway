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
package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func opts() *RESTOptions {
	return &RESTOptions{Protocol: "http", ServerIP: "127.0.0.1", ServerPort: 11111}
}

// Path/query construction for the AI resources must match the swagger routes.
func TestAIResourceURLs(t *testing.T) {
	c := NewLoxiClient(opts())
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"apikey create/list base", c.AIApiKey().GetUrlString(),
			"http://127.0.0.1:11111/netlox/v1/config/ai/apikey"},
		{"apikey get one", c.AIApiKey().SubResources([]string{"lxb_x"}).GetUrlString(),
			"http://127.0.0.1:11111/netlox/v1/config/ai/apikey/lxb_x"},
		{"apikey list by tenant", c.AIApiKey().Query(map[string]string{"tenant_id": "t-a"}).GetUrlString(),
			"http://127.0.0.1:11111/netlox/v1/config/ai/apikey?tenant_id=t-a"},
		{"ratelimit base (POST)", c.AITenantRatelimit().GetUrlString(),
			"http://127.0.0.1:11111/netlox/v1/config/ai/tenant/ratelimit"},
		{"ratelimit get by tenant", c.AITenantRatelimit().SubResources([]string{"t-a"}).GetUrlString(),
			"http://127.0.0.1:11111/netlox/v1/config/ai/tenant/ratelimit/t-a"},
		{"kv inventory query", c.AIKvInventory().Query(map[string]string{"service_id": "3", "ep_idx": "0"}).GetUrlString(),
			"http://127.0.0.1:11111/netlox/v1/config/ai/kv/inventory?ep_idx=0&service_id=3"},
		{"user create", c.User().GetUrlString(),
			"http://127.0.0.1:11111/netlox/v1/auth/users"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

// Create-key body must match the cicd payload keys, and omit unset fields.
func TestAIApiKeyCreateBody(t *testing.T) {
	enabled := true
	req := AIApiKeyCreateRequest{
		TenantID: "cicd-tenant", Name: "cicd-key-1",
		AllowedModels: []string{"Qwen/Qwen3-0.6B", "llama-3"},
		RateLimitRps:  5, BurstSize: 10, TokensPerMin: 1000, Enabled: &enabled,
	}
	b, _ := json.Marshal(req)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"tenant_id", "name", "allowed_models", "rate_limit_rps", "burst_size", "tokens_per_min", "enabled"} {
		if _, ok := m[k]; !ok {
			t.Errorf("create body missing key %q", k)
		}
	}
	if _, ok := m["expires_at"]; ok {
		t.Errorf("expires_at should be omitted when unset")
	}
}

// PATCH body: only the fields being changed appear; enabled=false must survive.
func TestAIApiKeyPatchBody(t *testing.T) {
	// allowed-models only
	b, _ := json.Marshal(AIApiKeyPatchRequest{AllowedModels: []string{"mistral-7b"}})
	if string(b) != `{"allowed_models":["mistral-7b"]}` {
		t.Errorf("patch allowed-models body = %s", b)
	}
	// enabled=false only (pointer keeps the false value)
	f := false
	b, _ = json.Marshal(AIApiKeyPatchRequest{Enabled: &f})
	if string(b) != `{"enabled":false}` {
		t.Errorf("patch enabled body = %s", b)
	}
}

func TestAIRatelimitBody(t *testing.T) {
	b, _ := json.Marshal(AITenantRateLimitMod{TenantID: "cicd-tenant", Rps: 50, TokensPerMin: 2000})
	if string(b) != `{"tenant_id":"cicd-tenant","rps":50,"tokens_per_min":2000}` {
		t.Errorf("ratelimit body = %s", b)
	}
}

// The new PATCH verb must issue an HTTP PATCH with the JSON body and Bearer auth.
func TestUpdateVerbPatch(t *testing.T) {
	var method, gotBody, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	rc := &RESTClient{Options: RESTOptions{Token: "jwt", BearerAuth: true}, Client: srv.Client()}
	f := false
	body, _ := json.Marshal(AIApiKeyPatchRequest{Enabled: &f})
	resp, err := rc.PATCH(context.Background(), srv.URL, body)
	if err != nil {
		t.Fatalf("PATCH: %v", err)
	}
	defer resp.Body.Close()
	if method != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", method)
	}
	if gotBody != `{"enabled":false}` {
		t.Errorf("body = %s", gotBody)
	}
	if auth != "Bearer jwt" {
		t.Errorf("auth = %q, want Bearer jwt", auth)
	}
}
