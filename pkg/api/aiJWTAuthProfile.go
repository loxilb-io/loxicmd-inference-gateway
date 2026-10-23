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

// AIJWTAuthProfile is the client for the JWT auth profiles
// (/config/ai/jwtauthprofile): POST creates or replaces one, GET lists
// them, DELETE /{name} removes one (409 while a rule references it).
// There is no PUT or PATCH.
type AIJWTAuthProfile struct {
	CommonAPI
}

// JWTAuthProfileEntry is the swagger JWTAuthProfileEntry: a named issuer
// configuration for bearer-token admission that LB rules reference by
// name. name and issuer are required by the gateway; the rest are sent
// when set, since the swagger states that zero or absent selects the
// documented default for every numeric field.
type JWTAuthProfileEntry struct {
	Name                     string   `json:"name"`
	Issuer                   string   `json:"issuer"`
	JwksURL                  string   `json:"jwks_url,omitempty"`
	Audiences                []string `json:"audiences,omitempty"`
	Algs                     []string `json:"algs,omitempty"`
	LeewaySec                int64    `json:"leeway_sec,omitempty"`
	RefreshSec               int64    `json:"refresh_sec,omitempty"`
	TenantClaim              string   `json:"tenant_claim,omitempty"`
	UserClaim                string   `json:"user_claim,omitempty"`
	ModelsClaim              string   `json:"models_claim,omitempty"`
	RolesClaim               string   `json:"roles_claim,omitempty"`
	ModelRolePrefix          string   `json:"model_role_prefix,omitempty"`
	UsernameClaim            string   `json:"username_claim,omitempty"`
	ModelAuthz               string   `json:"model_authz,omitempty"`
	DefaultTenant            string   `json:"default_tenant,omitempty"`
	ForwardIdentity          bool     `json:"forward_identity,omitempty"`
	AuthorizationPassthrough bool     `json:"authorization_passthrough,omitempty"`
	PriorityClass            string   `json:"priority_class,omitempty"`
}

// JWTAuthProfileList is the GET /config/ai/jwtauthprofile answer.
type JWTAuthProfileList struct {
	Profiles []JWTAuthProfileEntry `json:"jwtAuthProfileAttr"`
}

// JWTAuthProfileModelAuthz is the model_authz enumeration, as the swagger
// lists it; priority_class shares PriorityClasses.
var JWTAuthProfileModelAuthz = []string{"claims-required", "allow-all"}
