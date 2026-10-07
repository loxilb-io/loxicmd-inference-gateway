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
package create

import (
	"encoding/json"
	"testing"
)

func certBodyMap(t *testing.T, o *CreateCertOptions) map[string]any {
	t.Helper()
	body, err := certCreateBody(o, "CERT", "KEY", "")
	if err != nil {
		t.Fatalf("usage %q rejected: %v", o.Usage, err)
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

// An upload without --usage is what it was before the flag: no usage member.
// A client entry carries its key. A CA bundle has no key, and the keyPem
// member the schema requires is sent empty, never left out.
func TestCertCreateBody(t *testing.T) {
	m := certBodyMap(t, &CreateCertOptions{CertID: "web", CertFile: "c", KeyFile: "k"})
	assertKey(t, m, "keyPem", "KEY")
	assertAbsent(t, m, "usage")

	m = certBodyMap(t, &CreateCertOptions{CertID: "be-client", CertFile: "c", KeyFile: "k", Usage: "client"})
	assertKey(t, m, "usage", "client")
	assertKey(t, m, "keyPem", "KEY")

	m = certBodyMap(t, &CreateCertOptions{CertID: "be-ca", CertFile: "c", Usage: "ca"})
	assertKey(t, m, "usage", "ca")
	assertKey(t, m, "certPem", "CERT")
	assertKey(t, m, "keyPem", "")

	for name, o := range map[string]CreateCertOptions{
		"a CA with a key":         {CertFile: "c", KeyFile: "k", Usage: "ca"},
		"a client without a key":  {CertFile: "c", Usage: "client"},
		"a server without a key":  {CertFile: "c", Usage: "server"},
		"no usage without a key":  {CertFile: "c"},
		"a usage that is not one": {CertFile: "c", KeyFile: "k", Usage: "root"},
	} {
		if _, err := certCreateBody(&o, "CERT", "KEY", ""); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The ID printed is the one the gateway answers with, so an ID it minted is
// shown. A gateway that answers without a body leaves the ID that was asked for.
func TestCertCreatedID(t *testing.T) {
	for _, c := range []struct {
		name, body, requested, want string
	}{
		{"minted by the gateway", `{"certId":"2f1c7c1e-minted"}`, "", "2f1c7c1e-minted"},
		{"named, and answered", `{"certId":"web"}`, "web", "web"},
		{"named, no body", ``, "web", "web"},
		{"not named, no body", ``, "", ""},
		{"a body that is not the answer", `{"code":201}`, "web", "web"},
		{"a body that is not JSON", `created`, "", ""},
	} {
		if got := certCreatedID([]byte(c.body), c.requested); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
