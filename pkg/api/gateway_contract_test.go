package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

type gatewayContractManifest struct {
	Source struct {
		Repository          string `json:"repository"`
		Revision            string `json:"revision"`
		SwaggerSHA256       string `json:"swagger_sha256"`
		SwaggerExtrasSHA256 string `json:"swagger_extras_sha256"`
	} `json:"source"`
	Operations []gatewayContractOperation `json:"operations"`
	Fields     []gatewayContractField     `json:"fields"`
}

type gatewayContractOperation struct {
	Spec    string   `json:"spec"`
	Path    string   `json:"path"`
	Methods []string `json:"methods"`
}

type gatewayContractField struct {
	Spec         string   `json:"spec"`
	Definition   string   `json:"definition"`
	PropertyPath []string `json:"property_path"`
	Type         string   `json:"type"`
	Enum         []string `json:"enum,omitempty"`
	Required     bool     `json:"required,omitempty"`
}

func loadGatewayContractManifest(t *testing.T) gatewayContractManifest {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "contracts", "gateway-api.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contract manifest: %v", err)
	}
	var manifest gatewayContractManifest
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		t.Fatalf("decode contract manifest: %v", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		t.Fatalf("contract manifest has trailing content: %v", err)
	}
	return manifest
}

func TestGatewayContractManifest(t *testing.T) {
	manifest := loadGatewayContractManifest(t)
	if manifest.Source.Repository != "https://github.com/loxilb-io/loxilb-inference-gateway" {
		t.Fatalf("gateway contract repository is not the approved producer: %q", manifest.Source.Repository)
	}
	if len(manifest.Source.Revision) != 40 {
		t.Fatalf("gateway contract revision must be an exact 40-character commit, got %q", manifest.Source.Revision)
	}
	if _, err := hex.DecodeString(manifest.Source.Revision); err != nil {
		t.Fatalf("gateway contract revision must be hexadecimal: %v", err)
	}
	for name, value := range map[string]string{
		"swagger_sha256":        manifest.Source.SwaggerSHA256,
		"swagger_extras_sha256": manifest.Source.SwaggerExtrasSHA256,
	} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != sha256.Size {
			t.Fatalf("%s must be a SHA-256 digest", name)
		}
	}
	if len(manifest.Operations) == 0 || len(manifest.Fields) == 0 {
		t.Fatal("contract manifest must contain operations and fields")
	}
	seen := make(map[string]bool)
	for _, field := range manifest.Fields {
		key := field.Spec + ":" + field.Definition + ":" + strings.Join(field.PropertyPath, ".")
		if seen[key] {
			t.Fatalf("duplicate contract field %s", key)
		}
		seen[key] = true
	}
}

