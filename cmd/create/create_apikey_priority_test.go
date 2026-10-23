package create

import (
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

func TestCreateAPIKeyPriorityFlagsRegistered(t *testing.T) {
	flags := NewCreateAPIKeyCmd(&api.RESTOptions{}).Flags()
	for name, typ := range map[string]string{"priority-class": "string", "default-objective": "string", "allowed-objectives": "stringSlice"} {
		f := flags.Lookup(name)
		if f == nil || f.Value.Type() != typ {
			t.Fatalf("--%s not registered as %s", name, typ)
		}
	}
}
