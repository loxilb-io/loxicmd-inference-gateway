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

// OPA is the client for the OPA L4 policy watcher (/config/opa/watcher).
// This is a raw-middleware endpoint (see api/swagger-extras.yml); its error
// bodies use the SimpleError {"error": "..."} envelope.
// GET returns status; POST configures/starts the watcher; DELETE stops it.
type OPA struct {
	CommonAPI
}

// OPAWatcherConfig is the POST /config/opa/watcher body. opa_url is required;
// policy_path/poll_interval_sec/fail_open are optional (server defaults
// loxilb/l4 and 30s apply when omitted).
type OPAWatcherConfig struct {
	OpaURL          string `json:"opa_url"`
	PolicyPath      string `json:"policy_path,omitempty"`
	PollIntervalSec int    `json:"poll_interval_sec,omitempty"`
	FailOpen        bool   `json:"fail_open,omitempty"`
}
