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

// PII is the client for PII detection (Presidio integration, /config/pii).
// Subpaths: enable (toggle), configure, url-patterns, status, stats.
type PII struct {
	CommonAPI
}

// PIIEnableRequest is the POST /config/pii/enable body.
type PIIEnableRequest struct {
	Enabled bool `json:"enabled"`
}

// PIIConfigEntry is the POST /config/pii/configure body. All fields are
// optional; only the ones the user sets are sent (pointer + omitempty), so an
// unset flag never overwrites server-side defaults.
type PIIConfigEntry struct {
	Mode           *string  `json:"mode,omitempty"`      // detect, mask, redact, anonymize
	Direction      *string  `json:"direction,omitempty"` // both, request, response
	FailMode       *string  `json:"fail_mode,omitempty"` // open, closed
	ScanMode       *string  `json:"scan_mode,omitempty"` // full, truncate
	AnalyzerURL    *string  `json:"analyzer_url,omitempty"`
	AnonymizerURL  *string  `json:"anonymizer_url,omitempty"`
	ScoreThreshold *float64 `json:"score_threshold,omitempty"` // 0.0-1.0
	TimeoutMs      *int64   `json:"timeout_ms,omitempty"`
	MaxBodySize    *int64   `json:"max_body_size,omitempty"`
	MinBodySize    *int64   `json:"min_body_size,omitempty"`
	EnableV2       *bool    `json:"enable_v2,omitempty"`
	DefaultOper    *string  `json:"default_operator,omitempty"` // replace, redact, hash, mask, encrypt
	EncryptionKey  *string  `json:"encryption_key,omitempty"`
	BatchSize      *int64   `json:"batch_size,omitempty"`
}

// PIIURLPattern is a single URL scan pattern.
type PIIURLPattern struct {
	Pattern   string `json:"pattern"`
	IsExclude bool   `json:"is_exclude"`
}

// PIIURLPatternsEntry is the POST /config/pii/url-patterns body.
type PIIURLPatternsEntry struct {
	Mode     string          `json:"mode"` // add, replace, clear
	Patterns []PIIURLPattern `json:"patterns,omitempty"`
}
