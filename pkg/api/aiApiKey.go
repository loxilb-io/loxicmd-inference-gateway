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

// AIApiKey is the client for per-tenant inference-gateway API keys
// (/config/ai/apikey). Data-plane enforcement is selected independently on
// each load-balancer service through serviceArguments.api_key_auth.
type AIApiKey struct {
	CommonAPI
}

// AIApiKeyCreateRequest is the POST /config/ai/apikey body (ApiKeyCreateRequest).
type AIApiKeyCreateRequest struct {
	TenantID      string   `json:"tenant_id"`
	Name          string   `json:"name,omitempty"`
	APIKey        string   `json:"api_key,omitempty"`
	AllowedModels []string `json:"allowed_models,omitempty"`
	RateLimitRps  int64    `json:"rate_limit_rps,omitempty"`
	BurstSize     int64    `json:"burst_size,omitempty"`
	TokensPerMin  int64    `json:"tokens_per_min,omitempty"`
	ExpiresAt     string   `json:"expires_at,omitempty"`
	Enabled       *bool    `json:"enabled,omitempty"`
	// The key's priority: the highest class a request with it may have
	// (a cap, never a raise), the objective it gets when it names none,
	// and the objectives it may ask for by header, out of the tenant's.
	PriorityClass     string   `json:"priority_class,omitempty"`
	DefaultObjective  string   `json:"default_objective,omitempty"`
	AllowedObjectives []string `json:"allowed_objectives,omitempty"`
}

// PriorityClasses are the priority classes a key, tenant, JWT profile or
// network identity may cap a request at.
var PriorityClasses = []string{"standard", "critical", "sheddable"}

// IsPriorityClass reports whether s names one of PriorityClasses.
func IsPriorityClass(s string) bool {
	for _, c := range PriorityClasses {
		if c == s {
			return true
		}
	}
	return false
}

// AIApiKeyCreateResponse is the 201 response. raw_key is the plaintext key and
// is returned ONLY once, at creation.
type AIApiKeyCreateResponse struct {
	RawKey string `json:"raw_key,omitempty"`
	KeyID  string `json:"key_id"`
}

// AIApiKeySummary is the GET response item (ApiKeySummary); it never carries the
// raw key.
type AIApiKeySummary struct {
	KeyID         string   `json:"key_id"`
	TenantID      string   `json:"tenant_id"`
	Name          string   `json:"name"`
	AllowedModels []string `json:"allowed_models"`
	RateLimitRps  int64    `json:"rate_limit_rps"`
	BurstSize     int64    `json:"burst_size"`
	TokensPerMin  int64    `json:"tokens_per_min"`
	CreatedAt     string   `json:"created_at"`
	ExpiresAt     string   `json:"expires_at"`
	Enabled       bool     `json:"enabled"`
	// Absent when the key sets none.
	PriorityClass     string   `json:"priority_class,omitempty"`
	DefaultObjective  string   `json:"default_objective,omitempty"`
	AllowedObjectives []string `json:"allowed_objectives,omitempty"`
}

// AIApiKeyPatchRequest is the PATCH /config/ai/apikey/{key_id} body
// (swagger-extras raw middleware). Every field is optional: omitted leaves
// the stored value. The class fields are pointers because an empty string
// (or an empty list) clears the stored value, while nil stays off the wire;
// the three rate fields are pointers because zero is a value the swagger
// gives a meaning to (no per-key limit, burst equal to one second's
// allowance, no token quota), not "unset".
type AIApiKeyPatchRequest struct {
	AllowedModels     []string  `json:"allowed_models,omitempty"`
	Enabled           *bool     `json:"enabled,omitempty"`
	RateLimitRps      *int64    `json:"rate_limit_rps,omitempty"`
	BurstSize         *int64    `json:"burst_size,omitempty"`
	TokensPerMin      *int64    `json:"tokens_per_min,omitempty"`
	PriorityClass     *string   `json:"priority_class,omitempty"`
	DefaultObjective  *string   `json:"default_objective,omitempty"`
	AllowedObjectives *[]string `json:"allowed_objectives,omitempty"`
}
