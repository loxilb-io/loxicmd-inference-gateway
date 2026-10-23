package api

import (
	"encoding/json"
	"testing"
)

// The priority fields travel under the names the gateway documents: the
// key create body and the summary carry them when set and omit them when
// not; the key PATCH and the tenant POST send an empty value to clear and
// nothing to keep.
func TestPriorityFieldsOnTheWire(t *testing.T) {
	b, _ := json.Marshal(AIApiKeyCreateRequest{TenantID: "t", PriorityClass: "critical", DefaultObjective: "batch", AllowedObjectives: []string{"batch", "interactive"}})
	if want := `{"tenant_id":"t","priority_class":"critical","default_objective":"batch","allowed_objectives":["batch","interactive"]}`; string(b) != want {
		t.Fatalf("create body %s, want %s", b, want)
	}
	b, _ = json.Marshal(AIApiKeyCreateRequest{TenantID: "t"})
	if string(b) != `{"tenant_id":"t"}` {
		t.Fatalf("create body without priority %s", b)
	}

	var sum AIApiKeySummary
	const reported = `{"key_id":"k","enabled":true,"priority_class":"sheddable","default_objective":"bulk","allowed_objectives":["bulk"]}`
	if err := json.Unmarshal([]byte(reported), &sum); err != nil || sum.PriorityClass != "sheddable" || len(sum.AllowedObjectives) != 1 {
		t.Fatalf("summary read as %+v, %v", sum, err)
	}

	empty := ""
	none := []string{}
	b, _ = json.Marshal(AIApiKeyPatchRequest{PriorityClass: &empty, DefaultObjective: &empty, AllowedObjectives: &none})
	if string(b) != `{"priority_class":"","default_objective":"","allowed_objectives":[]}` {
		t.Fatalf("patch clear body %s", b)
	}
	b, _ = json.Marshal(AIApiKeyPatchRequest{})
	if string(b) != `{}` {
		t.Fatalf("patch keep body %s", b)
	}

	class := "critical"
	b, _ = json.Marshal(AITenantRateLimitMod{TenantID: "t", PriorityClass: &class, AllowedObjectives: &none})
	if string(b) != `{"tenant_id":"t","priority_class":"critical","allowed_objectives":[]}` {
		t.Fatalf("tenant body %s", b)
	}
	var entry AITenantRateLimitEntry
	if err := json.Unmarshal([]byte(`{"tenant_id":"t","rps":1,"tokens_per_min":2,"priority_class":"standard","default_objective":"d","allowed_objectives":["a","b"],"updated_at":"2026-10-23T00:00:00Z"}`), &entry); err != nil || entry.DefaultObjective != "d" || len(entry.AllowedObjectives) != 2 {
		t.Fatalf("entry read as %+v, %v", entry, err)
	}
	out, _ := json.Marshal(entry)
	if want := `{"tenant_id":"t","rps":1,"tokens_per_min":2,"priority_class":"standard","default_objective":"d","allowed_objectives":["a","b"],"updated_at":"2026-10-23T00:00:00Z"}`; string(out) != want {
		t.Fatalf("entry written back as %s", out)
	}
}

func TestIsPriorityClass(t *testing.T) {
	for _, ok := range []string{"standard", "critical", "sheddable"} {
		if !IsPriorityClass(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "Critical", "high", "tier:priority"} {
		if IsPriorityClass(bad) {
			t.Errorf("%q accepted", bad)
		}
	}
}
