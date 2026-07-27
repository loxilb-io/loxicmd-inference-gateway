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

// LlamaFirewall is the client for AI security scanning (/config/llamafirewall).
// Subpaths: enable (toggle), configure, scanners, status, stats, health.
type LlamaFirewall struct {
	CommonAPI
}

// LlamaFirewallEnableRequest is the POST /config/llamafirewall/enable body.
type LlamaFirewallEnableRequest struct {
	Enabled bool `json:"enabled"`
}

// LlamaFirewallConfigEntry is the POST /config/llamafirewall/configure body.
// Optional fields use pointers so an unset flag is not sent.
type LlamaFirewallConfigEntry struct {
	ServerURL          *string  `json:"server_url,omitempty"`
	TimeoutSec         *int64   `json:"timeout_sec,omitempty"`
	FailClosed         *bool    `json:"fail_closed,omitempty"`
	BlockThreshold     *float64 `json:"block_threshold,omitempty"` // 0.0-1.0
	CacheEnabled       *bool    `json:"cache_enabled,omitempty"`
	CacheTTLSec        *int64   `json:"cache_ttl_sec,omitempty"`
	ConnectionPoolSize *int64   `json:"connection_pool_size,omitempty"`
	ScanPatterns       []string `json:"scan_patterns,omitempty"`
	SkipPatterns       []string `json:"skip_patterns,omitempty"`
}

// LlamaFirewallScannersEntry is the POST /config/llamafirewall/scanners body.
// Each scanner toggle is optional (pointer) so only changed scanners are sent.
type LlamaFirewallScannersEntry struct {
	PromptGuard    *bool `json:"prompt_guard,omitempty"`
	CodeShield     *bool `json:"code_shield,omitempty"`
	Regex          *bool `json:"regex,omitempty"`
	HiddenASCII    *bool `json:"hidden_ascii,omitempty"`
	AgentAlignment *bool `json:"agent_alignment,omitempty"`
	PIIDetection   *bool `json:"pii_detection,omitempty"`
}
