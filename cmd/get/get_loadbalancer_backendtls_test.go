/*
 * Copyright (c) 2026 NetLOX Inc
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
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

func TestBackendTLSCell(t *testing.T) {
	for _, c := range []struct {
		name string
		eff  *api.BackendTLSEffective
		want string
	}{
		{"a rule without a TLS backend leg", nil, "-"},
		{"a gateway that reports no status", &api.BackendTLSEffective{}, "-"},
		{"no listener yet", &api.BackendTLSEffective{Status: "pending", CA: "none"}, "pending"},
		{"a build that cannot verify", &api.BackendTLSEffective{Status: "unsupported", CA: "none"}, "unsupported"},
		{"an unverified leg", &api.BackendTLSEffective{Status: "applied", CA: "none"}, "applied: no verify"},
		{"verified", &api.BackendTLSEffective{Status: "applied", Verify: true, CA: "be-ca"}, "applied: verify"},
		{"verified, with a client certificate and a name", &api.BackendTLSEffective{Status: "applied", Verify: true,
			CA: "be-ca", ClientCert: true, ClientCertID: "be-client", ServerName: "be.example.com"},
			"applied: verify, client, name"},
		{"the listener runs something else", &api.BackendTLSEffective{Status: "failed", Verify: true, CA: "old-ca"},
			"failed: verify"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := backendTLSCell(c.eff); got != c.want {
				t.Fatalf("cell %q, want %q", got, c.want)
			}
			// The table wraps a cell at 30 characters, onto the rows of the
			// rule's other endpoints.
			if len(c.want) > 30 {
				t.Fatalf("cell %q is wider than the table's column", c.want)
			}
		})
	}
}
