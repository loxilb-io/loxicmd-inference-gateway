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
package lifecycle

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

const logsPageBody = `{"logs":["2025-01-01T00:00:02Z INFO second","2025-01-01T00:00:01Z INFO first"],` +
	`"log_file":"gateway.log","log_count":2,"total_size":4096,"has_more":true,` +
	`"next_cursor":"b2Zmc2V0OjEwMjQ","scanned_bytes":128}`

const logArchivesBody = `{"archives":["gateway.log","gateway.log.1.gz"],` +
	`"archive_info":[{"name":"gateway.log","size_bytes":4096,"modified":"2025-01-01T00:00:02Z"},` +
	`{"name":"gateway.log.1.gz","size_bytes":1024,"modified":"2024-12-31T00:00:00Z"}]}`

func TestLogsGetRendersOldestFirstWithPagingHint(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, logsPageBody))
	var out bytes.Buffer
	opts := LogsOptions{Lines: 100, Level: "INFO"}
	if err := LogsGet(gw.options(), &out, false, opts); err != nil {
		t.Fatalf("get logs failed: %v", err)
	}
	got := gw.requests[0]
	if got.Method != http.MethodGet || !strings.HasSuffix(got.Path, "/logs") {
		t.Fatalf("CLI sent %s %s, want GET .../logs", got.Method, got.Path)
	}
	if got.Query.Get("lines") != "100" || got.Query.Get("level") != "INFO" || got.Query.Has("keyword") {
		t.Fatalf("query was %v, want lines=100 level=INFO and nothing else", got.Query)
	}
	text := out.String()
	first, second := strings.Index(text, "INFO first"), strings.Index(text, "INFO second")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("lines were not printed oldest first:\n%s", text)
	}
	if !strings.Contains(text, "loxicmd get logs --file gateway.log --level INFO --cursor b2Zmc2V0OjEwMjQ") {
		t.Fatalf("paging hint missing or does not repeat the file and filters:\n%s", text)
	}
}

func TestLogsGetNoMatchSaysSo(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK,
		`{"logs":[],"log_file":"gateway.log","log_count":0,"total_size":4096,"has_more":false,"scanned_bytes":4096}`))
	var out bytes.Buffer
	if err := LogsGet(gw.options(), &out, false, LogsOptions{Lines: 10, Keyword: "nothing"}); err != nil {
		t.Fatalf("get logs failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "No matching lines in gateway.log." {
		t.Fatalf("unexpected rendering of an empty page:\n%s", out.String())
	}
}

func TestLogsGetJSONIsVerbatimBody(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, logsPageBody))
	var out bytes.Buffer
	if err := LogsGet(gw.options(), &out, true, LogsOptions{Lines: 100}); err != nil {
		t.Fatalf("get logs failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != logsPageBody {
		t.Fatalf("JSON mode rewrote the body:\n%s", out.String())
	}
}

// The gateway has no upper bound on lines; the CLI holds the ceiling, and
// refuses before any request is made.
func TestLogsGetLinesOutOfRangeIsInvalidArguments(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, logsPageBody))
	for _, lines := range []int{0, -1, MaxLogLines + 1} {
		var out bytes.Buffer
		err := LogsGet(gw.options(), &out, false, LogsOptions{Lines: lines})
		requireReason(t, err, api.ReasonInvalidArguments)
	}
	if len(gw.requests) != 0 {
		t.Fatalf("an out-of-range --lines still reached the gateway: %v", gw.requests)
	}
}

func TestLogsGetRefusesAPathAsFile(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, logsPageBody))
	for _, name := range []string{"../etc/passwd", "/var/log/x", `dir\x`, ".", ".."} {
		var out bytes.Buffer
		err := LogsGet(gw.options(), &out, false, LogsOptions{Lines: 10, File: name})
		requireReason(t, err, api.ReasonInvalidArguments)
	}
	if len(gw.requests) != 0 {
		t.Fatalf("a path-shaped --file still reached the gateway: %v", gw.requests)
	}
}

