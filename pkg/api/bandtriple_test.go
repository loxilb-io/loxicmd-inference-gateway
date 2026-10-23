package api

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v2"
)

// The wire form (critical:standard:sheddable) reads into the three values
// and writes back byte for byte, through the one Parse/String pair.
func TestBandTripleRoundTrip(t *testing.T) {
	for _, wire := range []string{"8:3:1", "0:0:0", "100:0:0", "1:62:4294967295"} {
		parsed, err := ParseBandTriple(wire)
		if err != nil {
			t.Fatalf("parse %q: %v", wire, err)
		}
		if got := parsed.String(); got != wire {
			t.Fatalf("%q round-trips as %q", wire, got)
		}
	}
	if got, _ := ParseBandTriple("8:3:1"); got != (BandTriple{Critical: 8, Standard: 3, Sheddable: 1}) {
		t.Fatalf("8:3:1 parsed as %+v", got)
	}
}

// Shape only: not three whole numbers separated by ':' is refused; what
// the numbers may be is the gateway's rule.
func TestBandTripleShape(t *testing.T) {
	for _, bad := range []string{"", "8", "8:3", "8:3:1:0", "8:3:x", "-1:0:0", "8::1", " 8:3:1", "8:3:1 ", "1.5:0:0", "4294967296:0:0"} {
		if _, err := ParseBandTriple(bad); err == nil {
			t.Errorf("%q parsed, want a shape error", bad)
		}
	}
}

// JSON and YAML carry the wire string, not an object or an array.
func TestBandTripleWireForms(t *testing.T) {
	type doc struct {
		Weights *BandTriple `json:"fc_band_weights,omitempty" yaml:"fc_band_weights,omitempty"`
	}
	in := doc{Weights: &BandTriple{8, 3, 1}}

	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"fc_band_weights":"8:3:1"}` {
		t.Fatalf("json %s", b)
	}
	var backJSON doc
	if err := json.Unmarshal(b, &backJSON); err != nil || *backJSON.Weights != *in.Weights {
		t.Fatalf("json back %+v, %v", backJSON.Weights, err)
	}
	if err := json.Unmarshal([]byte(`{"fc_band_weights":"8:3"}`), &backJSON); err == nil {
		t.Fatal("a malformed triple decoded from json")
	}
	if b, _ := json.Marshal(doc{}); string(b) != `{}` {
		t.Fatalf("nil triple written: %s", b)
	}

	y, err := yaml.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	// yaml.v2 quotes the value because 8:3:1 reads as a YAML 1.1
	// sexagesimal number otherwise; the quoted scalar is still the wire form.
	if string(y) != "fc_band_weights: \"8:3:1\"\n" {
		t.Fatalf("yaml %q", y)
	}
	var backYAML doc
	if err := yaml.Unmarshal(y, &backYAML); err != nil || *backYAML.Weights != *in.Weights {
		t.Fatalf("yaml back %+v, %v", backYAML.Weights, err)
	}
}
