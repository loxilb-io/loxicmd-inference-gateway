package get

import (
	"reflect"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

func TestFcBandsCell(t *testing.T) {
	for _, c := range []struct {
		name string
		eff  *api.FcEffective
		want string
	}{
		{"no gate state", nil, "-"},
		{"a gateway without bands", &api.FcEffective{Mode: "enforce"}, "-"},
		{"as configured", &api.FcEffective{WakePolicy: "strict", WakePolicyEffective: "strict", BandFallbackReason: "none"}, "strict"},
		{"a fallback", &api.FcEffective{WakePolicy: "weighted", WakePolicyEffective: "strict", BandFallbackReason: "zero_weight"}, "weighted (effective: strict, reason: zero_weight)"},
		{"a fallback that keeps the policy", &api.FcEffective{WakePolicy: "strict", WakePolicyEffective: "strict", BandFallbackReason: "no_queue"}, "strict (reason: no_queue)"},
		{"an effective value without a reason", &api.FcEffective{WakePolicy: "weighted", WakePolicyEffective: "strict"}, "weighted (effective: strict)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := fcBandsCell(c.eff); got != c.want {
				t.Fatalf("cell %q, want %q", got, c.want)
			}
		})
	}
}

// Absent band state prints "-" in every cell, never 0; reported state
// prints every value, zero included, with the knob sources in order.
func TestFcBandRows(t *testing.T) {
	dash := []string{"-", "-", "-", "-", "-", "-", "-", "-", "-"}
	want := [][]string{append([]string{"critical"}, dash...), append([]string{"standard"}, dash...), append([]string{"sheddable"}, dash...)}
	if got := fcBandRows(nil); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows without state %v", got)
	}
	if got := fcBandRows(&api.FcEffective{Mode: "enforce"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows for a gateway without bands %v", got)
	}
	eff := &api.FcEffective{WakePolicy: "weighted", Bands: &api.FcEffectiveBands{
		CriticalReservePct: 20, CriticalReserveUnits: 2,
		Critical:  api.FcBandEffective{Inflight: 1, QueuePct: 50, MinSharePct: 10, FloorUnits: 1, Weight: 8, WeightPct: 67},
		Standard:  api.FcBandEffective{Queued: 2, WaitMs: 5000, QueuePct: 30, MinSharePct: 5, FloorUnits: 1, Weight: 3, WeightPct: 25},
		Sheddable: api.FcBandEffective{WaitMs: 1000, QueuePct: 20, Weight: 1, WeightPct: 8},
	}, Source: &api.FcEffectiveSource{BandWeights: "rule", BandMinSharePct: "rule", BandWaitMs: "env"}}
	want = [][]string{
		{"critical", "1", "0", "0", "50", "10", "1", "8", "67", "rule/rule/env/-"},
		{"standard", "0", "2", "5000", "30", "5", "1", "3", "25", "rule/rule/env/-"},
		{"sheddable", "0", "0", "1000", "20", "0", "0", "1", "8", "rule/rule/env/-"},
	}
	if got := fcBandRows(eff); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows %v, want %v", got, want)
	}
	eff.Source = nil
	if got := fcBandRows(eff)[0][9]; got != "-" {
		t.Fatalf("source without sources %q", got)
	}
}

func TestFcBandsHeading(t *testing.T) {
	svc := api.LoadBalancerService{Name: "chat", ExternalIP: "192.0.2.1", Port: 8000, Protocol: "tcp"}
	if got := fcBandsHeading(svc); got != "chat 192.0.2.1:8000/tcp  bands: -" {
		t.Fatalf("heading without state %q", got)
	}
	svc.FcEffective = &api.FcEffective{WakePolicy: "weighted", WakePolicyEffective: "strict", BandFallbackReason: "zero_weight",
		ShedQueue: "on", ShedQueueEffective: "off", TenantWake: "fifo", TenantWakeEffective: "fifo", ConfigSource: "store",
		Bands: &api.FcEffectiveBands{CriticalReservePct: 20, CriticalReserveUnits: 2}}
	want := "chat 192.0.2.1:8000/tcp  wake policy: weighted (effective: strict, reason: zero_weight)  shed queue: on (effective: off)  tenant wake: fifo  critical reserve: 20% (2 units)  config source: store"
	if got := fcBandsHeading(svc); got != want {
		t.Fatalf("heading\n%q, want\n%q", got, want)
	}
	svc.Name = ""
	svc.FcEffective.Bands = nil
	svc.FcEffective.ConfigSource = ""
	if got := fcBandsHeading(svc); got != "- 192.0.2.1:8000/tcp  wake policy: weighted (effective: strict, reason: zero_weight)  shed queue: on (effective: off)  tenant wake: fifo  critical reserve: -  config source: -" {
		t.Fatalf("heading with absent members %q", got)
	}
}
