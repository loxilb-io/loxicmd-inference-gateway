/*
 * Copyright (c) 2026 LoxiLB Authors
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

package cmd

import "runtime"

// Build identity injected with -ldflags by the Makefile and the release
// workflow, alongside the existing Version and BuildInfo. An empty value
// means the binary was built without that fact (a local unstamped build);
// the JSON output reports the empty string rather than guessing, so release
// attestation can tell a stamped binary from an unstamped one.
var (
	// SourceRevision is the full git commit SHA the binary was built from.
	SourceRevision string
	// GatewayContract identifies the gateway API contract this CLI was
	// built against: the swagger_sha256 of testdata/contracts/gateway-api.json.
	GatewayContract string
	// BuildWorkflow is the CI workflow run that produced the binary
	// (owner/repo/run-id), empty for local builds.
	BuildWorkflow string
	// SourceDateEpoch is the reproducible-timestamp input the build used,
	// as a decimal Unix epoch string.
	SourceDateEpoch string
)

// buildIdentity is the data payload of `loxicmd version -o json`. Key names
// are part of the public contract once released; the Go toolchain is read
// from the running binary itself, so it cannot be stamped wrongly.
type buildIdentity struct {
	Version         string `json:"version"`
	SourceRevision  string `json:"sourceRevision"`
	GatewayContract string `json:"gatewayContract"`
	GoVersion       string `json:"goVersion"`
	BuildWorkflow   string `json:"buildWorkflow"`
	SourceDateEpoch string `json:"sourceDateEpoch"`
	BuildInfo       string `json:"buildInfo"`
}

func currentBuildIdentity() buildIdentity {
	return buildIdentity{
		Version:         Version,
		SourceRevision:  SourceRevision,
		GatewayContract: GatewayContract,
		GoVersion:       runtime.Version(),
		BuildWorkflow:   BuildWorkflow,
		SourceDateEpoch: SourceDateEpoch,
		BuildInfo:       BuildInfo,
	}
}
