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
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/featuregate"

	"github.com/spf13/cobra"
)

type CreateJWTAuthProfileOptions struct {
	Issuer                   string
	JwksURL                  string
	Audiences                []string
	Algs                     []string
	LeewaySec                int64
	RefreshSec               int64
	TenantClaim              string
	UserClaim                string
	ModelsClaim              string
	RolesClaim               string
	ModelRolePrefix          string
	UsernameClaim            string
	ModelAuthz               string
	DefaultTenant            string
	ForwardIdentity          bool
	AuthorizationPassthrough bool
	PriorityClass            string
}

func NewCreateJWTAuthProfileCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := CreateJWTAuthProfileOptions{}

	var createJWTAuthProfileCmd = &cobra.Command{
		Use:     "jwtauthprofile <NAME> --issuer=<url> [--jwks-url=<url>] [--audiences=<aud>,] [--algs=<alg>,] [--leeway-sec=<n>] [--refresh-sec=<n>] [--tenant-claim=<path>] [--user-claim=<path>] [--models-claim=<path>] [--roles-claim=<path>] [--model-role-prefix=<prefix>] [--username-claim=<path>] [--model-authz=claims-required|allow-all] [--default-tenant=<tenant>] [--forward-identity] [--authorization-passthrough] [--priority-class=<class>]",
		Short:   "Create or replace an inference-gateway JWT auth profile",
		Aliases: []string{"jwtauthprofiles", "jwtprofile"},
		Long: `Create or replace a JWT auth profile: a named issuer configuration for
bearer-token admission that LB rules reference by name (POST
/config/ai/jwtauthprofile). Posting an existing name replaces the profile
and restarts its key lifecycle: the profile answers 503-class on the data
plane until the first JWKS fetch against the new configuration succeeds.
Only public key material is ever fetched or held. A successful create is
not proof the issuer is reachable; activation does not wait for the fetch.

Claim extraction is dot-paths because the identity provider owns the
claim schema; the defaults are Keycloak-shaped. A numeric flag left at
zero selects the gateway's documented default. What the gateway refuses
(an issuer that is not an http(s) URL, an algorithm it never accepts) is
printed as it answers.

ex)
	loxicmd create jwtauthprofile keycloak --issuer=https://sso.example.com/realms/ai
	loxicmd create jwtauthprofile okta --issuer=https://example.okta.com --audiences=ai-gateway --tenant-claim=org_id --priority-class=critical`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exitcode.Usagef("create jwtauthprofile needs <NAME> arg")
			}
			if o.Issuer == "" {
				return exitcode.Usagef("--issuer is required")
			}
			if err := validateJWTAuthProfileOptions(&o); err != nil {
				return exitcode.Invalidf("%s", err.Error())
			}
			entry := jwtAuthProfileEntry(args[0], &o)

			if err := featuregate.Require(restOptions, featuregate.JWTAuthProfile, "create jwtauthprofile", cmd.ErrOrStderr()); err != nil {
				return err
			}
			if o.PriorityClass != "" {
				if err := featuregate.Require(restOptions, featuregate.Objectives, "create jwtauthprofile --priority-class", cmd.ErrOrStderr()); err != nil {
					return err
				}
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.AIJWTAuthProfile().Create(ctx, entry)
			if err != nil {
				return exitcode.Unavailablef("create jwtauthprofile: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				return featuregate.Classify("create jwtauthprofile", featuregate.JWTAuthProfile, resp.StatusCode, body)
			}
			return printOperationResult(cmd.OutOrStdout(), body, restOptions.PrintOption)
		},
	}
	f := createJWTAuthProfileCmd.Flags()
	f.StringVar(&o.Issuer, "issuer", "", "Exact string the token iss claim must equal; an http(s) URL. When --jwks-url is unset it is also the base for OIDC discovery (issuer + /.well-known/openid-configuration) (required)")
	f.StringVar(&o.JwksURL, "jwks-url", "", "Overrides OIDC discovery of the JWKS endpoint when set")
	f.StringSliceVar(&o.Audiences, "audiences", nil, "Accept-list matched against the token aud values and azp; at least one must match. Empty skips the audience check")
	f.StringSliceVar(&o.Algs, "algs", nil, "Signature-algorithm accept-list (RS256/RS384/RS512, ES256/ES384/ES512, PS256/PS384/PS512); default RS256,ES256. alg=none and HMAC are refused by the gateway unconditionally")
	f.Int64Var(&o.LeewaySec, "leeway-sec", 0, "Clock-skew allowance in seconds for exp/nbf/iat (gateway default 30)")
	f.Int64Var(&o.RefreshSec, "refresh-sec", 0, "Periodic JWKS refresh interval in seconds (gateway default 3600)")
	f.StringVar(&o.TenantClaim, "tenant-claim", "", "Dot-path to the tenant identity claim (gateway default tenant_id)")
	f.StringVar(&o.UserClaim, "user-claim", "", "Dot-path to the stable user identity claim (gateway default sub)")
	f.StringVar(&o.ModelsClaim, "models-claim", "", "Dot-path to a string array of allowed models. Unset derives models from roles via --model-role-prefix; when set and present in the token it is authoritative, even when empty")
	f.StringVar(&o.RolesClaim, "roles-claim", "", "Dot-path to the roles array (gateway default realm_access.roles)")
	f.StringVar(&o.ModelRolePrefix, "model-role-prefix", "", "Prefix filter turning roles into allowed models (gateway default \"model:\")")
	f.StringVar(&o.UsernameClaim, "username-claim", "", "Dot-path to a display-only username (gateway default preferred_username)")
	f.StringVar(&o.ModelAuthz, "model-authz", "", "What happens when no model list can be derived from the token: claims-required (default) denies every model; allow-all admits any model for an otherwise-valid token")
	f.StringVar(&o.DefaultTenant, "default-tenant", "", "Tenant used when the tenant claim is absent. Empty means such tokens are denied (401): an unattributable request cannot be metered")
	f.BoolVar(&o.ForwardIdentity, "forward-identity", false, "Inject verified X-Auth-Tenant/X-Auth-User headers upstream (client-sent copies are always stripped)")
	f.BoolVar(&o.AuthorizationPassthrough, "authorization-passthrough", false, "Leave the client Authorization header on the upstream request instead of stripping it after verification")
	f.StringVar(&o.PriorityClass, "priority-class", "", "The highest priority class a request admitted by this profile's tokens may have: standard, critical or sheddable. It can only lower what the tenant allows; omitted means no cap from the profile")
	return createJWTAuthProfileCmd
}

