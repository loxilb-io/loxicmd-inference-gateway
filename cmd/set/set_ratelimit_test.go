package set

import (
	"encoding/json"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

func TestParseTenantModelLimits(t *testing.T) {
	got, err := parseTenantModelLimits([]string{"llama-70b=9000", "mistral-7b=0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Model != "llama-70b" || got[0].TokensPerMin != 9000 || got[1].TokensPerMin != 0 {
		t.Fatalf("unexpected parsed limits: %+v", got)
	}
}

func TestParseTenantModelLimitsRejectsInvalid(t *testing.T) {
	for name, values := range map[string][]string{
		"missing separator": {"llama-70b"},
		"empty model":       {"=100"},
		"empty quota":       {"llama-70b="},
		"negative":          {"llama-70b=-1"},
		"not numeric":       {"llama-70b=many"},
		"duplicate":         {"llama-70b=1", "llama-70b=2"},
		"delimiter":         {"tenant|model=1"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseTenantModelLimits(values); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestTenantModelLimitTombstoneSerialization(t *testing.T) {
	req := api.AITenantRateLimitMod{
		TenantID: "tenant-a",
		ModelLimits: []api.AITenantModelRateLimit{
			{Model: "mistral-7b", TokensPerMin: 0},
		},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"tenant_id":"tenant-a","model_limits":[{"model":"mistral-7b","tokens_per_min":0}]}`
	if string(b) != want {
		t.Fatalf("serialized request = %s, want %s", b, want)
	}
}

func TestTenantRateLimitLegacySerialization(t *testing.T) {
	b, err := json.Marshal(api.AITenantRateLimitMod{TenantID: "tenant-a", Rps: 10, TokensPerMin: 1000})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"tenant_id":"tenant-a","rps":10,"tokens_per_min":1000}`
	if string(b) != want {
		t.Fatalf("serialized request = %s, want %s", b, want)
	}
}

func TestTenantRateLimitLegacyResponseSerialization(t *testing.T) {
	b, err := json.Marshal(api.AITenantRateLimitEntry{
		TenantID: "tenant-a", Rps: 10, TokensPerMin: 1000, UpdatedAt: "2026-08-28T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"tenant_id":"tenant-a","rps":10,"tokens_per_min":1000,"updated_at":"2026-08-28T00:00:00Z"}`
	if string(b) != want {
		t.Fatalf("serialized response = %s, want %s", b, want)
	}
}
