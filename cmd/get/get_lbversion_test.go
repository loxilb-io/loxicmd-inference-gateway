package get

import (
	"encoding/json"
	"errors"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
	"io"
	"net/http"
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

type brokenVersionBody struct{}

func (brokenVersionBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (brokenVersionBody) Close() error             { return nil }

func TestLBVersionResponseFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		body io.ReadCloser
		code exitcode.Code
	}{
		{"malformed", io.NopCloser(strings.NewReader(`{"version":`)), exitcode.ContractMismatch},
		{"wrong-type", io.NopCloser(strings.NewReader(`{"version":42}`)), exitcode.ContractMismatch},
		{"read-error", brokenVersionBody{}, exitcode.Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := PrintGetVersionResult(&http.Response{Body: tc.body}, api.RESTOptions{PrintOption: "json"})
			var classified *exitcode.CLIError
			if !errors.As(err, &classified) || classified.Code != tc.code {
				t.Fatalf("got %v, want code %d", err, tc.code)
			}
		})
	}
}
