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
package api

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The profile body carries name and issuer always and the rest when set;
// a listed profile reads back every one of the 18 members.
func TestJWTAuthProfileWire(t *testing.T) {
	b, _ := json.Marshal(JWTAuthProfileEntry{Name: "keycloak", Issuer: "https://sso.example.com/realms/ai"})
	if string(b) != `{"name":"keycloak","issuer":"https://sso.example.com/realms/ai"}` {
		t.Fatalf("minimal body %s", b)
	}
	full := JWTAuthProfileEntry{Name: "okta", Issuer: "https://example.okta.com", JwksURL: "https://example.okta.com/keys",
		Audiences: []string{"ai-gateway", "api"}, Algs: []string{"RS256", "ES256"}, LeewaySec: 60, RefreshSec: 600,
		TenantClaim: "org_id", UserClaim: "sub", ModelsClaim: "models", RolesClaim: "groups", ModelRolePrefix: "llm:",
		UsernameClaim: "email", ModelAuthz: "allow-all", DefaultTenant: "public", ForwardIdentity: true,
		AuthorizationPassthrough: true, PriorityClass: "critical"}
	if reflect.TypeOf(full).NumField() != 18 {
		t.Fatalf("JWTAuthProfileEntry has %d fields, the swagger 18", reflect.TypeOf(full).NumField())
	}
	b, _ = json.Marshal(full)
	const want = `{"name":"okta","issuer":"https://example.okta.com","jwks_url":"https://example.okta.com/keys","audiences":["ai-gateway","api"],` +
		`"algs":["RS256","ES256"],"leeway_sec":60,"refresh_sec":600,"tenant_claim":"org_id","user_claim":"sub","models_claim":"models",` +
		`"roles_claim":"groups","model_role_prefix":"llm:","username_claim":"email","model_authz":"allow-all","default_tenant":"public",` +
		`"forward_identity":true,"authorization_passthrough":true,"priority_class":"critical"}`
	if string(b) != want {
		t.Fatalf("full body %s", b)
	}
	var list JWTAuthProfileList
	if err := json.Unmarshal([]byte(`{"jwtAuthProfileAttr":[`+want+`]}`), &list); err != nil || len(list.Profiles) != 1 || !reflect.DeepEqual(list.Profiles[0], full) {
		t.Fatalf("list read as %+v, %v", list, err)
	}
	if !reflect.DeepEqual(JWTAuthProfileModelAuthz, []string{"claims-required", "allow-all"}) {
		t.Fatalf("model_authz enum %v", JWTAuthProfileModelAuthz)
	}
}
