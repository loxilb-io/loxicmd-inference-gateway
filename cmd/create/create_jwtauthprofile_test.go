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
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// Every flag lands on its swagger field; a flag left unset is absent from
// the body so the gateway's documented default applies.
func TestJWTAuthProfileEntry(t *testing.T) {
	o := CreateJWTAuthProfileOptions{Issuer: "https://example.okta.com", JwksURL: "https://example.okta.com/keys",
		Audiences: []string{"ai-gateway"}, Algs: []string{"RS256"}, LeewaySec: 60, RefreshSec: 600, TenantClaim: "org_id",
		UserClaim: "sub", ModelsClaim: "models", RolesClaim: "groups", ModelRolePrefix: "llm:", UsernameClaim: "email",
		ModelAuthz: "allow-all", DefaultTenant: "public", ForwardIdentity: true, AuthorizationPassthrough: true, PriorityClass: "critical"}
	b, _ := json.Marshal(jwtAuthProfileEntry("okta", &o))
	const want = `{"name":"okta","issuer":"https://example.okta.com","jwks_url":"https://example.okta.com/keys","audiences":["ai-gateway"],` +
		`"algs":["RS256"],"leeway_sec":60,"refresh_sec":600,"tenant_claim":"org_id","user_claim":"sub","models_claim":"models",` +
		`"roles_claim":"groups","model_role_prefix":"llm:","username_claim":"email","model_authz":"allow-all","default_tenant":"public",` +
		`"forward_identity":true,"authorization_passthrough":true,"priority_class":"critical"}`
	if string(b) != want {
		t.Fatalf("body %s", b)
	}
	b, _ = json.Marshal(jwtAuthProfileEntry("keycloak", &CreateJWTAuthProfileOptions{Issuer: "https://sso.example.com/realms/ai"}))
	if string(b) != `{"name":"keycloak","issuer":"https://sso.example.com/realms/ai"}` {
		t.Fatalf("minimal body %s", b)
	}
}

// Format only: the two enums. The issuer's form, the algorithms and the
// claim paths are the gateway's.
func TestValidateJWTAuthProfileOptions(t *testing.T) {
	for _, c := range []struct {
		name string
		o    CreateJWTAuthProfileOptions
		want string
	}{
		{"model authz", CreateJWTAuthProfileOptions{ModelAuthz: "deny"}, "--model-authz must be one of claims-required|allow-all"},
		{"priority class", CreateJWTAuthProfileOptions{PriorityClass: "high"}, "--priority-class must be one of standard|critical|sheddable"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := validateJWTAuthProfileOptions(&c.o)
			if err == nil || err.Error() != c.want {
				t.Fatalf("error %v, want %q", err, c.want)
			}
		})
	}
	t.Run("the gateway's rules are left to it", func(t *testing.T) {
		o := CreateJWTAuthProfileOptions{Issuer: "not-a-url", Algs: []string{"HS256"}, LeewaySec: -1, ModelAuthz: "allow-all", PriorityClass: "sheddable"}
		if err := validateJWTAuthProfileOptions(&o); err != nil {
			t.Fatalf("refused locally: %v", err)
		}
	})
	t.Run("the flags are registered", func(t *testing.T) {
		flags := NewCreateJWTAuthProfileCmd(&api.RESTOptions{}).Flags()
		for name, typ := range map[string]string{"issuer": "string", "jwks-url": "string", "audiences": "stringSlice", "algs": "stringSlice",
			"leeway-sec": "int64", "refresh-sec": "int64", "tenant-claim": "string", "user-claim": "string", "models-claim": "string",
			"roles-claim": "string", "model-role-prefix": "string", "username-claim": "string", "model-authz": "string",
			"default-tenant": "string", "forward-identity": "bool", "authorization-passthrough": "bool", "priority-class": "string"} {
			f := flags.Lookup(name)
			if f == nil || f.Value.Type() != typ {
				t.Fatalf("--%s not registered as %s", name, typ)
			}
		}
	})
}
