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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// snapshotChecksumPrefix is the algorithm marker the gateway stamps onto a
// snapshot document's checksum ("sha256:<hex>").
const snapshotChecksumPrefix = "sha256:"

// snapshotIdentity is the subset of a snapshot document the CLI reads. The
// document itself is passed through untouched — modelling all thirteen
// configuration domains here would make the CLI a second, competing owner of
// a schema whose single owner is the gateway.
type snapshotIdentity struct {
	SchemaVersion string `json:"schema_version"`
	Checksum      string `json:"checksum"`
	Generation    uint64 `json:"generation"`
}

// VerifySnapshotChecksum checks a downloaded snapshot document against its own
// checksum before the caller does anything durable with it, and returns the
// exact bytes that were verified.
//
// How the check works: the gateway computes the checksum over the document's
// canonical encoding with the checksum field itself emptied, and the response
// body IS that canonical encoding with the checksum filled in. Blanking the
// one string value therefore reproduces the checksummed byte sequence exactly,
// with no need for the CLI to model or re-encode the document.
//
// headerChecksum, when the response carried X-Snapshot-Checksum, must agree
// with the document's own field — a body/header disagreement means the
// response was assembled from two different documents.
//
// A document that carries no checksum at all comes from a gateway older than
// the checksummed format: that is reported (ChecksumVerified false), not
// invented. A document that carries one and fails is an error.
func VerifySnapshotChecksum(body []byte, headerChecksum string) ([]byte, *SnapshotFileResult, error) {
	// A truncated download is the failure this whole path exists to catch,
	// and truncation shows up first as a document that does not parse.
	verified := bytes.TrimRight(body, " \t\r\n")
	if len(verified) == 0 {
		return nil, nil, &LifecycleError{
			Reason:  ReasonInvalidJSON,
			Message: "gateway returned an empty snapshot document",
		}
	}
	var identity snapshotIdentity
	if err := json.Unmarshal(verified, &identity); err != nil {
		return nil, nil, &LifecycleError{
			Reason:  ReasonInvalidJSON,
			Message: fmt.Sprintf("snapshot document is not valid JSON (truncated or malformed download): %v", err),
		}
	}

	result := &SnapshotFileResult{
		Bytes:         len(verified),
		Checksum:      identity.Checksum,
		SchemaVersion: identity.SchemaVersion,
		Generation:    identity.Generation,
	}

	if identity.Checksum == "" {
		if headerChecksum != "" {
			return nil, nil, &LifecycleError{
				Reason: ReasonChecksumMismatch,
				Message: fmt.Sprintf("response header reports checksum %s but the document carries none",
					headerChecksum),
			}
		}
		// Older gateway: nothing to verify against, and nothing may be
		// claimed about integrity.
		return verified, result, nil
	}
	if headerChecksum != "" && headerChecksum != identity.Checksum {
		return nil, nil, &LifecycleError{
			Reason: ReasonChecksumMismatch,
			Message: fmt.Sprintf("response header reports checksum %s but the document reports %s",
				headerChecksum, identity.Checksum),
		}
	}

	needle := []byte(`"checksum":"` + identity.Checksum + `"`)
	if n := bytes.Count(verified, needle); n != 1 {
		return nil, nil, &LifecycleError{
			Reason: ReasonChecksumMismatch,
			Message: fmt.Sprintf("cannot verify the snapshot checksum: the document's checksum field is not in its canonical encoding (found %d occurrences of %s)",
				n, identity.Checksum),
		}
	}
	blanked := bytes.Replace(verified, needle, []byte(`"checksum":""`), 1)
	sum := sha256.Sum256(blanked)
	computed := snapshotChecksumPrefix + hex.EncodeToString(sum[:])
	if computed != identity.Checksum {
		return nil, nil, &LifecycleError{
			Reason: ReasonChecksumMismatch,
			Message: fmt.Sprintf("snapshot document is corrupt: it reports checksum %s but its contents hash to %s",
				identity.Checksum, computed),
		}
	}
	result.ChecksumVerified = true
	return verified, result, nil
}

// WriteSnapshotFile stores a verified snapshot document at path through
// WriteFileAtomic, so a failed download cannot destroy the backup a previous
// successful one left behind.
func WriteSnapshotFile(path string, data []byte) error {
	_, err := WriteFileAtomic(path, bytes.NewReader(data))
	return err
}

// WriteFileAtomic stores everything r yields at path without ever leaving a
// partial file where a whole one used to be: it writes a temporary file in
// the destination directory, flushes it to stable storage, and renames it
// into place. It returns the number of bytes stored.
//
// The file is created 0600 — what lands here is a snapshot document carrying
// the full configuration of the gateway, including encrypted secret material,
// or a log archive carrying whatever the gateway wrote to its log.
func WriteFileAtomic(path string, r io.Reader) (int64, error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return 0, &LifecycleError{
			Reason:  ReasonFileWrite,
			Message: fmt.Sprintf("cannot create a temporary file in %s: %v", dir, err),
		}
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}
	if err := tmp.Chmod(0600); err != nil {
		cleanup()
		return 0, &LifecycleError{
			Reason:  ReasonFileWrite,
			Message: fmt.Sprintf("cannot set permissions on %s: %v", tmpName, err),
		}
	}
	n, err := io.Copy(tmp, r)
	if err != nil {
		cleanup()
		return 0, &LifecycleError{
			Reason:  ReasonFileWrite,
			Message: fmt.Sprintf("cannot write %s: %v", tmpName, err),
		}
	}
	// Flush the contents before the rename publishes the name: a rename
	// that reaches the disk ahead of the data would leave an empty file
	// under a name that promises a whole document.
	if err := tmp.Sync(); err != nil {
		cleanup()
		return 0, &LifecycleError{
			Reason:  ReasonFileWrite,
			Message: fmt.Sprintf("cannot flush %s: %v", tmpName, err),
		}
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return 0, &LifecycleError{
			Reason:  ReasonFileWrite,
			Message: fmt.Sprintf("cannot close %s: %v", tmpName, err),
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return 0, &LifecycleError{
			Reason:  ReasonFileWrite,
			Message: fmt.Sprintf("cannot move the file into place at %s: %v", path, err),
		}
	}
	// Persist the directory entry itself, so the published name survives a
	// crash on the same terms as its contents.
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
	return n, nil
}
