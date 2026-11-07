package create

import (
	"strings"
	"testing"
)

func TestEppFlagsOnTheWire(t *testing.T) {
	t.Run("omitted stays off the wire", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.52", Select: "rr"})
		assertAbsent(t, m, "eppEndpoint", "eppFailureMode", "eppTimeoutMs", "eppPlaintext")
	})
	t.Run("given is sent under the swagger names", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.52", Select: "rr",
			EppEndpoint: "epp.example.svc:9002", EppFailureMode: "FailOpen", EppTimeoutMs: 500, EppPlaintext: true})
		assertKey(t, m, "eppEndpoint", "epp.example.svc:9002")
		assertKey(t, m, "eppFailureMode", "FailOpen")
		assertKey(t, m, "eppTimeoutMs", 500)
		assertKey(t, m, "eppPlaintext", true)
	})
	t.Run("P/D FailOpen keeps both contracts on the wire", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.52", Select: "rr", Mode: "fullproxy",
			EppEndpoint: "epp.example.svc:9002", EppFailureMode: "FailOpen", PdDisaggMode: true})
		assertKey(t, m, "eppEndpoint", "epp.example.svc:9002")
		assertKey(t, m, "eppFailureMode", "FailOpen")
		assertKey(t, m, "pd_disagg_mode", true)
	})
}

func TestValidateEppOptions(t *testing.T) {
	const ep = "epp.example.svc:9002"
	for _, c := range []struct {
		name string
		o    CreateLoadBalancerOptions
		want string
	}{
		{"a fullproxy rule", CreateLoadBalancerOptions{Mode: "fullproxy", EppEndpoint: ep, EppFailureMode: "FailClose", EppTimeoutMs: 600000}, ""},
		{"inert fields without an endpoint", CreateLoadBalancerOptions{Mode: "onearm", EppFailureMode: "FailOpen", EppTimeoutMs: 5, EppPlaintext: true}, ""},
		{"LB-EPP-FULLPROXY", CreateLoadBalancerOptions{Mode: "onearm", EppEndpoint: ep}, "AI features require '--mode fullproxy'"},
		{"P/D with explicit FailOpen", CreateLoadBalancerOptions{Mode: "fullproxy", EppEndpoint: ep, PdDisaggMode: true, EppFailureMode: "FailOpen", EpRoles: []string{"prefill", "decode"}}, ""},
		{"P/D with FailClose", CreateLoadBalancerOptions{Mode: "fullproxy", EppEndpoint: ep, PdDisaggMode: true, EppFailureMode: "FailClose", EpRoles: []string{"prefill", "decode"}}, "requires --epp-failure-mode FailOpen"},
		{"P/D with omitted mode", CreateLoadBalancerOptions{Mode: "fullproxy", EppEndpoint: ep, PdDisaggMode: true, EpRoles: []string{"prefill", "decode"}}, "requires --epp-failure-mode FailOpen"},
		{"native P/D without EPP", CreateLoadBalancerOptions{Mode: "fullproxy", PdDisaggMode: true, EppFailureMode: "FailClose", EpRoles: []string{"prefill", "decode"}}, ""},
		{"an unknown failure mode", CreateLoadBalancerOptions{Mode: "fullproxy", EppEndpoint: ep, EppFailureMode: "failopen"}, "--epp-failure-mode must be one of FailOpen|FailClose"},
		{"a timeout over the ceiling", CreateLoadBalancerOptions{Mode: "fullproxy", EppEndpoint: ep, EppTimeoutMs: 600001}, "--epp-timeout-ms must be within 0..600000"},
		{"a negative timeout", CreateLoadBalancerOptions{Mode: "fullproxy", EppEndpoint: ep, EppTimeoutMs: -1}, "--epp-timeout-ms must be within 0..600000"},
		{"an address over 255 bytes", CreateLoadBalancerOptions{Mode: "fullproxy", EppEndpoint: strings.Repeat("a", 256)}, "--epp-endpoint must be at most 255 bytes"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := validateLBAIOptions(&c.o)
			if c.want == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %v, want %q", err, c.want)
			}
		})
	}
}
