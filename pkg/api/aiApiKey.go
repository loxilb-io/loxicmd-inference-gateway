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
// (/config/ai/apikey). Note: today these are control-plane CRUD only —
// data-plane enforcement (401/403/429) is on the roadmap.
type AIApiKey struct {
	CommonAPI
}

// AIApiKeyCreateRequest is the POST /config/ai/apikey body (ApiKeyCreateRequest).
type AIApiKeyCreateRequest struct {
	TenantID      string   `json:"tenant_id"`
	Name          string   `json:"name,omitempty"`
	AllowedModels []string `json:"allowed_models,omitempty"`
	RateLimitRps  int64    `json:"rate_limit_rps,omitempty"`
	BurstSize     int64    `json:"burst_size,omitempty"`
	TokensPerMin  int64    `json:"tokens_per_min,omitempty"`
	ExpiresAt     string   `json:"expires_at,omitempty"`
	Enabled       *bool    `json:"enabled,omitempty"`
}

// AIApiKeyCreateResponse is the 201 response. raw_key is the plaintext key and
// is returned ONLY once, at creation.
type AIApiKeyCreateResponse struct {
	RawKey string `json:"raw_key"`
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
}

// AIApiKeyPatchRequest is the PATCH /config/ai/apikey/{key_id} body
// (swagger-extras raw middleware). Both fields optional.
type AIApiKeyPatchRequest struct {
	AllowedModels []string `json:"allowed_models,omitempty"`
	Enabled       *bool    `json:"enabled,omitempty"`
}
