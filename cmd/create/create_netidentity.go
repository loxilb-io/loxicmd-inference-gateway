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
	"net"
	"net/http"
	"strings"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/featuregate"

	"github.com/spf13/cobra"
)

type CreateNetIdentityOptions struct {
	Tenant          string
	Cidr            []string
	IngressIf       string
	Vlan            uint32
	Vni             uint32
	IpsecTunnel     string
	Services        []string
	AllowedModels   []string
	Enforce         string
	Credential      string
	ForwardIdentity bool
	PriorityClass   string
	FailMode        string
}

func NewCreateNetIdentityCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := CreateNetIdentityOptions{}

	var createNetIdentityCmd = &cobra.Command{
		Use:     "netidentity <NAME> --tenant=<tenant> (--cidr=<a.b.c.d/len>,|--ingress-if=<if>|--vlan=<id>|--vni=<vni>|--ipsec-tunnel=<name>) [--services=<VIP:port>,] [--allowed-models=<m>,] [--enforce=advisory|strict] [--credential=none|network] [--forward-identity] [--priority-class=<class>] [--fail-mode=neutral|closed]",
		Short:   "Create or replace an inference-gateway network-identity binding",
		Aliases: []string{"netidentities", "netid"},
		Long: `Create or replace a network-identity binding: the tenant of the requests
that reach AI services from a network (POST /config/ai/netidentity).
Posting an existing name replaces the binding.

The match is exactly one of --cidr (where a request came from; it does not
prove who sent it), --ingress-if, --vlan, --vni and --ipsec-tunnel (the
interface the connection arrived on). A cidr match is for limits, fairness
and records, never in place of a credential. What the gateway refuses
(a match that is not exactly one, a credential with a cidr match, a
forward identity without services, a conflicting binding) is printed as
it answers.

ex)
	loxicmd create netidentity lab --tenant=tenant-a --cidr=10.10.0.0/16
	loxicmd create netidentity edge --tenant=tenant-b --vlan=100 --credential=network --enforce=strict`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return exitcode.Usagef("create netidentity needs <NAME> arg")
			}
			if o.Tenant == "" {
				return exitcode.Usagef("--tenant is required")
			}
			if err := validateNetIdentityOptions(&o); err != nil {
				return exitcode.Invalidf("%s", err.Error())
			}
			entry := netIdentityEntry(args[0], &o)

			if err := featuregate.Require(restOptions, featuregate.NetIdentity, "create netidentity", cmd.ErrOrStderr()); err != nil {
				return err
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.AINetIdentity().Create(ctx, entry)
			if err != nil {
				return exitcode.Unavailablef("create netidentity: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				return featuregate.Classify("create netidentity", featuregate.NetIdentity, resp.StatusCode, body)
			}
			return printOperationResult(cmd.OutOrStdout(), body, restOptions.PrintOption)
		},
	}
	f := createNetIdentityCmd.Flags()
	f.StringVar(&o.Tenant, "tenant", "", "The tenant a matching request is attributed to, 1 to 127 bytes; the same namespace as API-key and JWT tenants (required)")
	f.StringSliceVar(&o.Cidr, "cidr", nil, "IPv4 networks in a.b.c.d/len form, without host bits, that the socket peer of the request is in; at most 256")
	f.StringVar(&o.IngressIf, "ingress-if", "", "Name of the interface the connection arrived on, at most 15 characters")
	f.Uint32Var(&o.Vlan, "vlan", 0, "A VLAN ID (1 to 4094): the VLAN's bridge interface on this node, vlan<id>")
	f.Uint32Var(&o.Vni, "vni", 0, "A VXLAN network identifier (1 to 16777215): the interface vxlan<vni> on this node")
	f.StringVar(&o.IpsecTunnel, "ipsec-tunnel", "", "Name of an IPsec tunnel configured on this node: the tunnel's VTI interface")
	f.StringSliceVar(&o.Services, "services", nil, "The AI services the binding applies to, each as VIP:port; empty applies it to every AI service; at most 256")
	f.StringSliceVar(&o.AllowedModels, "allowed-models", nil, "The models a request admitted or held by this binding may name, compared exactly; empty allows any; at most 256")
	f.StringVar(&o.Enforce, "enforce", "", "advisory (default): a request that presents a credential is decided by it; strict: a credential whose tenant is not the binding's is refused with 403 network_tenant_mismatch")
	f.StringVar(&o.Credential, "credential", "", "none (default): the binding never stands in for a credential; network: on a service that requires a credential, a request that presents none is admitted as the binding's tenant (interface matches only)")
	f.BoolVar(&o.ForwardIdentity, "forward-identity", false, "Forward the request's tenant upstream as X-Auth-Tenant, and its user as X-Auth-User, on a request whose source matched this binding; needs --services")
	f.StringVar(&o.PriorityClass, "priority-class", "", "The highest priority class a request the binding names the tenant of may have: standard, critical or sheddable")
	f.StringVar(&o.FailMode, "fail-mode", "", "What the binding's services do with a request while the binding lookup itself cannot answer: neutral (default) decides it as if the service had no binding, closed refuses it with 503 netid_unavailable")
	return createNetIdentityCmd
}

// Per-value bounds of the match, as the swagger states them.
const (
	netIdentityVlanCeiling = 4094
	netIdentityVniCeiling  = 16777215
)

// validateNetIdentityOptions checks the flags for format only: enum
// membership, each value's own range and the cidr form. Which match
// fields may go together, and with which credential, is the gateway's
// 400.
func validateNetIdentityOptions(o *CreateNetIdentityOptions) error {
	for _, c := range []struct {
		flag, value string
		allowed     []string
	}{
		{"--enforce", o.Enforce, api.NetIdentityEnforceModes},
		{"--credential", o.Credential, api.NetIdentityCredentials},
		{"--fail-mode", o.FailMode, api.NetIdentityFailModes},
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
	if o.Vlan > netIdentityVlanCeiling {
		return fmt.Errorf("--vlan must be within 1..%d", netIdentityVlanCeiling)
	}
	if o.Vni > netIdentityVniCeiling {
		return fmt.Errorf("--vni must be within 1..%d", netIdentityVniCeiling)
	}
	for _, cidr := range o.Cidr {
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil || ip.To4() == nil {
			return fmt.Errorf("--cidr %q is not an IPv4 network in a.b.c.d/len form", cidr)
		}
	}
	return nil
}

// netIdentityEntry builds the body from the flags, field for field.
func netIdentityEntry(name string, o *CreateNetIdentityOptions) api.NetIdentityEntry {
	return api.NetIdentityEntry{
		Name: name,
		Match: api.NetIdentityMatch{
			Cidr: o.Cidr, IngressIf: o.IngressIf, Vlan: o.Vlan, Vni: o.Vni, IpsecTunnel: o.IpsecTunnel,
		},
		Services:        o.Services,
		Tenant:          o.Tenant,
		AllowedModels:   o.AllowedModels,
		Enforce:         o.Enforce,
		Credential:      o.Credential,
		ForwardIdentity: o.ForwardIdentity,
		PriorityClass:   o.PriorityClass,
		FailMode:        o.FailMode,
	}
}
