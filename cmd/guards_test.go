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

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestExitTaxonomyGuards structurally enforces the migration that moved every
// command onto the exit-code taxonomy, so the patterns it removed cannot come
// back:
//
//  1. no os.Exit outside cmd/root.go — the single exit point is the only
//     place the process status is decided, so the exit code and the reported
//     failure can never disagree;
//  2. no Run: handler on a cobra.Command outside cmd/root.go — a Run: handler
//     cannot return an error, so its failures exit 0 (and cobra silently
//     prefers RunE when both are set, leaving Run: as dead code);
//  3. no "Error:"-prefixed print to stderr outside cmd/root.go — the single
//     exit point owns that line, and a command printing it too shows every
//     failure twice.
//
// cmd/root.go hosts the exit point itself plus the version and completion
// commands, which cannot fail; it is the one file exempt from all three.
func TestExitTaxonomyGuards(t *testing.T) {
	var violations []string
	fset := token.NewFileSet()

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || path == "root.go" {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", path, perr)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				if isSelector(node.Fun, "os", "Exit") {
					violations = append(violations, fmt.Sprintf(
						"%s: os.Exit call (only the single exit point in cmd/root.go decides the process status)",
						fset.Position(node.Pos())))
				}
				if isStderrErrorPrint(node) {
					violations = append(violations, fmt.Sprintf(
						"%s: prints an \"Error:\" line to stderr (the single exit point owns that line; return the error instead)",
						fset.Position(node.Pos())))
				}
			case *ast.KeyValueExpr:
				if key, ok := node.Key.(*ast.Ident); ok && key.Name == "Run" {
					if _, isFunc := node.Value.(*ast.FuncLit); isFunc {
						violations = append(violations, fmt.Sprintf(
							"%s: Run: handler (use RunE: and return a classified error — Run: cannot fail, so it exits 0)",
							fset.Position(node.Pos())))
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Error(v)
	}
}

// isSelector reports whether expr is the selector pkg.name.
func isSelector(expr ast.Expr, pkg, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == pkg
}

// isStderrErrorPrint reports whether the call writes an "Error:"-prefixed
// string literal via fmt.Fprint/Fprintf/Fprintln to a stderr-shaped writer
// (os.Stderr, or a writer conventionally named errOut / stderr).
func isStderrErrorPrint(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !strings.HasPrefix(sel.Sel.Name, "Fprint") {
		return false
	}
	if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "fmt" {
		return false
	}
	if len(call.Args) < 2 {
		return false
	}
	switch target := call.Args[0].(type) {
	case *ast.Ident:
		if target.Name != "errOut" && target.Name != "stderr" {
			return false
		}
	case *ast.SelectorExpr:
		if !isSelector(target, "os", "Stderr") {
			return false
		}
	case *ast.CallExpr:
		sel, ok := target.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "ErrOrStderr" {
			return false
		}
	default:
		return false
	}
	for _, arg := range call.Args[1:] {
		if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING &&
			strings.HasPrefix(lit.Value, `"Error:`) {
			return true
		}
	}
	return false
}
