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
	"strings"
	"testing"
)

func TestParseErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"SimpleError envelope (swagger-extras)", `{"error":"invalid service_id"}`, "invalid service_id"},
		{"Error envelope (generated spec)", `{"message":"not found"}`, "not found"},
		{"loxilb result envelope", `{"result":"Success"}`, "Success"},
		{"msg key", `{"msg":"boom"}`, "boom"},
		{"unknown json falls back to raw body", `{"foo":"bar"}`, `{"foo":"bar"}`},
		{"plain text body", "  bad gateway\n", "bad gateway"},
		{"empty body", "", ""},
		{"empty error field falls back to raw", `{"error":""}`, `{"error":""}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseErrorMessage([]byte(tc.body))
			if got != tc.want {
				t.Fatalf("ParseErrorMessage(%q) = %q, want %q", tc.body, got, tc.want)
			}
		})
	}
}

func TestNewAPIError(t *testing.T) {
	e := NewAPIError(429, []byte(`{"error":"rate limit exceeded"}`))
	if e.StatusCode != 429 {
		t.Fatalf("StatusCode = %d, want 429", e.StatusCode)
	}
	if e.Message != "rate limit exceeded" {
		t.Fatalf("Message = %q, want %q", e.Message, "rate limit exceeded")
	}
	if !strings.Contains(e.Error(), "429") || !strings.Contains(e.Error(), "rate limit exceeded") {
		t.Fatalf("Error() = %q, want it to mention status and message", e.Error())
	}
}
