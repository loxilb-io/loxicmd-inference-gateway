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
package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// fakeGateway is an HTTP server that answers the configuration-lifecycle
// endpoints with whatever a test needs it to. Every failure class the CLI
// claims to handle is reachable from here, which is the point: a live gateway
// cannot be asked to fail on demand in a dozen specific ways.
type fakeGateway struct {
	server *httptest.Server
	// handler answers every request; tests replace it per case.
	handler func(w http.ResponseWriter, r *http.Request)
	// requests records what the CLI actually sent.
	requests []recordedRequest
}

type recordedRequest struct {
	Method string
	Path   string
	Query  url.Values
	Body   string
}

func newFakeGateway(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *fakeGateway {
	t.Helper()
	gw := &fakeGateway{handler: handler}
	gw.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
		}
		gw.requests = append(gw.requests, recordedRequest{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Body: string(body),
		})
		gw.handler(w, r)
	}))
	t.Cleanup(gw.server.Close)
	return gw
}

// options points the CLI at the fake gateway.
func (g *fakeGateway) options() *api.RESTOptions {
	u, err := url.Parse(g.server.URL)
	if err != nil {
		panic(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		panic(err)
	}
	return &api.RESTOptions{Protocol: "http", ServerIP: u.Hostname(), ServerPort: port, Timeout: 10}
}

// jsonResponse writes a status and a raw body.
func jsonResponse(status int, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// snapshotDocument builds a document whose checksum is correct for its own
// bytes, exactly as the gateway's canonical encoder produces it: compact JSON,
// checksum computed over the same bytes with the checksum value emptied.
func snapshotDocument(t *testing.T, schemaVersion string, generation uint64) []byte {
	t.Helper()
	body := `{"schema_version":"` + schemaVersion + `","kind":"loxilb-config-snapshot",` +
		`"generation":` + strconv.FormatUint(generation, 10) + `,` +
		`"included_domains":["loadbalancer","endpoint"],"excluded_domains":["conntrack"],` +
		`"domains":{"loadbalancer":[]},"checksum":""}`
	sum := sha256.Sum256([]byte(body))
	checksum := "sha256:" + hex.EncodeToString(sum[:])
	return []byte(strings.Replace(body, `"checksum":""`, `"checksum":"`+checksum+`"`, 1))
}

// documentChecksum reads back the checksum a test document carries.
func documentChecksum(t *testing.T, document []byte) string {
	t.Helper()
	_, result, err := api.VerifySnapshotChecksum(document, "")
	if err != nil {
		t.Fatalf("test fixture does not verify: %v", err)
	}
	return result.Checksum
}

// uncheckedDocument builds a document with no checksum field at all, the way a
// gateway older than the checksummed format answers.
func uncheckedDocument() []byte {
	return []byte(`{"schema_version":"1.0","kind":"loxilb-config-snapshot","domains":{}}`)
}

// requireReason asserts that err is a lifecycle failure with the given stable
// reason code. Every failure class must carry one: an error without a code is
// an error automation cannot branch on.
func requireReason(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected failure with reason %q, got success", want)
	}
	if got := api.ReasonOf(err); got != want {
		t.Fatalf("reason = %q, want %q (error: %v)", got, want, err)
	}
}
