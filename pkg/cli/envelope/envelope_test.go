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

package envelope

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// loadSchema reads contracts/command-result.schema.json, the frozen contract
// this package implements.
func loadSchema(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "contracts", "command-result.schema.json"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(b, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	return schema
}

// TestEnvelopeKeysMatchSchema proves the Go struct and the schema describe
// the same document: every schema-required key is marshaled, and no marshaled
// key is absent from the schema (the schema forbids additionalProperties, so
// an extra Go field would make every emitted document invalid).
func TestEnvelopeKeysMatchSchema(t *testing.T) {
	schema := loadSchema(t)

	var required []string
	for _, k := range schema["required"].([]any) {
		required = append(required, k.(string))
	}
	properties := schema["properties"].(map[string]any)

	var buf bytes.Buffer
	if err := New("get.snapshot").Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("decode emitted envelope: %v", err)
	}

	var emitted []string
	for k := range doc {
		emitted = append(emitted, k)
		if _, ok := properties[k]; !ok {
			t.Errorf("emitted key %q is not in the schema, which forbids additional properties", k)
		}
		if doc[k] == nil {
			t.Errorf("key %q marshaled as null; the contract requires empty values, never null", k)
		}
	}
	sort.Strings(required)
	sort.Strings(emitted)
	if !reflect.DeepEqual(required, emitted) {
		t.Errorf("schema-required keys %v != emitted keys %v", required, emitted)
	}
}

// TestCodesMatchSchemaEnum proves the Go Code constants and the schema's
// enum are the same set, so neither side can grow a verdict the other
// rejects.
func TestCodesMatchSchemaEnum(t *testing.T) {
	schema := loadSchema(t)
	enumAny := schema["properties"].(map[string]any)["code"].(map[string]any)["enum"].([]any)
	var schemaCodes []string
	for _, v := range enumAny {
		schemaCodes = append(schemaCodes, v.(string))
	}
	goCodes := []string{
		string(OK), string(InvalidArgument), string(Auth), string(Precondition),
		string(Unavailable), string(ContractMismatch), string(Failed), string(Partial),
	}
	sort.Strings(schemaCodes)
	sort.Strings(goCodes)
	if !reflect.DeepEqual(schemaCodes, goCodes) {
		t.Errorf("schema enum %v != Go codes %v", schemaCodes, goCodes)
	}
}

// TestExitCodeMapping pins the frozen code-to-exit correspondence from
// contracts/exit-codes.md, including the reserved legacy value for a code
// this build does not know.
func TestExitCodeMapping(t *testing.T) {
	want := map[Code]int{
		OK: 0, InvalidArgument: 2, Auth: 3, Precondition: 4,
		Unavailable: 5, ContractMismatch: 6, Failed: 7, Partial: 8,
	}
	for code, exit := range want {
		if got := code.ExitCode(); got != exit {
			t.Errorf("%s: exit %d, want %d", code, got, exit)
		}
	}
	if got := Code("BOGUS").ExitCode(); got != 1 {
		t.Errorf("unknown code: exit %d, want the reserved legacy value 1", got)
	}
}

// TestFailDerivesSuccess proves success can never disagree with the code.
func TestFailDerivesSuccess(t *testing.T) {
	r := New("appliance.status").Fail(Unavailable, "backend unreachable")
	if r.Success {
		t.Error("a failed envelope must not report success")
	}
	if r.Code != Unavailable || r.Message != "backend unreachable" {
		t.Errorf("failure not recorded: code=%s message=%q", r.Code, r.Message)
	}
}

// TestWriteRestoresInvariants proves a caller nil-ing Data or Warnings still
// yields {} and [] on the wire, never null.
func TestWriteRestoresInvariants(t *testing.T) {
	r := New("version")
	r.Data = nil
	r.Warnings = nil
	var buf bytes.Buffer
	if err := r.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := doc["data"].(map[string]any); !ok {
		t.Errorf("data marshaled as %T, want an object", doc["data"])
	}
	if _, ok := doc["warnings"].([]any); !ok {
		t.Errorf("warnings marshaled as %T, want an array", doc["warnings"])
	}
}

// TestWriteEmitsExactlyOneDocument proves stdout discipline: one JSON
// document, one trailing newline, nothing else.
func TestWriteEmitsExactlyOneDocument(t *testing.T) {
	var buf bytes.Buffer
	if err := New("version").Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	var first any
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("first document: %v", err)
	}
	var second any
	if err := dec.Decode(&second); err == nil {
		t.Error("a second JSON document followed the first; the contract is exactly one")
	}
	if buf.Bytes()[buf.Len()-1] != '\n' {
		t.Error("document must end with a newline")
	}
}
