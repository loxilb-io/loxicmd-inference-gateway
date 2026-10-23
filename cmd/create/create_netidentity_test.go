package create

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

func TestNetIdentityEntryFromFlags(t *testing.T) {
	e := netIdentityEntry("lab", &CreateNetIdentityOptions{Tenant: "t", Cidr: []string{"10.0.0.0/8"}, Services: []string{"192.0.2.1:8000"},
		AllowedModels: []string{"m"}, Enforce: "advisory", Credential: "none", ForwardIdentity: true, PriorityClass: "sheddable", FailMode: "neutral"})
	b, _ := json.Marshal(e)
	const want = `{"name":"lab","match":{"cidr":["10.0.0.0/8"]},"services":["192.0.2.1:8000"],"tenant":"t","allowed_models":["m"],"enforce":"advisory","credential":"none","forward_identity":true,"priority_class":"sheddable","fail_mode":"neutral"}`
	if string(b) != want {
		t.Fatalf("body %s", b)
	}
	e = netIdentityEntry("edge", &CreateNetIdentityOptions{Tenant: "t", IngressIf: "eth1"})
	if b, _ = json.Marshal(e); string(b) != `{"name":"edge","match":{"ingress_if":"eth1"},"tenant":"t"}` {
		t.Fatalf("minimal body %s", b)
	}
	for name, o := range map[string]CreateNetIdentityOptions{
		"vni": {Vni: 5001}, "ipsec": {IpsecTunnel: "site-b"}, "vlan": {Vlan: 100},
	} {
		if b, _ = json.Marshal(netIdentityEntry("x", &o)); !bytes.Contains(b, []byte(`"match":{"`)) || bytes.Contains(b, []byte(`"match":{}`)) {
			t.Fatalf("%s match %s", name, b)
		}
	}
}

// Format only: enums, each value's range and the cidr form. Which match
// fields go together is the gateway's.
func TestValidateNetIdentityOptions(t *testing.T) {
	for _, c := range []struct {
		name string
		o    CreateNetIdentityOptions
		want string
	}{
		{"enforce", CreateNetIdentityOptions{Enforce: "always"}, "--enforce must be one of advisory|strict"},
		{"credential", CreateNetIdentityOptions{Credential: "yes"}, "--credential must be one of none|network"},
		{"fail mode", CreateNetIdentityOptions{FailMode: "open"}, "--fail-mode must be one of neutral|closed"},
		{"priority class", CreateNetIdentityOptions{PriorityClass: "high"}, "--priority-class must be one of standard|critical|sheddable"},
		{"vlan", CreateNetIdentityOptions{Vlan: 4095}, "--vlan must be within 1..4094"},
		{"vni", CreateNetIdentityOptions{Vni: 16777216}, "--vni must be within 1..16777215"},
		{"cidr v6", CreateNetIdentityOptions{Cidr: []string{"2001:db8::/32"}}, `--cidr "2001:db8::/32" is not an IPv4 network in a.b.c.d/len form`},
		{"cidr no length", CreateNetIdentityOptions{Cidr: []string{"10.0.0.1"}}, `--cidr "10.0.0.1" is not an IPv4 network in a.b.c.d/len form`},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := validateNetIdentityOptions(&c.o)
			if err == nil || err.Error() != c.want {
				t.Fatalf("error %v, want %q", err, c.want)
			}
		})
	}
	t.Run("cross-field rules are left to the gateway", func(t *testing.T) {
		o := CreateNetIdentityOptions{Cidr: []string{"10.0.0.0/8"}, Vlan: 100, Credential: "network", ForwardIdentity: true}
		if err := validateNetIdentityOptions(&o); err != nil {
			t.Fatalf("refused locally: %v", err)
		}
	})
	t.Run("the flags are registered", func(t *testing.T) {
		flags := NewCreateNetIdentityCmd(&api.RESTOptions{}).Flags()
		for name, typ := range map[string]string{"tenant": "string", "cidr": "stringSlice", "ingress-if": "string", "vlan": "uint32",
			"vni": "uint32", "ipsec-tunnel": "string", "services": "stringSlice", "allowed-models": "stringSlice", "enforce": "string",
			"credential": "string", "forward-identity": "bool", "priority-class": "string", "fail-mode": "string"} {
			f := flags.Lookup(name)
			if f == nil || f.Value.Type() != typ {
				t.Fatalf("--%s not registered as %s", name, typ)
			}
		}
		if f := NewCreateObjectiveCmd(&api.RESTOptions{}).Flags().Lookup("priority"); f == nil || f.Value.Type() != "int32" {
			t.Fatal("create objective --priority not registered as int32")
		}
	})
}

// The gateway's result line is printed as answered, a warning after the
// status included; -o json prints the body.
func TestPrintOperationResult(t *testing.T) {
	var out bytes.Buffer
	if err := printOperationResult(&out, []byte(`{"result":"Success; warning: fail_mode: closed by lab"}`), ""); err != nil || out.String() != "Success; warning: fail_mode: closed by lab\n" {
		t.Fatalf("human %q, %v", out.String(), err)
	}
	out.Reset()
	if err := printOperationResult(&out, []byte(`{"result":"Success"}`), "json"); err != nil || out.String() != "{\n    \"result\": \"Success\"\n}\n" {
		t.Fatalf("json %q, %v", out.String(), err)
	}
	out.Reset()
	if err := printOperationResult(&out, []byte("OK\n"), ""); err != nil || out.String() != "OK\n" {
		t.Fatalf("plain %q, %v", out.String(), err)
	}
}
