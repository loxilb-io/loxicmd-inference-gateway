package create

import (
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// The twelve band and reservation flags go on the wire under the field
// names the gateway documents, with the same presence rules as the other
// fc_* flags: given goes on the wire (an explicit reset included), omitted
// stays off it.
func TestFcBandServiceArguments(t *testing.T) {
	t.Run("declared", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.50", Select: "rr",
			FcWakePolicy: "weighted", FcBandWeights: api.BandTriple{Critical: 8, Standard: 3, Sheddable: 1}, FcBandWeightsSet: true,
			FcBandMinSharePct: api.BandTriple{Critical: 10, Standard: 5, Sheddable: 0}, FcBandMinSharePctSet: true,
			FcCriticalReservePct: 20, FcCriticalReservePctSet: true,
			FcBandWaitMs: api.BandTriple{Critical: 0, Standard: 5000, Sheddable: 1000}, FcBandWaitMsSet: true,
			FcBandQueuePct: api.BandTriple{Critical: 50, Standard: 30, Sheddable: 20}, FcBandQueuePctSet: true,
			FcShedQueue: "on", FcTenantWake: "rr", FcLlmdHeaders: "on",
			AiDefaultReservation: "on", AiDefaultMaxTokens: 512, AiDefaultMaxTokensSet: true,
			AiObjectiveStrict: true, AiObjectiveStrictSet: true})
		assertKey(t, m, "fc_wake_policy", "weighted")
		assertKey(t, m, "fc_band_weights", "8:3:1")
		assertKey(t, m, "fc_band_min_share_pct", "10:5:0")
		assertKey(t, m, "fc_critical_reserve_pct", 20)
		assertKey(t, m, "fc_band_wait_ms", "0:5000:1000")
		assertKey(t, m, "fc_band_queue_pct", "50:30:20")
		assertKey(t, m, "fc_shed_queue", "on")
		assertKey(t, m, "fc_tenant_wake", "rr")
		assertKey(t, m, "fc_llmd_headers", "on")
		assertKey(t, m, "ai_default_reservation", "on")
		assertKey(t, m, "ai_default_max_tokens", 512)
		assertKey(t, m, "ai_objective_strict", true)
	})
	t.Run("omitted stays off the wire", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.51", Select: "rr"})
		assertAbsent(t, m, "fc_wake_policy", "fc_band_weights", "fc_band_min_share_pct", "fc_critical_reserve_pct",
			"fc_band_wait_ms", "fc_band_queue_pct", "fc_shed_queue", "fc_tenant_wake", "fc_llmd_headers",
			"ai_default_reservation", "ai_default_max_tokens", "ai_objective_strict")
	})
	t.Run("explicit resets go on the wire", func(t *testing.T) {
		m := serviceMap(t, &CreateLoadBalancerOptions{ExternalIP: "192.0.2.52", Select: "rr",
			FcWakePolicy: "inherit", FcBandWeightsSet: true, FcCriticalReservePctSet: true,
			AiDefaultMaxTokensSet: true, AiObjectiveStrictSet: true})
		assertKey(t, m, "fc_wake_policy", "inherit")
		assertKey(t, m, "fc_band_weights", "0:0:0")
		assertKey(t, m, "fc_critical_reserve_pct", 0)
		assertKey(t, m, "ai_default_max_tokens", 0)
		assertKey(t, m, "ai_objective_strict", false)
		assertAbsent(t, m, "fc_band_min_share_pct", "fc_band_wait_ms", "fc_band_queue_pct")
	})
	for _, c := range []struct {
		name string
		o    CreateLoadBalancerOptions
		want string
	}{
		{"an unknown wake policy", CreateLoadBalancerOptions{FcWakePolicy: "fair"}, "--fc-wake-policy must be one of strict|weighted|inherit"},
		{"an unknown shed queue", CreateLoadBalancerOptions{FcShedQueue: "yes"}, "--fc-shed-queue must be one of on|off|inherit"},
		{"an unknown tenant wake", CreateLoadBalancerOptions{FcTenantWake: "lifo"}, "--fc-tenant-wake must be one of fifo|rr|inherit"},
		{"an unknown llm-d switch", CreateLoadBalancerOptions{FcLlmdHeaders: "yes"}, "--fc-llmd-headers must be one of on|off|inherit"},
		{"an unknown reservation switch", CreateLoadBalancerOptions{AiDefaultReservation: "yes"}, "--ai-default-reservation must be one of on|off|inherit"},
		{"a weight above 62", CreateLoadBalancerOptions{FcBandWeights: api.BandTriple{Critical: 63, Standard: 1, Sheddable: 1}, FcBandWeightsSet: true}, "--fc-band-weights: each value must be within 0..62"},
		{"a floor above 100", CreateLoadBalancerOptions{FcBandMinSharePct: api.BandTriple{Critical: 0, Standard: 101, Sheddable: 0}, FcBandMinSharePctSet: true}, "--fc-band-min-share-pct: each value must be within 0..100"},
		{"a queue share above 100", CreateLoadBalancerOptions{FcBandQueuePct: api.BandTriple{Critical: 0, Standard: 0, Sheddable: 101}, FcBandQueuePctSet: true}, "--fc-band-queue-pct: each value must be within 0..100"},
		{"a wait above an hour", CreateLoadBalancerOptions{FcBandWaitMs: api.BandTriple{Critical: 3600001, Standard: 0, Sheddable: 0}, FcBandWaitMsSet: true}, "--fc-band-wait-ms: each value must be within 0..3600000"},
		{"a reserve above 100", CreateLoadBalancerOptions{FcCriticalReservePct: 101}, "--fc-critical-reserve-pct must be within 0..100"},
		{"tokens above the ceiling", CreateLoadBalancerOptions{AiDefaultMaxTokens: 100000001}, "--ai-default-max-tokens must be within 0..100000000"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := validateLBAIOptions(&c.o)
			if err == nil || err.Error() != c.want {
				t.Fatalf("error %v, want %q", err, c.want)
			}
		})
	}
	// The rules between values are the gateway's: a sum of weights over 64
	// and a zero beside non-zero weights pass here and are its 400.
	t.Run("cross-field rules are left to the gateway", func(t *testing.T) {
		o := CreateLoadBalancerOptions{FcWakePolicy: "weighted", FcBandWeights: api.BandTriple{Critical: 62, Standard: 62, Sheddable: 0}, FcBandWeightsSet: true,
			FcBandMinSharePct: api.BandTriple{Critical: 100, Standard: 100, Sheddable: 100}, FcBandMinSharePctSet: true, FcCriticalReservePct: 100}
		if err := validateLBAIOptions(&o); err != nil {
			t.Fatalf("refused locally: %v", err)
		}
	})
	t.Run("the flags are registered", func(t *testing.T) {
		flags := NewCreateLoadBalancerCmd(&api.RESTOptions{}).Flags()
		for name, typ := range map[string]string{"fc-wake-policy": "string",
			"fc-band-weights": "critical:standard:sheddable", "fc-band-min-share-pct": "critical:standard:sheddable",
			"fc-critical-reserve-pct": "uint32", "fc-band-wait-ms": "critical:standard:sheddable",
			"fc-band-queue-pct": "critical:standard:sheddable", "fc-shed-queue": "string", "fc-tenant-wake": "string",
			"fc-llmd-headers": "string", "ai-default-reservation": "string", "ai-default-max-tokens": "uint32",
			"ai-objective-strict": "bool"} {
			f := flags.Lookup(name)
			if f == nil || f.Value.Type() != typ {
				t.Fatalf("--%s not registered as %s", name, typ)
			}
		}
	})
	t.Run("a malformed triple is refused at the flag", func(t *testing.T) {
		cmd := NewCreateLoadBalancerCmd(&api.RESTOptions{})
		err := cmd.Flags().Set("fc-band-weights", "8:3")
		if err == nil {
			t.Fatal("8:3 accepted")
		}
		if err := cmd.Flags().Set("fc-band-weights", "8:3:1"); err != nil {
			t.Fatal(err)
		}
		if got := cmd.Flags().Lookup("fc-band-weights").Value.String(); got != "8:3:1" {
			t.Fatalf("flag reads back %q", got)
		}
	})
}
