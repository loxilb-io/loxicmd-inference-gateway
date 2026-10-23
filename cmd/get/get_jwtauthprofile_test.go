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
	"reflect"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// The default row is name, issuer, tenant claim and priority class; wide
// adds the rest in swagger order; absent prints "-", a zero number too.
func TestJWTAuthProfileRendering(t *testing.T) {
	p := api.JWTAuthProfileEntry{Name: "okta", Issuer: "https://example.okta.com", JwksURL: "https://example.okta.com/keys",
		Audiences: []string{"ai-gateway", "api"}, Algs: []string{"RS256"}, LeewaySec: 60, RefreshSec: 600, TenantClaim: "org_id",
		UserClaim: "sub", ModelsClaim: "models", RolesClaim: "groups", ModelRolePrefix: "llm:", UsernameClaim: "email",
		ModelAuthz: "allow-all", DefaultTenant: "public", ForwardIdentity: true, AuthorizationPassthrough: true, PriorityClass: "critical"}
	if got := jwtAuthProfileRow(p, false); !reflect.DeepEqual(got, []string{"okta", "https://example.okta.com", "org_id", "critical"}) {
		t.Fatalf("row %v", got)
	}
	want := []string{"okta", "https://example.okta.com", "org_id", "critical", "https://example.okta.com/keys", "ai-gateway,api", "RS256",
		"60", "600", "sub", "models", "groups", "llm:", "email", "allow-all", "public", "true", "true"}
	if got := jwtAuthProfileRow(p, true); !reflect.DeepEqual(got, want) {
		t.Fatalf("wide row %v", got)
	}
	bare := api.JWTAuthProfileEntry{Name: "x", Issuer: "https://i"}
	if got := jwtAuthProfileRow(bare, true); !reflect.DeepEqual(got, []string{"x", "https://i", "-", "-", "-", "-", "-", "-", "-", "-", "-", "-", "-", "-", "-", "-", "false", "false"}) {
		t.Fatalf("bare wide row %v", got)
	}
	if len(jwtAuthProfileRow(bare, true)) != len(JWTAUTHPROFILE_WIDE_TITLE) || len(jwtAuthProfileRow(bare, false)) != len(JWTAUTHPROFILE_TITLE) {
		t.Fatal("row and title widths differ")
	}
}
