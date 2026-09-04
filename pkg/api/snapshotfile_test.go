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

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// checksummedDocument builds a document carrying a checksum that is correct
// for its own bytes, the way the gateway's canonical encoder produces one.
func checksummedDocument(body string) []byte {
	if !strings.Contains(body, `"checksum":""`) {
		panic("fixture must contain an empty checksum field")
	}
	sum := sha256.Sum256([]byte(body))
	checksum := "sha256:" + hex.EncodeToString(sum[:])
	return []byte(strings.Replace(body, `"checksum":""`, `"checksum":"`+checksum+`"`, 1))
}

const documentBody = `{"schema_version":"1.5","kind":"loxilb-config-snapshot","generation":9,` +
	`"domains":{"loadbalancer":[]},"checksum":""}`

func TestVerifySnapshotChecksum(t *testing.T) {
	document := checksummedDocument(documentBody)

	t.Run("accepts a document that matches its own checksum", func(t *testing.T) {
		verified, result, err := VerifySnapshotChecksum(document, "")
		if err != nil {
			t.Fatalf("verification failed: %v", err)
		}
		if !result.ChecksumVerified || result.SchemaVersion != "1.5" || result.Generation != 9 {
			t.Fatalf("unexpected identity: %+v", result)
		}
		if !bytes.Equal(verified, document) {
			t.Fatal("verified bytes differ from the document")
		}
	})

	t.Run("accepts a trailing newline without changing the verdict", func(t *testing.T) {
		_, result, err := VerifySnapshotChecksum(append(append([]byte{}, document...), '\n'), "")
		if err != nil || !result.ChecksumVerified {
			t.Fatalf("a trailing newline broke verification: %v", err)
		}
	})

	t.Run("agrees with the response header", func(t *testing.T) {
		_, result, err := VerifySnapshotChecksum(document, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := VerifySnapshotChecksum(document, result.Checksum); err != nil {
			t.Fatalf("matching header rejected: %v", err)
		}
	})

	for name, tc := range map[string]struct {
		body   []byte
		header string
		reason string
	}{
		"truncated": {document[:20], "", ReasonInvalidJSON},
		"empty":     {nil, "", ReasonInvalidJSON},
		"not json":  {[]byte("snapshot"), "", ReasonInvalidJSON},
		"one byte changed": {bytes.Replace(document, []byte(`"generation":9`),
			[]byte(`"generation":8`), 1), "", ReasonChecksumMismatch},
		"header disagrees": {document, "sha256:" + strings.Repeat("0", 64), ReasonChecksumMismatch},
		"header present but document has none": {
			[]byte(`{"schema_version":"1.0","domains":{}}`), "sha256:" + strings.Repeat("0", 64),
			ReasonChecksumMismatch},
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			_, _, err := VerifySnapshotChecksum(tc.body, tc.header)
			if err == nil {
				t.Fatal("expected a failure")
			}
			if got := ReasonOf(err); got != tc.reason {
				t.Fatalf("reason = %q, want %q (%v)", got, tc.reason, err)
			}
		})
	}

	t.Run("reports an unchecksummed document instead of inventing a verdict", func(t *testing.T) {
		_, result, err := VerifySnapshotChecksum([]byte(`{"schema_version":"1.0","domains":{}}`), "")
		if err != nil {
			t.Fatalf("an older document must be usable: %v", err)
		}
		if result.ChecksumVerified {
			t.Fatal("claimed verification of a document with no checksum")
		}
	})
}

func TestWriteSnapshotFile(t *testing.T) {
	t.Run("publishes atomically with restrictive permissions", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "snapshot.json")
		if err := WriteSnapshotFile(path, []byte("payload")); err != nil {
			t.Fatalf("write failed: %v", err)
		}
		stored, err := os.ReadFile(path)
		if err != nil || string(stored) != "payload" {
			t.Fatalf("stored = %q, err = %v", stored, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Fatalf("mode = %04o, want 0600", perm)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("temporary file left behind: %d entries", len(entries))
		}
	})

	t.Run("replaces an existing file in place", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "snapshot.json")
		if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := WriteSnapshotFile(path, []byte("new")); err != nil {
			t.Fatal(err)
		}
		stored, _ := os.ReadFile(path)
		if string(stored) != "new" {
			t.Fatalf("stored = %q", stored)
		}
	})

	t.Run("reports a missing destination directory", func(t *testing.T) {
		err := WriteSnapshotFile(filepath.Join(t.TempDir(), "absent", "snapshot.json"), []byte("payload"))
		if err == nil {
			t.Fatal("expected a failure")
		}
		if got := ReasonOf(err); got != ReasonFileWrite {
			t.Fatalf("reason = %q, want %q", got, ReasonFileWrite)
		}
	})
}
