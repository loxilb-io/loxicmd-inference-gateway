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

// L4Trace is the client for L4 connection tracing (/config/l4trace).
// Subpaths: enable (optional sampling body), disable, status, sampling (PUT),
// stats/reset.
type L4Trace struct {
	CommonAPI
}

// L4TraceEnableRequest is the optional POST /config/l4trace/enable body.
// SamplingRate is a pointer so it is only sent when explicitly requested.
type L4TraceEnableRequest struct {
	SamplingRate *int64 `json:"sampling_rate,omitempty"` // 0-100
}

// L4TraceSamplingRequest is the PUT /config/l4trace/sampling body.
type L4TraceSamplingRequest struct {
	SamplingRate int64 `json:"sampling_rate"` // 0-100, required
}
