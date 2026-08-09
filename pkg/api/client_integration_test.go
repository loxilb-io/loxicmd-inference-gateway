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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestBearerAuthOverTLS exercises the full client path: NewLoxiClient's TLS
// transport talking to a self-signed HTTPS server with --insecure, the Bearer
// auth header, and decoding a SimpleError response body.
func TestBearerAuthOverTLS(t *testing.T) {
	var gotAuth string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
	}))
	defer srv.Close()

	// Build a client the way NewLoxiClient would for `--protocol https --insecure`.
	tlsCfg, err := buildTLSConfig(&RESTOptions{Protocol: "https", Insecure: true})
	if err != nil {
		t.Fatalf("buildTLSConfig: %v", err)
	}
	rc := &RESTClient{
		Options: RESTOptions{Token: "jwt123", BearerAuth: true},
		Client:  &http.Client{Transport: &http.Transport{TLSClientConfig: tlsCfg}},
	}

	resp, err := rc.GET(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("GET over self-signed TLS failed: %v", err)
	}
	defer resp.Body.Close()

	if gotAuth != "Bearer jwt123" {
		t.Fatalf("server saw Authorization = %q, want %q", gotAuth, "Bearer jwt123")
	}

	body, _ := io.ReadAll(resp.Body)
	apiErr := NewAPIError(resp.StatusCode, body)
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("StatusCode = %d, want 429", apiErr.StatusCode)
	}
	if apiErr.Message != "rate limit exceeded" {
		t.Fatalf("decoded message = %q, want %q", apiErr.Message, "rate limit exceeded")
	}
}