func TestLogsGetUndecodableIsDecodeFailed(t *testing.T) {
	for _, body := range []string{`not json`, `{"logs":["x"]}`} {
		gw := newFakeGateway(t, jsonResponse(http.StatusOK, body))
		var out bytes.Buffer
		err := LogsGet(gw.options(), &out, false, LogsOptions{Lines: 10})
		requireReason(t, err, api.ReasonDecodeFailed)
	}
}

func TestLogsGetNon200IsStatusError(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusUnauthorized, `{"code":401,"message":"unauthorized"}`))
	var out bytes.Buffer
	err := LogsGet(gw.options(), &out, false, LogsOptions{Lines: 10})
	requireReason(t, err, api.ReasonUnauthorized)
	if out.Len() != 0 {
		t.Fatalf("a failed request still printed to stdout:\n%s", out.String())
	}
}

func TestLogArchivesListRendersTable(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, logArchivesBody))
	var out bytes.Buffer
	if err := LogArchivesList(gw.options(), &out, false); err != nil {
		t.Fatalf("get log-archives failed: %v", err)
	}
	if got := gw.requests[0]; got.Method != http.MethodGet || !strings.HasSuffix(got.Path, "/log-archives") {
		t.Fatalf("CLI sent %s %s, want GET .../log-archives", got.Method, got.Path)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "NAME") {
		t.Fatalf("expected a header and two rows:\n%s", out.String())
	}
	for _, want := range []string{"gateway.log.1.gz", "1024", "2024-12-31T00:00:00Z"} {
		if !strings.Contains(lines[2], want) {
			t.Fatalf("row missing %q:\n%s", want, out.String())
		}
	}
}

// A gateway that predates archive_info lists names alone; the rendering
// must not invent sizes for them.
func TestLogArchivesListNamesOnlyAndEmpty(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, `{"archives":["gateway.log"]}`))
	var out bytes.Buffer
	if err := LogArchivesList(gw.options(), &out, false); err != nil {
		t.Fatalf("get log-archives failed: %v", err)
	}
	rows := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(rows) != 2 || strings.Join(strings.Fields(rows[1]), " ") != "gateway.log - -" {
		t.Fatalf("names-only listing should show dashes for unknown metadata:\n%s", out.String())
	}

	gw = newFakeGateway(t, jsonResponse(http.StatusOK, `{}`))
	out.Reset()
	if err := LogArchivesList(gw.options(), &out, false); err != nil {
		t.Fatalf("get log-archives failed on an empty listing: %v", err)
	}
	if strings.TrimSpace(out.String()) != "No log archives." {
		t.Fatalf("unexpected rendering of an empty listing:\n%s", out.String())
	}
}

func TestLogArchivesListJSONIsVerbatimBody(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, logArchivesBody))
	var out bytes.Buffer
	if err := LogArchivesList(gw.options(), &out, true); err != nil {
		t.Fatalf("get log-archives failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != logArchivesBody {
		t.Fatalf("JSON mode rewrote the body:\n%s", out.String())
	}
}

func TestLogArchivesListUndecodableIsDecodeFailed(t *testing.T) {
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, `["gateway.log"]`))
	var out bytes.Buffer
	err := LogArchivesList(gw.options(), &out, false)
	requireReason(t, err, api.ReasonDecodeFailed)
}

