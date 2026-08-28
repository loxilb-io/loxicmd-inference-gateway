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

// AITenantRatelimit is the client for per-tenant rate limits
// (/config/ai/tenant/ratelimit). Quotas are enforced for traffic admitted by
// a load-balancer service whose api_key_auth policy is required.
type AITenantRatelimit struct {
	CommonAPI
}

// AITenantRateLimitMod is the POST body (TenantRateLimitMod).
type AITenantRateLimitMod struct {
	TenantID     string                   `json:"tenant_id"`
	Rps          int64                    `json:"rps,omitempty"`
	TokensPerMin int64                    `json:"tokens_per_min,omitempty"`
	BurstPct     int64                    `json:"burst_pct,omitempty"`
	ModelLimits  []AITenantModelRateLimit `json:"model_limits,omitempty"`
}

// AITenantModelRateLimit is a per-model quota update. TokensPerMin
// intentionally has no omitempty: zero is the server's deletion tombstone.
type AITenantModelRateLimit struct {
	Model        string `json:"model"`
	TokensPerMin int64  `json:"tokens_per_min"`
}

// AITenantRateLimitEntry is the GET response (TenantRateLimitEntry).
type AITenantRateLimitEntry struct {
	TenantID     string                   `json:"tenant_id"`
	Rps          int64                    `json:"rps"`
	TokensPerMin int64                    `json:"tokens_per_min"`
	BurstPct     int64                    `json:"burst_pct,omitempty"`
	ModelLimits  []AITenantModelRateLimit `json:"model_limits,omitempty"`
	UpdatedAt    string                   `json:"updated_at"`
}
