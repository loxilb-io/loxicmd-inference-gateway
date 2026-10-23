package get

import (
	"reflect"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// The wide view shows the three priority fields; one the key does not set
// prints "-". The default view is unchanged.
func TestAPIKeyRowPriority(t *testing.T) {
	k := api.AIApiKeySummary{KeyID: "k1", TenantID: "t", Name: "n", AllowedModels: []string{"m"}, RateLimitRps: 1, BurstSize: 2, TokensPerMin: 3, Enabled: true,
		CreatedAt: "c", ExpiresAt: "e", PriorityClass: "critical", DefaultObjective: "batch", AllowedObjectives: []string{"batch", "bulk"}}
	if got, want := apiKeyRow(k, false), []string{"k1", "t", "n", "m", "1", "2", "3", "true"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default row %v", got)
	}
	if got, want := apiKeyRow(k, true), []string{"k1", "t", "n", "m", "1", "2", "3", "true", "c", "e", "critical", "batch", "batch,bulk"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("wide row %v", got)
	}
	k.PriorityClass, k.DefaultObjective, k.AllowedObjectives = "", "", nil
	if got := apiKeyRow(k, true)[10:]; !reflect.DeepEqual(got, []string{"-", "-", "-"}) {
		t.Fatalf("absent priority %v", got)
	}
	if len(apiKeyRow(k, true)) != len(APIKEY_WIDE_TITLE) || len(apiKeyRow(k, false)) != len(APIKEY_TITLE) {
		t.Fatal("row and title widths differ")
	}
}

func TestRateLimitRowPriority(t *testing.T) {
	entry := api.AITenantRateLimitEntry{TenantID: "t", Rps: 1, TokensPerMin: 2, BurstPct: 3, UpdatedAt: "u",
		PriorityClass: "sheddable", AllowedObjectives: []string{"bulk"}}
	if got, want := rateLimitRow(entry, true), []string{"t", "1", "2", "3", "", "sheddable", "-", "bulk", "u"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("wide row %v", got)
	}
	if got, want := rateLimitRow(entry, false), []string{"t", "1", "2", "3", "0", "u"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default row %v", got)
	}
	if len(rateLimitRow(entry, true)) != len(RATELIMIT_WIDE_TITLE) || len(rateLimitRow(entry, false)) != len(RATELIMIT_TITLE) {
		t.Fatal("row and title widths differ")
	}
}