func archiveGateway(t *testing.T, name string, payload []byte) *fakeGateway {
	t.Helper()
	return newFakeGateway(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/log-archives/"+name) {
			http.Error(w, `{"code":404,"message":"not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	})
}

func TestLogArchiveDownloadStoresTheArchiveOwnerOnly(t *testing.T) {
	payload := []byte("\x1f\x8b\x08\x00compressed-bytes")
	gw := archiveGateway(t, "gateway.log.1.gz", payload)
	dest := filepath.Join(t.TempDir(), "gateway.log.1.gz")
	var out bytes.Buffer
	err := LogArchiveDownload(gw.options(), &out, Options{}, "gateway.log.1.gz", dest)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("stored archive differs from what the gateway sent (err %v)", err)
	}
	if st, _ := os.Stat(dest); st.Mode().Perm() != 0600 {
		t.Fatalf("archive stored with mode %o, want 0600", st.Mode().Perm())
	}
	if !strings.Contains(out.String(), "Archive gateway.log.1.gz written to "+dest+" (20 bytes).") {
		t.Fatalf("unexpected report:\n%s", out.String())
	}
	if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(dest), ".*.tmp-*")); len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
}

func TestLogArchiveDownloadJSONReportsTheFile(t *testing.T) {
	gw := archiveGateway(t, "gateway.log", []byte("plain"))
	dest := filepath.Join(t.TempDir(), "gateway.log")
	var out bytes.Buffer
	if err := LogArchiveDownload(gw.options(), &out, Options{JSON: true}, "gateway.log", dest); err != nil {
		t.Fatalf("download failed: %v", err)
	}
	var env struct {
		Command string `json:"command"`
		Success bool   `json:"success"`
		Data    struct {
			Archive api.LogArchiveFileResult `json:"archive"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("JSON mode did not write one envelope: %v\n%s", err, out.String())
	}
	if env.Command != "get.log-archives" || !env.Success ||
		env.Data.Archive.Name != "gateway.log" || env.Data.Archive.Path != dest || env.Data.Archive.Bytes != 5 {
		t.Fatalf("envelope does not describe the download:\n%s", out.String())
	}
}

func TestLogArchiveDownloadToStdoutIsTheBytesAlone(t *testing.T) {
	payload := []byte("line one\nline two\n")
	gw := archiveGateway(t, "gateway.log", payload)
	var out bytes.Buffer
	if err := LogArchiveDownload(gw.options(), &out, Options{}, "gateway.log", "-"); err != nil {
		t.Fatalf("download failed: %v", err)
	}
	if !bytes.Equal(out.Bytes(), payload) {
		t.Fatalf("stdout carried more than the archive:\n%s", out.String())
	}
	// The archive on stdout and the JSON result on stdout cannot coexist.
	out.Reset()
	err := LogArchiveDownload(gw.options(), &out, Options{JSON: true}, "gateway.log", "-")
	requireReason(t, err, api.ReasonInvalidArguments)
}

func TestLogArchiveDownloadRefusesBadArguments(t *testing.T) {
	gw := archiveGateway(t, "gateway.log", []byte("x"))
	dest := filepath.Join(t.TempDir(), "out")
	var out bytes.Buffer
	for _, name := range []string{"", "../gateway.log", "logs/gateway.log", ".."} {
		requireReason(t, LogArchiveDownload(gw.options(), &out, Options{}, name, dest), api.ReasonInvalidArguments)
	}
	requireReason(t, LogArchiveDownload(gw.options(), &out, Options{}, "gateway.log", ""), api.ReasonInvalidArguments)
	if len(gw.requests) != 0 {
		t.Fatalf("a refused invocation still reached the gateway: %v", gw.requests)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("a refused download still created %s", dest)
	}
}

func TestLogArchiveDownloadAbsentIsStatusError(t *testing.T) {
	gw := archiveGateway(t, "gateway.log", []byte("x"))
	dest := filepath.Join(t.TempDir(), "missing.gz")
	var out bytes.Buffer
	err := LogArchiveDownload(gw.options(), &out, Options{}, "missing.gz", dest)
	if api.HTTPStatusOf(err) != http.StatusNotFound {
		t.Fatalf("a 404 was not reported as a status failure: %v", err)
	}
	if _, serr := os.Stat(dest); !os.IsNotExist(serr) {
		t.Fatalf("a failed download still created %s", dest)
	}
}

// A response that breaks off before its declared length is a transport
// failure, and the destination must be untouched — the file that was
// there before (if any) stays, and no temporary file remains.
func TestLogArchiveDownloadTruncatedLeavesTheDestinationAlone(t *testing.T) {
	gw := newFakeGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = w.Write([]byte("only a little"))
	})
	dir := t.TempDir()
	dest := filepath.Join(dir, "gateway.log.1.gz")
	if err := os.WriteFile(dest, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := LogArchiveDownload(gw.options(), &out, Options{}, "gateway.log.1.gz", dest)
	requireReason(t, err, api.ReasonRequestFailed)
	if got, _ := os.ReadFile(dest); string(got) != "previous" {
		t.Fatalf("a truncated download replaced the previous file: %q", got)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".*.tmp-*")); len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
}
