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
	"fmt"
	"net/http"
)

// OriginatorHeader names who a request is made on behalf of. The gateway
// records its value beside the authenticated account in the audit trail,
// never in place of it, and whether the value is trusted is decided there
// from that account's own delegation permission.
const OriginatorHeader = "X-Loxilb-Originator"

const (
	originatorScheme   = "cli:"
	originatorMaxBytes = 256
)

// CLIOriginator builds the value the CLI sends: "cli:<os user>@<host>".
// The gateway keeps a value only if it is at most 256 bytes of printable
// ASCII and drops anything else without telling the caller, so a value
// that would be dropped is refused here, where the operator can see why.
func CLIOriginator(osUser, host string) (string, error) {
	if osUser == "" {
		return "", fmt.Errorf("the OS user name is empty")
	}
	if host == "" {
		return "", fmt.Errorf("the host name is empty")
	}
	v := originatorScheme + osUser + "@" + host
	if len(v) > originatorMaxBytes {
		return "", fmt.Errorf("%q is %d bytes; the gateway accepts at most %d", v, len(v), originatorMaxBytes)
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 0x20 || v[i] > 0x7e {
			return "", fmt.Errorf("%q has a byte outside printable ASCII at offset %d", v, i)
		}
	}
	return v, nil
}

// setOriginatorHeader names the originator on the request when one is
// configured. No header is set otherwise, so a plain invocation reads in
// the audit trail as the authenticated account acting for itself.
func (r *RESTClient) setOriginatorHeader(req *http.Request) {
	if r.Options.Originator == "" {
		return
	}
	req.Header.Set(OriginatorHeader, r.Options.Originator)
}
