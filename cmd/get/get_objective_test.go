package get

import (
	"reflect"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

func TestObjectiveRendering(t *testing.T) {
	list := api.AIObjectiveList{Source: "store", SourceTimestamp: "2026-10-23T05:50:58Z", StoreReachable: true, StoreLastRead: "2026-10-23T05:50:58Z"}
	if got := objectiveSourceLine(list); got != "source: store  source time: 2026-10-23T05:50:58Z  store reachable: true  store last read: 2026-10-23T05:50:58Z" {
		t.Fatalf("source line %q", got)
	}
	if got := objectiveSourceLine(api.AIObjectiveList{}); got != "source: -  source time: -  store reachable: false  store last read: -" {
		t.Fatalf("absent source line %q", got)
	}
	if got := objectiveRow(api.AIObjectiveEntry{Name: "batch", Priority: -5}); !reflect.DeepEqual(got, []string{"batch", "-5", "-", "-"}) {
		t.Fatalf("row %v", got)
	}
	if got := objectiveRow(api.AIObjectiveEntry{Name: "fast", Priority: 10, Services: []string{"a:1", "b:2"}, UpdatedAt: "u"}); !reflect.DeepEqual(got, []string{"fast", "10", "a:1,b:2", "u"}) {
		t.Fatalf("row %v", got)
	}
	if len(objectiveRow(api.AIObjectiveEntry{})) != len(OBJECTIVE_TITLE) {
		t.Fatal("row and title widths differ")
	}
}

func TestNetIdentityRendering(t *testing.T) {
	for _, c := range []struct {
		m    api.NetIdentityMatch
		want string
	}{
		{api.NetIdentityMatch{}, "-"},
		{api.NetIdentityMatch{Cidr: []string{"10.0.0.0/8", "10.1.0.0/16"}}, "cidr=10.0.0.0/8,10.1.0.0/16"},
		{api.NetIdentityMatch{IngressIf: "eth1"}, "ingress_if=eth1"},
		{api.NetIdentityMatch{Vlan: 100}, "vlan=100"},
		{api.NetIdentityMatch{Vni: 5001}, "vni=5001"},
		{api.NetIdentityMatch{IpsecTunnel: "site-b"}, "ipsec_tunnel=site-b"},
		{api.NetIdentityMatch{Cidr: []string{"10.0.0.0/8"}, Vlan: 7}, "cidr=10.0.0.0/8 vlan=7"},
	} {
		if got := netIdentityMatchCell(c.m); got != c.want {
			t.Errorf("match %+v -> %q, want %q", c.m, got, c.want)
		}
	}
	b := api.NetIdentityEntry{Name: "lab", Tenant: "t", Match: api.NetIdentityMatch{Vlan: 100}, Enforce: "strict", Credential: "network", FailMode: "closed",
		ForwardIdentity: true, PriorityClass: "critical", AllowedModels: []string{"m"}, Services: []string{"a:1"}}
	if got := netIdentityRow(b, false); !reflect.DeepEqual(got, []string{"lab", "t", "vlan=100", "strict", "network", "closed"}) {
		t.Fatalf("row %v", got)
	}
	if got := netIdentityRow(b, true); !reflect.DeepEqual(got, []string{"lab", "t", "vlan=100", "strict", "network", "closed", "true", "critical", "m", "a:1"}) {
		t.Fatalf("wide row %v", got)
	}
	bare := api.NetIdentityEntry{Name: "x", Tenant: "t"}
	if got := netIdentityRow(bare, true); !reflect.DeepEqual(got, []string{"x", "t", "-", "-", "-", "-", "false", "-", "-", "-"}) {
		t.Fatalf("bare wide row %v", got)
	}
	if len(netIdentityRow(bare, true)) != len(NETIDENTITY_WIDE_TITLE) || len(netIdentityRow(bare, false)) != len(NETIDENTITY_TITLE) {
		t.Fatal("row and title widths differ")
	}
}
