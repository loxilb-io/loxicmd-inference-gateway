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

// AINetIdentity is the client for the network-identity bindings
// (/config/ai/netidentity): POST creates or replaces one, GET lists them,
// DELETE /{name} removes one. There is no PUT or PATCH.
type AINetIdentity struct {
	CommonAPI
}

// NetIdentityEntry is the swagger NetIdentityEntry: the tenant of the
// requests that reach AI services from a network. name, match and tenant
// are required by the gateway; the rest are sent when set.
type NetIdentityEntry struct {
	Name            string           `json:"name"`
	Match           NetIdentityMatch `json:"match"`
	Services        []string         `json:"services,omitempty"`
	Tenant          string           `json:"tenant"`
	AllowedModels   []string         `json:"allowed_models,omitempty"`
	Enforce         string           `json:"enforce,omitempty"`
	Credential      string           `json:"credential,omitempty"`
	ForwardIdentity bool             `json:"forward_identity,omitempty"`
	PriorityClass   string           `json:"priority_class,omitempty"`
	FailMode        string           `json:"fail_mode,omitempty"`
}

// NetIdentityMatch is the source a binding applies to: exactly one of
// cidr, ingress_if, vlan, vni and ipsec_tunnel, which the gateway judges.
type NetIdentityMatch struct {
	Cidr        []string `json:"cidr,omitempty"`
	IngressIf   string   `json:"ingress_if,omitempty"`
	Vlan        uint32   `json:"vlan,omitempty"`
	Vni         uint32   `json:"vni,omitempty"`
	IpsecTunnel string   `json:"ipsec_tunnel,omitempty"`
}

// NetIdentityList is the GET /config/ai/netidentity answer.
type NetIdentityList struct {
	Bindings []NetIdentityEntry `json:"netIdentityAttr"`
}

// Enumerations of NetIdentityEntry, as the swagger lists them.
var (
	NetIdentityEnforceModes = []string{"advisory", "strict"}
	NetIdentityCredentials  = []string{"none", "network"}
	NetIdentityFailModes    = []string{"neutral", "closed"}
)
