package get

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

func TestLBProductDisplay(t *testing.T) {
	if got := lbProductDisplay(""); got != "loxilb" {
		t.Fatalf("legacy product display = %q", got)
	}
	if got := lbProductDisplay("loxilb-inference-gateway"); got != "loxilb-inference-gateway" {
		t.Fatalf("gateway product display = %q", got)
	}
}

func TestLBVersionJSONCompatibility(t *testing.T) {
	legacy, err := json.Marshal(api.LBVersionGet{Version: "v1", BuildInfo: "build"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(legacy), "product") {
		t.Fatalf("legacy JSON injected a product: %s", legacy)
	}
	gateway, err := json.Marshal(api.LBVersionGet{Version: "v1", BuildInfo: "build", Product: "loxilb-inference-gateway"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gateway), `"product":"loxilb-inference-gateway"`) {
		t.Fatalf("gateway JSON omitted product: %s", gateway)
	}
}
