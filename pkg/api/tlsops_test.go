/*
 * Copyright (c) 2025 LoxiLB Authors
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
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTLSOpsURLs(t *testing.T) {
	c := NewLoxiClient(opts())
	cases := []struct{ name, got, want string }{
		{"cert base (POST)", c.Cert().GetUrlString(),
			"http://127.0.0.1:11111/netlox/v1/config/cert"},
		{"cert by id", c.Cert().SubResources([]string{"web"}).GetUrlString(),
			"http://127.0.0.1:11111/netlox/v1/config/cert/web"},
		{"sni base", c.SNICertificate().GetUrlString(),
			"http://127.0.0.1:11111/netlox/v1/sni/certificates"},
		{"metrics base", c.Metrics().GetUrlString(),
			"http://127.0.0.1:11111/netlox/v1/config/metrics"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

// SNI delete must issue a DELETE with a JSON body carrying only the hostname.
func TestDeleteWithBodyVerb(t *testing.T) {
	var method, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"deleted"}`))
	}))
	defer srv.Close()

	rc := &RESTClient{Options: RESTOptions{}, Client: srv.Client()}
	// Exercise via the low-level verb with the SNI delete body shape.
	body, _ := json.Marshal(SNICertificateDeleteRequest{Hostname: "api.example.com"})
	resp, err := rc.DELETEWithBody(context.Background(), srv.URL, body)
	if err != nil {
		t.Fatalf("DELETEWithBody: %v", err)
	}
	defer resp.Body.Close()
	if method != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", method)
	}
	if gotBody != `{"hostname":"api.example.com"}` {
		t.Errorf("body = %s", gotBody)
	}
}

func TestCertModelOmitEmpty(t *testing.T) {
	// A GET-shape cert (no key) must omit keyPem; a create body must carry both.
	b, _ := json.Marshal(CertModel{CertID: "web", Hostnames: []string{"api.example.com"}})
	if strings.Contains(string(b), "keyPem") {
		t.Errorf("get-shape cert should omit keyPem, got %s", b)
	}
	b, _ = json.Marshal(CertModel{CertPem: "P", KeyPem: "K"})
	if !strings.Contains(string(b), "certPem") || !strings.Contains(string(b), "keyPem") {
		t.Errorf("create-shape cert must carry certPem+keyPem, got %s", b)
	}
}
