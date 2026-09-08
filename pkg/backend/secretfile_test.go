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
	"os"
	"path/filepath"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
)

func TestSecretFileRules(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, mode os.FileMode) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("opensesame\n"), mode); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("owner-only file reads and trims", func(t *testing.T) {
		secret, err := ReadSecretFile("password file", write("good", 0o600))
		if err != nil || string(secret) != "opensesame" {
			t.Fatalf("secret=%q err=%v", secret, err)
		}
	})
	t.Run("relative path is an invalid invocation", func(t *testing.T) {
		_, err := ReadSecretFile("password file", "relative.pass")
		requireCLIError(t, err, exitcode.InvalidArgument, "")
	})
	t.Run("group-readable file is refused", func(t *testing.T) {
		_, err := ReadSecretFile("password file", write("groupy", 0o640))
		requireCLIError(t, err, exitcode.Precondition, "secret-file-refused")
	})
	t.Run("symlink is refused", func(t *testing.T) {
		target := write("target", 0o600)
		link := filepath.Join(dir, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		_, err := ReadSecretFile("password file", link)
		requireCLIError(t, err, exitcode.Precondition, "secret-file-refused")
	})
	t.Run("missing file is a precondition", func(t *testing.T) {
		_, err := ReadSecretFile("password file", filepath.Join(dir, "absent"))
		requireCLIError(t, err, exitcode.Precondition, "file-read-failed")
	})
	t.Run("empty file is refused", func(t *testing.T) {
		p := filepath.Join(dir, "empty")
		if err := os.WriteFile(p, []byte("  \n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := ReadSecretFile("password file", p)
		requireCLIError(t, err, exitcode.Precondition, "secret-file-refused")
	})
}