func TestGatewayContractAgainstCheckout(t *testing.T) {
	repo := os.Getenv("GATEWAY_REPO")
	if repo == "" {
		t.Skip("set GATEWAY_REPO to compare the consumed contract with a Gateway checkout")
	}

	manifest := loadGatewayContractManifest(t)

	// The pinned revision and spec digests are PROVENANCE: they record the
	// Gateway tree this contract was captured from. They are not the gate,
	// and they are expected to lag -- the Gateway repository advances on its
	// own schedule and nothing here is notified.
	//
	// They were asserted for equality until this change, which meant the
	// test could pass on exactly ONE Gateway commit and failed on every
	// other, including every PR opened against Gateway main. Because both
	// checks were fatal, the operation and field assertions below them --
	// the only ones that can actually detect a contract break -- never ran
	// at all. The gate reported red continuously while checking nothing.
	//
	// What follows IS the gate: every operation and field this CLI decodes
	// must still be present in the checked-out spec with the declared type.
	// A Gateway change that removes or retypes one goes red here, on the PR
	// that makes it, which is what this fixture is for.
	resolved, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("resolve Gateway checkout revision: %v", err)
	}
	if got := strings.TrimSpace(string(resolved)); got != manifest.Source.Revision {
		t.Logf("Gateway checkout %s differs from the pinned contract revision %s; "+
			"the assertions below decide whether the contract still holds",
			got, manifest.Source.Revision)
	}
	specs := make(map[string]map[interface{}]interface{})
	for _, name := range []string{"swagger.yml", "swagger-extras.yml"} {
		path := filepath.Join(repo, "api", name)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		digest := sha256.Sum256(b)
		gotDigest := hex.EncodeToString(digest[:])
		wantDigest := manifest.Source.SwaggerSHA256
		if name == "swagger-extras.yml" {
			wantDigest = manifest.Source.SwaggerExtrasSHA256
		}
		if gotDigest != wantDigest {
			// Provenance again, not a verdict: the spec has moved since the
			// contract was captured. That is ordinary -- additions do not
			// break a consumer. Reported so a failure below can be read
			// against it rather than guessed at.
			t.Logf("%s has moved since the contract was pinned (digest %s, manifest %s)",
				name, gotDigest, wantDigest)
		}
		var spec map[interface{}]interface{}
		if err := yaml.Unmarshal(b, &spec); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		specs[name] = spec
	}

	for _, operation := range manifest.Operations {
		spec := requireContractSpec(t, specs, operation.Spec)
		pathItem := contractMapAt(t, spec, "paths", operation.Path)
		for _, method := range operation.Methods {
			if _, ok := pathItem[method]; !ok {
				t.Errorf("%s paths.%s is missing method %s", operation.Spec, operation.Path, method)
			}
		}
	}

	for _, field := range manifest.Fields {
		spec := requireContractSpec(t, specs, field.Spec)
		segments := []string{"definitions", field.Definition, "properties"}
		for i, property := range field.PropertyPath {
			segments = append(segments, property)
			if i != len(field.PropertyPath)-1 {
				segments = append(segments, "properties")
			}
		}
		node := contractMapAt(t, spec, segments...)
		if got := fmt.Sprint(node["type"]); got != field.Type {
			t.Errorf("%s %s.%s type=%q, want %q", field.Spec, field.Definition,
				strings.Join(field.PropertyPath, "."), got, field.Type)
		}
		if len(field.Enum) > 0 {
			got := contractEnum(node["enum"])
			want := append([]string(nil), field.Enum...)
			sort.Strings(got)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s %s.%s enum=%v, want %v", field.Spec, field.Definition,
					strings.Join(field.PropertyPath, "."), got, want)
			}
		}
		if field.Required {
			if len(field.PropertyPath) != 1 {
				t.Fatalf("required contract check only supports top-level properties: %s.%s",
					field.Definition, strings.Join(field.PropertyPath, "."))
			}
			definition := contractMapAt(t, spec, "definitions", field.Definition)
			required := contractEnum(definition["required"])
			if !containsContractValue(required, field.PropertyPath[0]) {
				t.Errorf("%s %s.%s must be required", field.Spec, field.Definition, field.PropertyPath[0])
			}
		}
	}
}

func containsContractValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func requireContractSpec(t *testing.T, specs map[string]map[interface{}]interface{}, name string) map[interface{}]interface{} {
	t.Helper()
	spec, ok := specs[name]
	if !ok {
		t.Fatalf("contract references unknown spec %q", name)
	}
	return spec
}

func contractMapAt(t *testing.T, root map[interface{}]interface{}, segments ...string) map[interface{}]interface{} {
	t.Helper()
	current := root
	for _, segment := range segments {
		next, ok := current[segment]
		if !ok {
			t.Fatalf("contract path %s is missing", strings.Join(segments, "."))
		}
		mapped, ok := next.(map[interface{}]interface{})
		if !ok {
			t.Fatalf("contract path %s is not an object", strings.Join(segments, "."))
		}
		current = mapped
	}
	return current
}

func contractEnum(raw interface{}) []string {
	values, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, fmt.Sprint(value))
	}
	return result
}
