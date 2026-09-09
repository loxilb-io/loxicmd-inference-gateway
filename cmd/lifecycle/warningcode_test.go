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
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// schemaWarningCodePattern reads the rule from the contract itself rather than
// restating it. A test carrying its own copy of the pattern proves the code
// agrees with the test, which is not the property anyone wants.
func schemaWarningCodePattern(t *testing.T) *regexp.Regexp {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "contracts", "command-result.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema struct {
		Properties struct {
			Warnings struct {
				Items struct {
					Properties struct {
						Code struct {
							Pattern string `json:"pattern"`
						} `json:"code"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"warnings"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(b, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	pattern := schema.Properties.Warnings.Items.Properties.Code.Pattern
	if pattern == "" {
		t.Fatal("the schema no longer constrains warnings[].code; this guard would pass vacuously")
	}
	return regexp.MustCompile(pattern)
}

// TestEveryNoteCodeMatchesTheSchema walks the package for api.Note literals and
// holds each Code against the schema's own pattern.
//
// It is structural rather than a list of the notes that exist today, because a
// list cannot fail for a note nobody added it to -- which is exactly how the two
// original codes shipped in the lowercase-hyphen shape of the lifecycle REASON
// codes and produced an envelope the published schema rejects. That envelope
// accompanied a SUCCESSFUL command, so a conforming consumer would reject a
// result that reported no problem.
func TestEveryNoteCodeMatchesTheSchema(t *testing.T) {
	valid := schemaWarningCodePattern(t)
	fset := token.NewFileSet()
	found := 0

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", path, perr)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isNoteType(lit.Type) {
				return true
			}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "Code" {
					continue
				}
				str, ok := kv.Value.(*ast.BasicLit)
				if !ok || str.Kind != token.STRING {
					continue
				}
				code, uerr := strconv.Unquote(str.Value)
				if uerr != nil {
					continue
				}
				found++
				if !valid.MatchString(code) {
					t.Errorf("%s: note code %q does not match the envelope schema's %s. "+
						"Warning codes are SCREAMING_SNAKE; the lowercase-hyphen shape belongs to "+
						"the lifecycle reason codes, which travel in data.componentCode",
						fset.Position(str.Pos()), code, valid.String())
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if found == 0 {
		t.Fatal("no api.Note codes found; the guard is not looking at anything")
	}
}

func isNoteType(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "api" && sel.Sel.Name == "Note"
}

// TestWarningBearingEnvelopeSatisfiesTheSchemaPattern closes the loop at the
// wire: the structural guard above proves the declared codes are well-shaped,
// this proves what a real command actually emits is. A legacy-contract gateway
// is the cheapest way to make a command emit a warning at all.
func TestWarningBearingEnvelopeSatisfiesTheSchemaPattern(t *testing.T) {
	valid := schemaWarningCodePattern(t)
	gw := newFakeGateway(t, jsonResponse(http.StatusOK, `{"result":"ok"}`))
	out := &strings.Builder{}

	if err := Persist(gw.options(), out, Options{JSON: true}, "create.persist"); err != nil {
		t.Fatalf("persist: %v", err)
	}
	var doc struct {
		Success  bool `json:"success"`
		Warnings []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out.String()), &doc); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out.String())
	}
	if !doc.Success {
		t.Fatal("the fixture was meant to succeed; a failure envelope may carry no warning")
	}
	if len(doc.Warnings) == 0 {
		t.Fatal("a legacy-contract gateway must produce a warning, or this proves nothing")
	}
	for _, w := range doc.Warnings {
		if !valid.MatchString(w.Code) {
			t.Errorf("emitted warning code %q does not match the schema's %s", w.Code, valid.String())
		}
	}
}
