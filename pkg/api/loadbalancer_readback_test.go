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

package api

import (
	"encoding/json"
	"reflect"
	"testing"
)

// What the gateway reports for a rule's member timeouts and TLS hardening
// values survives being read into the model and written out again, so a rule
// that is read and sent back keeps them.
func TestLoadBalancerServiceKeepsReadBackValues(t *testing.T) {
	const reported = `{
		"timeoutMemberConnect": 1500, "timeoutMemberData": 30000, "timeoutTcpInspect": 250,
		"alpn_protocols": ["h2", "http/1.1"],
		"tls_ciphers": "TLS_AES_256_GCM_SHA384:ECDHE-RSA-AES256-GCM-SHA384",
		"tls_versions": ["TLSv1.2", "TLSv1.3"],
		"hsts_max_age": 31536000, "hsts_include_subdomains": true, "hsts_preload": true,
		"mtls_frontend": {"client_cert_mode": "required", "client_crl_path": "/etc/loxilb/crl/clients.pem"}
	}`
	var svc LoadBalancerService
	if err := json.Unmarshal([]byte(reported), &svc); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(svc)
	if err != nil {
		t.Fatal(err)
	}
	var want, got map[string]any
	_ = json.Unmarshal([]byte(reported), &want)
	_ = json.Unmarshal(out, &got)
	for key, value := range want {
		if !reflect.DeepEqual(got[key], value) {
			t.Errorf("%s: written back as %v, reported as %v", key, got[key], value)
		}
	}

	// A rule that reports none of them writes none of them.
	out, _ = json.Marshal(LoadBalancerService{})
	got = nil
	_ = json.Unmarshal(out, &got)
	for key := range want {
		if _, present := got[key]; present {
			t.Errorf("%s is written for a rule that does not report it", key)
		}
	}
}
