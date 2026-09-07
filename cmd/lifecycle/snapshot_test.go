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
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// documentResponse serves a snapshot document with the checksum header the
// gateway stamps.
func documentResponse(document []byte, header string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if header != "" {
			w.Header().Set(snapshotChecksumHeader, header)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(document)
	}
}

func TestSnapshotWritesVerifiedDocument(t *testing.T) {
	document := snapshotDocument(t, "1.5", 7)
	gw := newFakeGateway(t, documentResponse(document, documentChecksum(t, document)))
	path := filepath.Join(t.TempDir(), "snapshot.json")
	var out, errOut bytes.Buffer

	err := Snapshot(gw.options(), &out, &errOut, Options{}, SnapshotOptions{File: path, Components: "loadbalancer"})
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if got := gw.requests[0].Query.Get("components"); got != "loadbalancer" {
		t.Fatalf("components = %q", got)
	}
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, document) {
		t.Fatalf("stored document differs from what the gateway sent")
	}
	// A snapshot document carries the full configuration, including
	// encrypted secret material.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Fatalf("stored mode = %04o, want 0600", perm)
		}
	}
	text := out.String()
	for _, want := range []string{"Snapshot written to " + path, "schema 1.5", "generation 7", "Checksum verified"} {
		if !strings.Contains(text, want) {
			t.Fatalf("output missing %q:\n%s", want, text)
		}
	}
}

