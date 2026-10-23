package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
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
	Model  string `json:"model"`
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

// gatewayContractField is one property this CLI reads or writes. It lives
// either in a named definition (Definition) or in the inline body schema
// of one operation (Body), as the swagger-extras PATCH bodies do; exactly
// one of the two is set.
type gatewayContractField struct {
	Spec         string               `json:"spec"`
	Definition   string               `json:"definition,omitempty"`
	Body         *gatewayContractBody `json:"body,omitempty"`
	PropertyPath []string             `json:"property_path"`
	Type         string               `json:"type"`
	Enum         []string             `json:"enum,omitempty"`
	Required     bool                 `json:"required,omitempty"`
}

// gatewayContractBody names the operation whose body parameter's inline
// schema holds the field.
type gatewayContractBody struct {
	Path   string `json:"path"`
	Method string `json:"method"`
}

// where names the field's container for messages and the duplicate check.
func (f gatewayContractField) where() string {
	if f.Body != nil {
		return f.Body.Method + " " + f.Body.Path + " body"
	}
	return f.Definition
}

func loadGatewayContractManifest(t *testing.T) gatewayContractManifest {
	t.Helper()
	model := os.Getenv("PRODUCT_MODEL")
	if model == "" {
		model = "general"
	}
	if model != "general" && model != "kcmvp" {
		t.Fatalf("unknown Product model %q", model)
	}
	path := filepath.Join("..", "..", "testdata", "contracts", "gateway-api.json")
	if model == "kcmvp" {
		path = filepath.Join("..", "..", "testdata", "contracts", "models", model, "gateway-api.json")
	}
	if override := os.Getenv("GATEWAY_CONTRACT_MANIFEST"); override != "" {
		path = override
	}
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
	if manifest.Model != "" && manifest.Model != model {
		t.Fatalf("contract model %q does not match selected model %q", manifest.Model, model)
	}
	return manifest
}

func TestGatewayContractExternalProfile(t *testing.T) {
	manifest := loadGatewayContractManifest(t)
	manifest.Source.Repository = "https://example.com/gateway"
	profile, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "gateway-api.json")
	if err := os.WriteFile(path, profile, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEWAY_CONTRACT_MANIFEST", path)
	got := loadGatewayContractManifest(t)
	if !reflect.DeepEqual(got, manifest) {
		t.Fatal("selected profile did not preserve its producer and consumed contract")
	}
	TestGatewayContractManifest(t)
}

func TestGatewayContractManifest(t *testing.T) {
	manifest := loadGatewayContractManifest(t)
	producer, err := url.Parse(manifest.Source.Repository)
	if err != nil || producer.Scheme != "https" || producer.Host == "" || producer.User != nil || producer.Fragment != "" {
		t.Fatalf("gateway contract repository must be a credential-free HTTPS URL: %q", manifest.Source.Repository)
	}
	expected := map[string]string{"general": "https://github.com/loxilb-io/loxilb-inference-gateway", "kcmvp": "https://github.com/netlox-io/loxilb-igw"}
	if manifest.Model != "general" && manifest.Model != "kcmvp" && os.Getenv("GATEWAY_CONTRACT_MANIFEST") == "" {
		t.Fatal("explicit valid model required")
	}
	if os.Getenv("GATEWAY_CONTRACT_MANIFEST") == "" && manifest.Source.Repository != expected[manifest.Model] {

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
		if (field.Definition == "") == (field.Body == nil) {
			t.Fatalf("contract field %s must name exactly one of definition and body", strings.Join(field.PropertyPath, "."))
		}
		key := field.Spec + ":" + field.where() + ":" + strings.Join(field.PropertyPath, ".")
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

	// GATEWAY_CONTRACT_EXACT=1 requires equality for a selected build profile.
	// In the default compatibility mode, these digests are PROVENANCE: they record the
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
		if os.Getenv("GATEWAY_CONTRACT_EXACT") == "1" {
			t.Fatalf("exact contract revision mismatch: got %s, want %s", got, manifest.Source.Revision)
		}
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
			if os.Getenv("GATEWAY_CONTRACT_EXACT") == "1" {
				t.Fatalf("exact %s digest mismatch: got %s, want %s", name, gotDigest, wantDigest)
			}
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
		var container map[interface{}]interface{}
		if field.Body != nil {
			container = contractBodySchema(t, spec, field.Body)
		} else {
			container = contractMapAt(t, spec, "definitions", field.Definition)
		}
		segments := []string{"properties"}
		for i, property := range field.PropertyPath {
			segments = append(segments, property)
			if i != len(field.PropertyPath)-1 {
				segments = append(segments, "properties")
			}
		}
		node := contractMapAt(t, container, segments...)
		if got := fmt.Sprint(node["type"]); got != field.Type {
			t.Errorf("%s %s.%s type=%q, want %q", field.Spec, field.where(),
				strings.Join(field.PropertyPath, "."), got, field.Type)
		}
		if len(field.Enum) > 0 {
			got := contractEnum(node["enum"])
			want := append([]string(nil), field.Enum...)
			sort.Strings(got)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s %s.%s enum=%v, want %v", field.Spec, field.where(),
					strings.Join(field.PropertyPath, "."), got, want)
			}
		}
		if field.Required {
			if len(field.PropertyPath) != 1 {
				t.Fatalf("required contract check only supports top-level properties: %s.%s",
					field.where(), strings.Join(field.PropertyPath, "."))
			}
			definition := container
			required := contractEnum(definition["required"])
			if !containsContractValue(required, field.PropertyPath[0]) {
				t.Errorf("%s %s.%s must be required", field.Spec, field.Definition, field.PropertyPath[0])
			}
		}
	}
}

// contractBodySchema returns the inline schema of the operation's body
// parameter (in: body), where the swagger-extras PATCH bodies keep their
// properties instead of a named definition.
func contractBodySchema(t *testing.T, spec map[interface{}]interface{}, body *gatewayContractBody) map[interface{}]interface{} {
	t.Helper()
	operation := contractMapAt(t, spec, "paths", body.Path, body.Method)
	parameters, ok := operation["parameters"].([]interface{})
	if !ok {
		t.Fatalf("%s %s has no parameters", body.Method, body.Path)
	}
	for _, raw := range parameters {
		parameter, ok := raw.(map[interface{}]interface{})
		if !ok || parameter["in"] != "body" {
			continue
		}
		schema, ok := parameter["schema"].(map[interface{}]interface{})
		if !ok {
			t.Fatalf("%s %s body parameter has no schema object", body.Method, body.Path)
		}
		return schema
	}
	t.Fatalf("%s %s has no body parameter", body.Method, body.Path)
	return nil
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
