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

// SNICertificate is the client for the SNI certificate store (/sni/certificates).
type SNICertificate struct {
	CommonAPI
}

// SNICertificateEntry is the POST body. certPath is optional (defaults to
// /opt/loxilb/cert/{hostname}).
type SNICertificateEntry struct {
	Hostname string `json:"hostname"`
	CertPath string `json:"certPath,omitempty"`
}

// SNICertificateItem is one row in the GET response.
type SNICertificateItem struct {
	Hostname string `json:"hostname"`
	CertPath string `json:"certPath"`
	RefCount int    `json:"refCount"`
}

// SNICertificateListResponse is the GET /sni/certificates response.
type SNICertificateListResponse struct {
	Certificates      []SNICertificateItem `json:"certificates"`
	TotalCertificates int                  `json:"totalCertificates"`
}

// SNICertificateDeleteRequest is the DELETE body (hostname only, not the full entry).
type SNICertificateDeleteRequest struct {
	Hostname string `json:"hostname"`
}

// SuccessResponse is the {message} envelope returned by SNI POST/DELETE.
type SuccessResponse struct {
	Message string `json:"message"`
}
