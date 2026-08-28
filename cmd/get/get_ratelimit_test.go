package get

import (
	"reflect"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

func TestRateLimitRows(t *testing.T) {
	entry := api.AITenantRateLimitEntry{
		TenantID: "tenant-a", Rps: 50, TokensPerMin: 2000, BurstPct: 125,
		ModelLimits: []api.AITenantModelRateLimit{
			{Model: "mistral-7b", TokensPerMin: 700},
			{Model: "llama-70b", TokensPerMin: 900},
		},
		UpdatedAt: "2026-08-28T00:00:00Z",
	}
	wantDefault := []string{"tenant-a", "50", "2000", "125", "2", "2026-08-28T00:00:00Z"}
	if got := rateLimitRow(entry, false); !reflect.DeepEqual(got, wantDefault) {
		t.Fatalf("default row = %v, want %v", got, wantDefault)
	}
	wantWide := []string{"tenant-a", "50", "2000", "125", "llama-70b=900,mistral-7b=700", "2026-08-28T00:00:00Z"}
	if got := rateLimitRow(entry, true); !reflect.DeepEqual(got, wantWide) {
		t.Fatalf("wide row = %v, want %v", got, wantWide)
	}
}
