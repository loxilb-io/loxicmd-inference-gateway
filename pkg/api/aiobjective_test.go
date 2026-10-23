package api

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The objective body carries name, priority (always, zero included) and
// services when set; the list reads back every member the gateway reports.
func TestObjectiveWire(t *testing.T) {
	b, _ := json.Marshal(AIObjectiveEntry{Name: "batch"})
	if string(b) != `{"name":"batch","priority":0}` {
		t.Fatalf("minimal body %s", b)
	}
	b, _ = json.Marshal(AIObjectiveEntry{Name: "fast", Priority: -5, Services: []string{"192.0.2.1:8000"}})
	if string(b) != `{"name":"fast","services":["192.0.2.1:8000"],"priority":-5}` {
		t.Fatalf("full body %s", b)
	}
	const reported = `{"objectiveAttr":[{"name":"probe","priority":1,"updated_at":"2026-10-23T05:50:57.997Z"}],"source":"store","source_timestamp":"2026-10-23T05:50:58.010Z","store_last_read":"2026-10-23T05:50:58.010Z","store_reachable":true}`
	var list AIObjectiveList
	if err := json.Unmarshal([]byte(reported), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Objectives) != 1 || list.Objectives[0].UpdatedAt == "" || !list.StoreReachable || list.Source != "store" {
		t.Fatalf("list read as %+v", list)
	}
	out, _ := json.Marshal(list)
	var want, got any
	_ = json.Unmarshal([]byte(reported), &want)
	_ = json.Unmarshal(out, &got)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("list written back as %s", out)
	}
}

// The binding body carries name, match and tenant always and the rest when
// set; a listed binding reads back every member.
func TestNetIdentityWire(t *testing.T) {
	b, _ := json.Marshal(NetIdentityEntry{Name: "lab", Tenant: "t", Match: NetIdentityMatch{Cidr: []string{"10.0.0.0/8"}}})
	if string(b) != `{"name":"lab","match":{"cidr":["10.0.0.0/8"]},"tenant":"t"}` {
		t.Fatalf("minimal body %s", b)
	}
	full := NetIdentityEntry{Name: "edge", Tenant: "t", Match: NetIdentityMatch{Vlan: 100}, Services: []string{"192.0.2.1:8000"},
		AllowedModels: []string{"m"}, Enforce: "strict", Credential: "network", ForwardIdentity: true, PriorityClass: "critical", FailMode: "closed"}
	b, _ = json.Marshal(full)
	const want = `{"name":"edge","match":{"vlan":100},"services":["192.0.2.1:8000"],"tenant":"t","allowed_models":["m"],"enforce":"strict","credential":"network","forward_identity":true,"priority_class":"critical","fail_mode":"closed"}`
	if string(b) != want {
		t.Fatalf("full body %s", b)
	}
	var list NetIdentityList
	if err := json.Unmarshal([]byte(`{"netIdentityAttr":[`+want+`]}`), &list); err != nil || len(list.Bindings) != 1 || !reflect.DeepEqual(list.Bindings[0], full) {
		t.Fatalf("list read as %+v, %v", list, err)
	}
}
