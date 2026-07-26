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
	"encoding/json"
	"fmt"
	"strings"
)

// APIError is a decoded non-2xx response from the gateway. It normalizes the
// two error envelopes the server uses: the generated spec's Error/ErrorResponse
// object and the raw swagger-extras middleware's SimpleError ({"error": "..."}).
type APIError struct {
	StatusCode int
	Message    string // best-effort human-readable message
	Body       string // trimmed raw response body, for diagnostics
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("server returned %d: %s", e.StatusCode, e.Message)
	}
	if e.Body != "" {
		return fmt.Sprintf("server returned %d: %s", e.StatusCode, e.Body)
	}
	return fmt.Sprintf("server returned %d", e.StatusCode)
}

// errorMessageKeys are the fields, in priority order, that the various loxilb /
// inference-gateway error envelopes use to carry a human-readable message.
var errorMessageKeys = []string{
	"error",   // SimpleError (swagger-extras raw middleware)
	"message", // Error / ErrorResponse (generated spec)
	"msg",
	"result", // loxilb create/delete result envelope
	"Error",
	"Message",
}

// ParseErrorMessage extracts a human-readable message from an error response
// body, tolerating both known envelopes and unknown/empty shapes. It returns
// the trimmed raw body when no recognized field is present.
func ParseErrorMessage(body []byte) string {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err == nil {
		for _, k := range errorMessageKeys {
			if v, ok := m[k]; ok {
				if s, ok := v.(string); ok {
					if s = strings.TrimSpace(s); s != "" {
						return s
					}
				}
			}
		}
	}
	return strings.TrimSpace(string(body))
}

// NewAPIError builds an APIError from an HTTP status code and raw response body.
func NewAPIError(statusCode int, body []byte) *APIError {
	return &APIError{
		StatusCode: statusCode,
		Message:    ParseErrorMessage(body),
		Body:       strings.TrimSpace(string(body)),
	}
}
