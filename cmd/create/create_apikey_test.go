package create

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

const exampleImportedAPIKey = "example-imported-key-material"

func TestReadImportedAPIKey(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		key, imported, err := readImportedAPIKey(&CreateAPIKeyOptions{}, strings.NewReader("unused"))
		if err != nil || imported || key != "" {
			t.Fatalf("got key_length=%d imported=%v err=%v", len(key), imported, err)
		}
	})

	t.Run("stdin", func(t *testing.T) {
		key, imported, err := readImportedAPIKey(
			&CreateAPIKeyOptions{APIKeyStdin: true}, strings.NewReader(exampleImportedAPIKey+"\n"))
		if err != nil || !imported || key != exampleImportedAPIKey {
			t.Fatalf("got key_length=%d imported=%v err=%v", len(key), imported, err)
		}
	})

	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "api-key")
		if err := os.WriteFile(path, []byte("  "+exampleImportedAPIKey+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		key, imported, err := readImportedAPIKey(&CreateAPIKeyOptions{APIKeyFile: path}, nil)
		if err != nil || !imported || key != exampleImportedAPIKey {
			t.Fatalf("got key_length=%d imported=%v err=%v", len(key), imported, err)
		}
	})

	for name, input := range map[string]struct {
		o CreateAPIKeyOptions
		s string
	}{
		"both inputs": {CreateAPIKeyOptions{APIKeyFile: "unused", APIKeyStdin: true}, exampleImportedAPIKey},
		"empty":       {CreateAPIKeyOptions{APIKeyStdin: true}, " \n"},
		"too short":   {CreateAPIKeyOptions{APIKeyStdin: true}, "short"},
		"too long":    {CreateAPIKeyOptions{APIKeyStdin: true}, strings.Repeat("x", maxImportedAPIKeyLength+1)},
		"space":       {CreateAPIKeyOptions{APIKeyStdin: true}, "example imported key material"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := readImportedAPIKey(&input.o, strings.NewReader(input.s)); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestImportedAPIKeyRequestSerialization(t *testing.T) {
	b, err := json.Marshal(api.AIApiKeyCreateRequest{TenantID: "tenant-a", APIKey: exampleImportedAPIKey})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte(`"api_key":"`+exampleImportedAPIKey+`"`)) {
		t.Fatal("request does not carry imported key")
	}
}

func TestPrintCreateAPIKeyResult(t *testing.T) {
	t.Run("imported text never echoes key", func(t *testing.T) {
		var out bytes.Buffer
		result := api.AIApiKeyCreateResponse{KeyID: "example-key-id"}
		if err := printCreateAPIKeyResult(&out, result, true, ""); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if strings.Contains(got, "raw_key") || strings.Contains(got, exampleImportedAPIKey) ||
			strings.Contains(got, "Store the") {
			t.Fatalf("import output exposed secret-only messaging: %q", got)
		}
		if !strings.Contains(got, "API key imported") || !strings.Contains(got, "example-key-id") {
			t.Fatalf("unexpected output: %q", got)
		}
	})

	t.Run("imported json omits raw key", func(t *testing.T) {
		var out bytes.Buffer
		result := api.AIApiKeyCreateResponse{KeyID: "example-key-id", RawKey: exampleImportedAPIKey}
		if err := printCreateAPIKeyResult(&out, result, true, "json"); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "raw_key") || strings.Contains(out.String(), exampleImportedAPIKey) {
			t.Fatalf("JSON output exposed imported key material: %q", out.String())
		}
	})

	t.Run("generated key is shown once", func(t *testing.T) {
		var out bytes.Buffer
		result := api.AIApiKeyCreateResponse{KeyID: "example-key-id", RawKey: "example-generated-key-material"}
		if err := printCreateAPIKeyResult(&out, result, false, ""); err != nil {
			t.Fatal(err)
		}
		if strings.Count(out.String(), result.RawKey) != 1 || !strings.Contains(out.String(), "Store the raw_key") {
			t.Fatalf("unexpected generated-key output: %q", out.String())
		}
	})
}
