package get

import "testing"

func TestVersionFeaturesLine(t *testing.T) {
	if got := versionFeaturesLine(nil); got != "features: -" {
		t.Fatalf("none %q", got)
	}
	if got := versionFeaturesLine([]string{"api_key_auth", "jwt_auth_profile"}); got != "features: api_key_auth, jwt_auth_profile" {
		t.Fatalf("some %q", got)
	}
}
