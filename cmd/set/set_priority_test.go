package set

import (
	"encoding/json"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// A priority flag given goes on the wire, an empty one included (it
// clears); one not given stays off it so the stored value is kept.
func TestApplyTenantPriority(t *testing.T) {
	req := api.AITenantRateLimitMod{TenantID: "tenant-a"}
	applyTenantPriority(&req, &SetRateLimitOptions{})
	if b, _ := json.Marshal(req); string(b) != `{"tenant_id":"tenant-a"}` {
		t.Fatalf("nothing given: %s", b)
	}
	req = api.AITenantRateLimitMod{TenantID: "tenant-a"}
	applyTenantPriority(&req, &SetRateLimitOptions{PriorityClass: "critical", PriorityClassSet: true,
		DefaultObjective: "batch", DefaultObjectiveSet: true, AllowedObjectives: []string{"batch", "bulk"}, AllowedObjectivesSet: true})
	if b, _ := json.Marshal(req); string(b) != `{"tenant_id":"tenant-a","priority_class":"critical","default_objective":"batch","allowed_objectives":["batch","bulk"]}` {
		t.Fatalf("given: %s", b)
	}
	req = api.AITenantRateLimitMod{TenantID: "tenant-a"}
	applyTenantPriority(&req, &SetRateLimitOptions{PriorityClassSet: true, DefaultObjectiveSet: true, AllowedObjectivesSet: true})
	if b, _ := json.Marshal(req); string(b) != `{"tenant_id":"tenant-a","priority_class":"","default_objective":"","allowed_objectives":[]}` {
		t.Fatalf("cleared: %s", b)
	}
}

func TestPriorityFlagsRegistered(t *testing.T) {
	for cmdName, flags := range map[string]map[string]string{
		"apikey":    {"priority-class": "string", "default-objective": "string", "allowed-objectives": "stringSlice"},
		"ratelimit": {"priority-class": "string", "default-objective": "string", "allowed-objectives": "stringSlice"},
	} {
		cmd := NewSetAPIKeyCmd(&api.RESTOptions{})
		if cmdName == "ratelimit" {
			cmd = NewSetRateLimitCmd(&api.RESTOptions{})
		}
		for name, typ := range flags {
			f := cmd.Flags().Lookup(name)
			if f == nil || f.Value.Type() != typ {
				t.Fatalf("set %s --%s not registered as %s", cmdName, name, typ)
			}
		}
	}
}
