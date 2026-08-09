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
	"net/http"
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
