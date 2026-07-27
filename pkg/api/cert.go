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

// Cert is the client for TLS certificate management (/config/cert). There is no
// list endpoint; certs are addressed individually by certId.
type Cert struct {
	CommonAPI
}

// CertModel is the Cert schema. certPem/keyPem are required on POST/PUT; keyPem
// is never returned on GET; hostnames is output-only.
type CertModel struct {
	CertID    string   `json:"certId,omitempty"`
	CertPem   string   `json:"certPem,omitempty"`
	KeyPem    string   `json:"keyPem,omitempty"`
	ChainPem  string   `json:"chainPem,omitempty"`
	Hostnames []string `json:"hostnames,omitempty"`
}
