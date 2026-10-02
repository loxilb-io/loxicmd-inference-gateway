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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetAuthHeader(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		bearer  bool
		wantHdr string // "" means the header must be absent
	}{
		{"bearer prefixes raw token", "jwt123", true, "Bearer jwt123"},
		{"bearer does not double-prefix", "Bearer jwt123", true, "Bearer jwt123"},
		{"raw mode leaves token untouched", "jwt123", false, "jwt123"},
		{"whitespace is trimmed before prefixing", "  jwt123\n", true, "Bearer jwt123"},
		{"empty token sets no header", "", true, ""},
		{"whitespace-only token sets no header", "   ", true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &RESTClient{Options: RESTOptions{Token: tc.token, BearerAuth: tc.bearer}}
			req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/x", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			r.setAuthHeader(req)
			got := req.Header.Get("Authorization")
			if got != tc.wantHdr {
				t.Fatalf("Authorization = %q, want %q", got, tc.wantHdr)
			}
		})
	}
}

// TestOriginatorHeaderOnEveryVerb pins that the originator rides on each
// kind of request the client can make, and on none when it is not set: a
// verb that skipped it would leave that class of change unattributed.
func TestOriginatorHeaderOnEveryVerb(t *testing.T) {
	verbs := map[string]func(*RESTClient, context.Context, string) (*http.Response, error){
		"GET": func(r *RESTClient, ctx context.Context, u string) (*http.Response, error) { return r.GET(ctx, u) },
		"POST": func(r *RESTClient, ctx context.Context, u string) (*http.Response, error) {
			return r.POST(ctx, u, []byte(`{}`))
		},
		"DELETE": func(r *RESTClient, ctx context.Context, u string) (*http.Response, error) { return r.DELETE(ctx, u) },
		"DELETEWithBody": func(r *RESTClient, ctx context.Context, u string) (*http.Response, error) {
			return r.DELETEWithBody(ctx, u, []byte(`{}`))
		},
		"PATCH": func(r *RESTClient, ctx context.Context, u string) (*http.Response, error) {
			return r.PATCH(ctx, u, []byte(`{}`))
		},
		"PUT": func(r *RESTClient, ctx context.Context, u string) (*http.Response, error) {
			return r.PUT(ctx, u, []byte(`{}`))
		},
	}
	for _, originator := range []string{"cli:svc@host-1", ""} {
		for name, do := range verbs {
			t.Run(name+"/"+originator, func(t *testing.T) {
				var got []string
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					got = r.Header.Values(OriginatorHeader)
				}))
				defer srv.Close()
				r := &RESTClient{Options: RESTOptions{Originator: originator}, Client: srv.Client()}
				resp, err := do(r, context.Background(), srv.URL)
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				resp.Body.Close()
				if originator == "" {
					if len(got) != 0 {
						t.Fatalf("%s = %q, want the header absent", OriginatorHeader, got)
					}
					return
				}
				if len(got) != 1 || got[0] != originator {
					t.Fatalf("%s = %q, want exactly %q", OriginatorHeader, got, originator)
				}
			})
		}
	}
}

// TestCLIOriginator pins the value's shape and that everything the gateway
// would drop silently is refused here instead.
func TestCLIOriginator(t *testing.T) {
	tests := []struct {
		name, user, host string
		want             string // "" means an error is expected
	}{
		{"plain", "svc-deploy", "ctl-1.example.net", "cli:svc-deploy@ctl-1.example.net"},
		{"exactly at the limit", "u", strings.Repeat("h", 256-len("cli:u@")), "cli:u@" + strings.Repeat("h", 256-len("cli:u@"))},
		{"one byte over the limit", "u", strings.Repeat("h", 257-len("cli:u@")), ""},
		{"empty user", "", "host", ""},
		{"empty host", "user", "", ""},
		{"non-ASCII user", "osé", "host", ""},
		{"control byte in host", "user", "ho\tst", ""},
		{"DEL in host", "user", "ho\x7fst", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CLIOriginator(tc.user, tc.host)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("CLIOriginator(%q, %q) = %q, want an error", tc.user, tc.host, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("CLIOriginator(%q, %q) = %q, %v; want %q", tc.user, tc.host, got, err, tc.want)
			}
		})
	}
}
