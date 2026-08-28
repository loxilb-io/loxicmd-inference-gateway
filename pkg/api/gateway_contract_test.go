package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"
)

type gatewayContractManifest struct {
	Source struct {
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
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatalf("decode contract manifest: %v", err)
	}
	return manifest
}

func TestGatewayContractManifest(t *testing.T) {
	manifest := loadGatewayContractManifest(t)
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
	specs := make(map[string]map[interface{}]interface{})
	for _, name := range []string{"swagger.yml", "swagger-extras.yml"} {
		path := filepath.Join(repo, "api", name)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
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