// TestSnapshotDoesNotClobberOnBadDownload is the point of the whole path: the
// destination already holds a good snapshot, and a failed download must leave
// it exactly as it was.
func TestSnapshotDoesNotClobberOnBadDownload(t *testing.T) {
	good := snapshotDocument(t, "1.5", 7)
	tampered := snapshotDocument(t, "1.5", 7)
	// Flip a byte inside the document body, leaving its checksum field
	// intact - a corrupted transfer, not a re-signed document.
	tampered = bytes.Replace(tampered, []byte(`"kind":"loxilb-snapshot"`),
		[]byte(`"kind":"loxilb-snapshoT"`), 1)

	for name, tc := range map[string]struct {
		body   []byte
		header string
		reason string
	}{
		"truncated download": {good[:len(good)/2], "", api.ReasonInvalidJSON},
		"empty body":         {[]byte{}, "", api.ReasonInvalidJSON},
		"corrupted body":     {tampered, "", api.ReasonChecksumMismatch},
		"header disagrees with document": {good, "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			api.ReasonChecksumMismatch},
	} {
		t.Run(name, func(t *testing.T) {
			previous := snapshotDocument(t, "1.4", 3)
			path := filepath.Join(t.TempDir(), "snapshot.json")
			if err := os.WriteFile(path, previous, 0600); err != nil {
				t.Fatal(err)
			}
			gw := newFakeGateway(t, documentResponse(tc.body, tc.header))
			var out, errOut bytes.Buffer

			err := Snapshot(gw.options(), &out, &errOut, Options{}, SnapshotOptions{File: path})
			requireReason(t, err, tc.reason)

			after, rerr := os.ReadFile(path)
			if rerr != nil {
				t.Fatalf("the previous snapshot is gone: %v", rerr)
			}
			if !bytes.Equal(after, previous) {
				t.Fatalf("a failed download overwrote the previous snapshot")
			}
			// No temporary file may be left behind either.
			entries, derr := os.ReadDir(filepath.Dir(path))
			if derr != nil {
				t.Fatal(derr)
			}
			if len(entries) != 1 {
				names := make([]string, 0, len(entries))
				for _, e := range entries {
					names = append(names, e.Name())
				}
				t.Fatalf("failed download left residue: %v", names)
			}
		})
	}
}

func TestSnapshotWithoutChecksumIsReportedNotClaimed(t *testing.T) {
	document := uncheckedDocument()
	gw := newFakeGateway(t, documentResponse(document, ""))
	path := filepath.Join(t.TempDir(), "snapshot.json")
	var out, errOut bytes.Buffer

	if err := Snapshot(gw.options(), &out, &errOut, Options{}, SnapshotOptions{File: path}); err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	text := out.String()
	if strings.Contains(text, "Checksum verified") {
		t.Fatalf("claimed a verification that never happened:\n%s", text)
	}
	if !strings.Contains(text, uncheckedDocumentNote) {
		t.Fatalf("missing the unverified note:\n%s", text)
	}

	out.Reset()
	err := Snapshot(gw.options(), &out, &errOut, Options{Strict: true}, SnapshotOptions{File: path})
	requireReason(t, err, api.ReasonContractLegacy)
}

func TestSnapshotToStdout(t *testing.T) {
	document := snapshotDocument(t, "1.5", 7)
	gw := newFakeGateway(t, documentResponse(document, documentChecksum(t, document)))
	var out, errOut bytes.Buffer

	if err := Snapshot(gw.options(), &out, &errOut, Options{}, SnapshotOptions{}); err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	// Redirecting stdout into a file must yield the document, not a report
	// about it.
	if !bytes.Equal(bytes.TrimRight(out.Bytes(), "\n"), document) {
		t.Fatalf("stdout is not the document:\n%s", out.String())
	}
}

func TestSnapshotStdoutStillRefusesACorruptDocument(t *testing.T) {
	document := snapshotDocument(t, "1.5", 7)
	corrupt := bytes.Replace(document, []byte(`"kind":"loxilb-snapshot"`),
		[]byte(`"kind":"loxilb-snapshoT"`), 1)
	gw := newFakeGateway(t, documentResponse(corrupt, ""))
	var out, errOut bytes.Buffer

	err := Snapshot(gw.options(), &out, &errOut, Options{}, SnapshotOptions{})
	requireReason(t, err, api.ReasonChecksumMismatch)
	if out.Len() != 0 {
		t.Fatalf("a corrupt document was printed anyway:\n%s", out.String())
	}
}

func TestSnapshotFailureClasses(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		reason string
	}{
		"unauthorized":     {http.StatusUnauthorized, `{"message":"Invalid authentication credentials"}`, api.ReasonUnauthorized},
		"gate busy":        {http.StatusConflict, `{"message":"Another snapshot or restore operation is in progress"}`, api.ReasonBusy},
		"maintenance mode": {http.StatusServiceUnavailable, `{"message":"Maintenance mode"}`, api.ReasonMaintenance},
		"bad components":   {http.StatusBadRequest, `{"message":"Invalid parameters","result":"unknown domain"}`, api.ReasonBadRequest},
		"capture failed":   {http.StatusInternalServerError, `{"message":"Internal service error"}`, api.ReasonServerError},
	} {
		t.Run(name, func(t *testing.T) {
			gw := newFakeGateway(t, jsonResponse(tc.status, tc.body))
			path := filepath.Join(t.TempDir(), "snapshot.json")
			var out, errOut bytes.Buffer
			err := Snapshot(gw.options(), &out, &errOut, Options{}, SnapshotOptions{File: path})
			requireReason(t, err, tc.reason)
			if _, serr := os.Stat(path); serr == nil {
				t.Fatal("a failed download still created the destination file")
			}
		})
	}
}

func TestSnapshotUnwritableDestination(t *testing.T) {
	document := snapshotDocument(t, "1.5", 7)
	gw := newFakeGateway(t, documentResponse(document, documentChecksum(t, document)))
	var out, errOut bytes.Buffer

	err := Snapshot(gw.options(), &out, &errOut, Options{},
		SnapshotOptions{File: filepath.Join(t.TempDir(), "no-such-directory", "snapshot.json")})
	requireReason(t, err, api.ReasonFileWrite)
}

func TestSnapshotJSONEnvelope(t *testing.T) {
	document := snapshotDocument(t, "1.5", 7)
	gw := newFakeGateway(t, documentResponse(document, documentChecksum(t, document)))
	path := filepath.Join(t.TempDir(), "snapshot.json")
	var out, errOut bytes.Buffer

	if err := Snapshot(gw.options(), &out, &errOut, Options{JSON: true}, SnapshotOptions{File: path}); err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	report := decodeReport(t, out.Bytes())
	if report.Snapshot == nil || !report.Snapshot.ChecksumVerified ||
		report.Snapshot.Path != path || report.Snapshot.Generation != 7 ||
		report.Snapshot.Bytes != len(document) {
		t.Fatalf("unexpected envelope: %+v", report.Snapshot)
	}
}