// validateJWTAuthProfileOptions checks the flags for format only: the two
// enums. The issuer's URL form, the algorithm accept-list and the claim
// paths are the gateway's 400, printed as it answers.
func validateJWTAuthProfileOptions(o *CreateJWTAuthProfileOptions) error {
	for _, c := range []struct {
		flag, value string
		allowed     []string
	}{
		{"--model-authz", o.ModelAuthz, api.JWTAuthProfileModelAuthz},
		{"--priority-class", o.PriorityClass, api.PriorityClasses},
	} {
		if c.value == "" {
			continue
		}
		ok := false
		for _, a := range c.allowed {
			ok = ok || a == c.value
		}
		if !ok {
			return fmt.Errorf("%s must be one of %s", c.flag, strings.Join(c.allowed, "|"))
		}
	}
	return nil
}

// jwtAuthProfileEntry builds the body from the flags, field for field.
func jwtAuthProfileEntry(name string, o *CreateJWTAuthProfileOptions) api.JWTAuthProfileEntry {
	return api.JWTAuthProfileEntry{
		Name:                     name,
		Issuer:                   o.Issuer,
		JwksURL:                  o.JwksURL,
		Audiences:                o.Audiences,
		Algs:                     o.Algs,
		LeewaySec:                o.LeewaySec,
		RefreshSec:               o.RefreshSec,
		TenantClaim:              o.TenantClaim,
		UserClaim:                o.UserClaim,
		ModelsClaim:              o.ModelsClaim,
		RolesClaim:               o.RolesClaim,
		ModelRolePrefix:          o.ModelRolePrefix,
		UsernameClaim:            o.UsernameClaim,
		ModelAuthz:               o.ModelAuthz,
		DefaultTenant:            o.DefaultTenant,
		ForwardIdentity:          o.ForwardIdentity,
		AuthorizationPassthrough: o.AuthorizationPassthrough,
		PriorityClass:            o.PriorityClass,
	}
}
