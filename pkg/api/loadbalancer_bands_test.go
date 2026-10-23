package api

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Every one of the twelve band and reservation knobs the gateway reports
// reads into the model and writes out again with the same value, so a rule
// that is read and sent back keeps them; a rule that reports none writes
// none.
func TestLoadBalancerServiceBandKnobsRoundTrip(t *testing.T) {
	const reported = `{
		"fc_wake_policy": "weighted", "fc_band_weights": "8:3:1",
		"fc_band_min_share_pct": "10:5:0", "fc_critical_reserve_pct": 20,
		"fc_band_wait_ms": "0:5000:1000", "fc_band_queue_pct": "50:30:20",
		"fc_shed_queue": "on", "fc_tenant_wake": "rr", "fc_llmd_headers": "on",
		"ai_default_reservation": "on", "ai_default_max_tokens": 512,
		"ai_objective_strict": true
	}`
	var svc LoadBalancerService
	if err := json.Unmarshal([]byte(reported), &svc); err != nil {
		t.Fatal(err)
	}
	if svc.FcBandWeights == nil || *svc.FcBandWeights != (BandTriple{8, 3, 1}) {
		t.Fatalf("fc_band_weights read as %+v", svc.FcBandWeights)
	}
	out, err := json.Marshal(svc)
	if err != nil {
		t.Fatal(err)
	}
	var want, got map[string]any
	_ = json.Unmarshal([]byte(reported), &want)
	_ = json.Unmarshal(out, &got)
	if len(want) != 12 {
		t.Fatalf("the fixture names %d knobs, want 12", len(want))
	}
	for key, value := range want {
		if !reflect.DeepEqual(got[key], value) {
			t.Errorf("%s: written back as %v, reported as %v", key, got[key], value)
		}
	}

	out, _ = json.Marshal(LoadBalancerService{})
	got = nil
	_ = json.Unmarshal(out, &got)
	for key := range want {
		if _, present := got[key]; present {
			t.Errorf("%s is written for a rule that does not report it", key)
		}
	}

	// Explicit zero, false and 0:0:0 go on the wire when set, since each
	// resets or declares something on a replace.
	zero := uint32(0)
	off := false
	none := BandTriple{}
	out, _ = json.Marshal(LoadBalancerService{FcCriticalReservePct: &zero, AiDefaultMaxTokens: &zero,
		AiObjectiveStrict: &off, FcBandWeights: &none})
	if string(out) != `{"externalIP":"","port":0,"protocol":"","sel":0,"mode":0,"BGP":false,"Monitor":false,"inactiveTimeOut":0,"block":0,"proxyprotocolv2":false,"egress":false,`+
		`"fc_band_weights":"0:0:0","fc_critical_reserve_pct":0,"ai_default_max_tokens":0,"ai_objective_strict":false}` {
		t.Fatalf("explicit resets written as %s", out)
	}
}

// The extended fc_effective the gateway reports on GET reads into the model
// and writes out identically: every band, reservation, store and source
// member, zero values inside the bands included.
func TestFcEffectiveBandsRoundTrip(t *testing.T) {
	const reported = `{
		"mode": "enforce", "max_outstanding": 8, "effective_max_outstanding": 8,
		"tenant_slots": 1024,
		"wake_policy": "weighted", "wake_policy_effective": "strict", "weights_in_use": true,
		"shed_queue": "on", "shed_queue_effective": "off",
		"tenant_wake": "rr", "tenant_wake_effective": "rr",
		"band_fallback_reason": "zero_weight", "llmd_headers": "on", "bands_configured": true,
		"bands": {
			"critical_reserve_pct": 20, "critical_reserve_units": 2,
			"critical": {"inflight": 1, "queued": 0, "wait_ms": 0, "queue_pct": 50, "min_share_pct": 10, "floor_units": 1, "weight": 8, "weight_pct": 67},
			"standard": {"inflight": 0, "queued": 2, "wait_ms": 5000, "queue_pct": 30, "min_share_pct": 5, "floor_units": 1, "weight": 3, "weight_pct": 25},
			"sheddable": {"inflight": 0, "queued": 0, "wait_ms": 1000, "queue_pct": 20, "min_share_pct": 0, "floor_units": 0, "weight": 1, "weight_pct": 8}
		},
		"default_reservation": "on", "default_max_tokens": 512,
		"reservation_clamped_burst": 3, "reservation_clamped_budget": 7,
		"reservation_epoch": 29876543, "reservation_held_claims": 2, "reservation_held_truncated": true,
		"reservation_held": [{"kind": "tenant", "bucket": "tenant-a", "tokens": 1024}, {"kind": "service", "bucket": "svc", "tokens": 0}],
		"config_source": "lkg", "store_reachable": false, "store_last_read": "2026-10-23T05:00:00Z",
		"source": {"mode": "rule", "default_reservation": "env", "default_max_tokens": "default",
			"wake_policy": "rule", "band_weights": "rule", "band_min_share_pct": "rule",
			"critical_reserve_pct": "rule", "band_wait_ms": "env", "band_queue_pct": "default",
			"shed_queue": "rule", "tenant_wake": "env", "llmd_headers": "rule"}
	}`
	var eff FcEffective
	if err := json.Unmarshal([]byte(reported), &eff); err != nil {
		t.Fatal(err)
	}
	if eff.Bands == nil || eff.Bands.Standard.Queued != 2 || eff.StoreReachable == nil || *eff.StoreReachable {
		t.Fatalf("read as %+v", eff)
	}
	out, err := json.Marshal(eff)
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	_ = json.Unmarshal([]byte(reported), &want)
	_ = json.Unmarshal(out, &got)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("written back as\n%s\nreported as\n%s", out, reported)
	}

	// A gateway that predates the bands reports none: none is written and
	// the absence is visible (nil, not an empty block).
	var older FcEffective
	_ = json.Unmarshal([]byte(`{"mode":"enforce","max_outstanding":8}`), &older)
	if older.Bands != nil || older.StoreReachable != nil || older.WakePolicy != "" {
		t.Fatalf("absent members read as present: %+v", older)
	}
	out, _ = json.Marshal(older)
	if string(out) != `{"mode":"enforce","max_outstanding":8}` {
		t.Fatalf("absent members written: %s", out)
	}
}
