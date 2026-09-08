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
package backend

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
)

// CheckSecretPath enforces the secret-file rules on a path without reading
// it: absolute, a regular file reached without following a symlink, and
// owner-only permissions. Commands use it for secret-bearing files the
// backend itself will open (backup key files); ReadSecretFile adds the read
// for secrets the CLI streams onward.
func CheckSecretPath(what, path string) error {
	if !filepath.IsAbs(path) {
		return exitcode.Invalidf("%s must be an absolute path, got %q", what, path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return &exitcode.CLIError{
			Code:          exitcode.Precondition,
			Message:       fmt.Sprintf("cannot read the %s: %v", what, err),
			ComponentCode: "file-read-failed",
		}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return &exitcode.CLIError{
			Code:          exitcode.Precondition,
			Message:       fmt.Sprintf("the %s %q is a symlink; secret files must be regular files", what, path),
			ComponentCode: "secret-file-refused",
		}
	}
	if !info.Mode().IsRegular() {
		return &exitcode.CLIError{
			Code:          exitcode.Precondition,
			Message:       fmt.Sprintf("the %s %q is not a regular file", what, path),
			ComponentCode: "secret-file-refused",
		}
	}
	if info.Mode().Perm()&0o077 != 0 {
		return &exitcode.CLIError{
			Code:          exitcode.Precondition,
			Message:       fmt.Sprintf("the %s %q is group- or world-accessible (%04o); make it owner-only (0600) first", what, path, info.Mode().Perm()),
			ComponentCode: "secret-file-refused",
		}
	}
	return nil
}

// ReadSecretFile validates a secret file with CheckSecretPath and returns
// its content with surrounding whitespace trimmed. The value never travels
// through argv or the environment; callers stream it to the backend's
// stdin.
func ReadSecretFile(what, path string) ([]byte, error) {
	if err := CheckSecretPath(what, path); err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, &exitcode.CLIError{
			Code:          exitcode.Precondition,
			Message:       fmt.Sprintf("cannot read the %s: %v", what, err),
			ComponentCode: "file-read-failed",
		}
	}
	secret := bytes.TrimSpace(content)
	if len(secret) == 0 {
		return nil, &exitcode.CLIError{
			Code:          exitcode.Precondition,
			Message:       fmt.Sprintf("the %s %q is empty", what, path),
			ComponentCode: "secret-file-refused",
		}
	}
	return secret, nil
}
