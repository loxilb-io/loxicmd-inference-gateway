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

// Package goldens pins the released command surface byte for byte: exit
// status, stdout and stderr of representative success-path invocations from
// every public command family, captured through the packaged binary against
// a fake gateway. These files are the compatibility contract the exit-code
// and error-handling migration must not move — any diff here is a released
// surface changing, and must be a conscious decision, never a side effect.
//
// Regenerate with:
//
//	go test ./cmd/goldens/ -run TestGolden -update
//
// and review the diff like source.
package goldens

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files with the observed output")

// fakeGateway answers every request with one canned response and records
// nothing: goldens pin what the CLI shows the operator, not what it sent.
type fakeGateway struct {
	server *httptest.Server
}

func newFakeGateway(t *testing.T, status int, body string) *fakeGateway {
	t.Helper()
	gw := &fakeGateway{}
	gw.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(gw.server.Close)
	return gw
}

func (g *fakeGateway) hostPort(t *testing.T) (string, string) {
	t.Helper()
	u, err := url.Parse(g.server.URL)
	if err != nil {
		t.Fatalf("parse fake gateway URL: %v", err)
	}
	return u.Hostname(), u.Port()
}

// buildCLI builds the packaged binary once per test run. Goldens must pin
// the binary automation runs, not an in-process approximation.
func buildCLI(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping packaged-binary test in short mode")
	}
	if runtime.GOOS != "linux" {
		t.Skip("loxicmd links Linux netlink; the packaged binary only builds there")
	}
	binary := filepath.Join(t.TempDir(), "loxicmd")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the CLI failed: %v\n%s", err, out)
	}
	return binary
}

// oneLbBody is a one-rule load-balancer listing, shaped like the gateway's
// GET /config/loadbalancer/all answer.
const oneLbBody = `{"lbAttr":[{` +
	`"serviceArguments":{"externalIP":"20.20.20.1","port":2020,"protocol":"tcp",` +
	`"sel":0,"mode":0,"BGP":false,"Monitor":false,"inactiveTimeOut":240,"block":0,"name":"test-lb"},` +
	`"secondaryIPs":[],"allowedSources":[],` +
	`"endpoints":[{"endpointIP":"31.31.31.1","targetPort":8080,"weight":1,"state":"active","counter":""}]}]}`

// durablePersistBody mirrors the gateway's POST /config/persist answer under
// the durability contract (same shape the lifecycle tests use).
const durablePersistBody = `{"result":"ok","path":"/etc/loxilb/snapshot.json",` +
	`"checksum":"sha256:abc","schema_version":"1.5","generation":7,` +
	`"included_domains":["loadbalancer","endpoint"],"excluded_domains":["conntrack"],` +
	`"external_dependencies":[{"type":"api-key-db","id":"keys","required":true,"status":"ready"}]}`

const successBody = `{"result":"Success"}`

// maintenanceBody mirrors the gateway's GET/PUT /maintenance answer while a
// drain window is open — the same shape the lifecycle tests pin. Every value
// is server-provided, so the rendering is deterministic.
const maintenanceBody = `{"state":"maintenance","operation_id":"maint-1800000000-1",` +
	`"refusing_new_config":true,"refusing_new_inference":false,"in_flight_streams":3,` +
	`"entered_at":"2027-01-15T10:00:00.000Z","elapsed_seconds":42,"drain_timeout_seconds":300,` +
	`"drain_deadline_exceeded":false,"cancellable":true}`

// readyBody mirrors GET /status/ready on a ready gateway with one attached
// and one detached interface, so both renderings are pinned.
const readyBody = `{"ready":true,"reasons":[],` +
	`"ebpf_attachments":[{"name":"eno1","mode":"tc","attached":true},` +
	`{"name":"eno2","mode":"tc","attached":false}]}`

