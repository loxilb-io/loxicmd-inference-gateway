package create

import (
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

func TestGetTargetPair(t *testing.T) {
	for name, input := range map[string]struct {
		target     string
		objectName string
		attachment api.PolObjType
	}{
		"port symbolic":         {"eth0:port", "eth0", 1},
		"egress symbolic":       {"eth1:egress-port", "eth1", 2},
		"numeric compatibility": {"eth2:2", "eth2", 2},
		"IPv4 rule":             {"192.0.2.10:443:tcp:rule", "192.0.2.10:443:tcp", 0},
		"IPv6 rule":             {"[2001:db8::10]:443:tcp:0", "[2001:db8::10]:443:tcp", 0},
	} {
		t.Run(name, func(t *testing.T) {
			body := api.PolMod{}
			if err := GetTargetPair(&body, input.target); err != nil {
				t.Fatal(err)
			}
			if body.Target.PolObjName != input.objectName || body.Target.AttachMent != input.attachment {
				t.Fatalf("target = %+v", body.Target)
			}
		})
	}
}

func TestGetTargetPairRejectsInvalid(t *testing.T) {
	for name, target := range map[string]string{
		"missing attachment": "eth0",
		"invalid attachment": "eth0:3",
		"port with colon":    "eth0:extra:port",
		"bare IPv6":          "2001:db8::10:443:tcp:rule",
		"missing protocol":   "192.0.2.10:443:rule",
		"invalid IP":         "example.invalid:443:tcp:rule",
		"invalid port":       "192.0.2.10:70000:tcp:rule",
		"invalid protocol":   "192.0.2.10:443:http:rule",
	} {
		t.Run(name, func(t *testing.T) {
			if err := GetTargetPair(&api.PolMod{}, target); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