// diagnosticsBody mirrors GET /diagnostics with every assembly section
// populated once.
const diagnosticsBody = `{"version":"v1.2.3","build_info":"rev abc","product":"loxilb-inference-gateway",` +
	`"api_version":"/netlox/v1 0.0.1","uptime_seconds":42,"ready":true,"ready_reasons":[],` +
	`"maintenance_state":"active",` +
	`"ebpf_attachments":[{"name":"eno1","mode":"tc","attached":true}],` +
	`"maps":[{"name":"conntrack","count":10,"capacity":1000}],` +
	`"external_dependencies":[{"type":"keystore","required":true,"status":"ready","latency_class":"fast"}]}`

// goldenCase is one pinned invocation. Every case runs against a fake
// gateway that answers with the given canned response.
type goldenCase struct {
	name   string
	args   []string
	status int
	body   string
}

var goldenCases = []goldenCase{
	{"get-lb-human", []string{"get", "lb"}, http.StatusOK, oneLbBody},
	{"get-lb-json", []string{"get", "lb", "-o", "json"}, http.StatusOK, oneLbBody},
	{"create-lb", []string{"create", "lb", "20.20.20.1", "--tcp=2020:8080", "--endpoints=31.31.31.1:1"}, http.StatusOK, successBody},
	{"delete-lb", []string{"delete", "lb", "20.20.20.1", "--tcp=2020"}, http.StatusOK, successBody},
	{"save-api", []string{"save", "--api"}, http.StatusOK, durablePersistBody},
	{"create-persist-json", []string{"create", "persist", "-o", "json"}, http.StatusOK, durablePersistBody},
	{"version-human", []string{"version"}, http.StatusOK, ""},
	{"version-json", []string{"version", "-o", "json"}, http.StatusOK, ""},
	{"completion-bash", []string{"completion", "bash"}, http.StatusOK, ""},
	{"get-maintenance-human", []string{"get", "maintenance"}, http.StatusOK, maintenanceBody},
	{"get-maintenance-json", []string{"get", "maintenance", "-o", "json"}, http.StatusOK, maintenanceBody},
	{"set-maintenance-on", []string{"set", "maintenance", "on", "--drain-timeout", "300"}, http.StatusOK, maintenanceBody},
	{"get-ready-human", []string{"get", "ready"}, http.StatusOK, readyBody},
	{"get-ready-json", []string{"get", "ready", "-o", "json"}, http.StatusOK, readyBody},
	{"get-diagnostics-human", []string{"get", "diagnostics"}, http.StatusOK, diagnosticsBody},
}

// normalize replaces the only run-dependent value — the Go toolchain of the
// test build — so goldens survive a toolchain bump without pinning less.
func normalize(s string) string {
	return strings.ReplaceAll(s, runtime.Version(), "<goversion>")
}

func TestGolden(t *testing.T) {
	binary := buildCLI(t)

	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			gw := newFakeGateway(t, tc.status, tc.body)
			host, port := gw.hostPort(t)
			args := append([]string{"-s", host, "-p", port}, tc.args...)
			cmd := exec.Command(binary, args...)
			var stdout, stderr strings.Builder
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()
			status := 0
			if exitErr, ok := err.(*exec.ExitError); ok {
				status = exitErr.ExitCode()
			} else if err != nil {
				t.Fatalf("running the CLI failed: %v", err)
			}

			got := fmt.Sprintf("exit=%d\n--- stdout ---\n%s--- stderr ---\n%s",
				status, normalize(stdout.String()), normalize(stderr.String()))
			path := filepath.Join("testdata", tc.name+".golden")

			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatalf("mkdir testdata: %v", err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				t.Logf("wrote %s (%d bytes)", path, len(got))
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (regenerate with -update): %v", err)
			}
			if got != string(want) {
				t.Errorf("released surface moved for %q.\nThis must be a conscious decision: inspect the diff, then regenerate with -update.\n--- got ---\n%s\n--- want ---\n%s",
					strconv.Quote(strings.Join(tc.args, " ")), got, want)
			}
		})
	}
}
