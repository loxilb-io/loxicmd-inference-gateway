package featuregate

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
)

func gateway(t *testing.T, status int, body string) (*api.RESTOptions, *int) {
	t.Helper()
	reads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/netlox/v1/version" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		reads++
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	return &api.RESTOptions{Protocol: "http", ServerIP: u.Hostname(), ServerPort: port}, &reads
}

// A peer that is not a loxilb-inference-gateway is refused with
// FEATURE_UNAVAILABLE and a message that says so, before anything is sent.
func TestRequireRefusesNonGateway(t *testing.T) {
	for name, c := range map[string]struct{ body, seen string }{
		"plain loxilb":    {`{"version":"0.9.8","buildInfo":"x"}`, "no product in /version"},
		"another product": {`{"version":"1","product":"something-else","features":["bands"]}`, `product "something-else" in /version`},
	} {
		t.Run(name, func(t *testing.T) {
			opts, _ := gateway(t, http.StatusOK, c.body)
			var stderr bytes.Buffer
			err := Require(opts, Bands, "create lb", &stderr)
			ce, ok := err.(*exitcode.CLIError)
			if !ok || ce.Code != exitcode.FeatureUnavailable {
				t.Fatalf("error %v, want FEATURE_UNAVAILABLE", err)
			}
			want := fmt.Sprintf("the peer at %s:%d is not a loxilb-inference-gateway (%s); create lb needs one", opts.ServerIP, opts.ServerPort, c.seen)
			if ce.Message != want {
				t.Fatalf("message %q, want %q", ce.Message, want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("a refusal also warned: %q", stderr.String())
			}
		})
	}
}

// A gateway that does not advertise the token warns once on stderr and
// lets the request go; one that advertises it is silent. /version is read
// once per gateway for the invocation.
func TestRequireWarnsOnMissingToken(t *testing.T) {
	opts, reads := gateway(t, http.StatusOK, `{"version":"main","product":"loxilb-inference-gateway","features":["api_key_auth","jwt_auth_profile","objectives"]}`)
	var stderr bytes.Buffer
	if err := Require(opts, Bands, "get lb --bands", &stderr); err != nil {
		t.Fatalf("missing token refused: %v", err)
	}
	if stderr.String() != "warning: gateway does not advertise bands; sending anyway, the gateway's answer decides\n" {
		t.Fatalf("warning %q", stderr.String())
	}
	stderr.Reset()
	if err := Require(opts, Objectives, "create objective", &stderr); err != nil || stderr.Len() != 0 {
		t.Fatalf("advertised token: err %v, stderr %q", err, stderr.String())
	}
	if *reads != 1 {
		t.Fatalf("/version read %d times, want once", *reads)
	}
}

// A gateway that cannot be read is UNAVAILABLE, as the request would be;
// a non-200 /version is classified by its status.
func TestRequireReadFailures(t *testing.T) {
	opts, _ := gateway(t, http.StatusUnauthorized, `{"message":"no"}`)
	err := Require(opts, Bands, "create lb", &bytes.Buffer{})
	if ce, ok := err.(*exitcode.CLIError); !ok || ce.Code != exitcode.Auth {
		t.Fatalf("401 on /version: %v", err)
	}
	opts, _ = gateway(t, http.StatusOK, ``)
	srvClosed := *opts
	srvClosed.ServerPort = 1
	err = Require(&srvClosed, Bands, "create lb", &bytes.Buffer{})
	if ce, ok := err.(*exitcode.CLIError); !ok || ce.Code != exitcode.Unavailable {
		t.Fatalf("unreachable: %v", err)
	}
}

// The gateway's router answers a path it does not serve with a 404 that
// names the path; a named item that is absent answers 404 without it.
func TestClassify(t *testing.T) {
	ce := Classify("get objective", Objectives, http.StatusNotFound, []byte(`{"code":404,"message":"path /netlox/v1/config/ai/objective was not found"}`))
	if ce.Code != exitcode.FeatureUnavailable || ce.HTTPStatus != 404 || !strings.Contains(ce.Message, "does not advertise objectives") {
		t.Fatalf("path 404: %+v", ce)
	}
	ce = Classify("delete objective", Objectives, http.StatusNotFound, []byte(`{"code":404,"message":"Resource not found"}`))
	if ce.Code != exitcode.Precondition || ce.Message != "server returned 404: Resource not found" {
		t.Fatalf("item 404: %+v", ce)
	}
	body := `{"code":400,"fields":["credential"],"message":"Malformed arguments for API call","result":"credential: network needs an interface match"}`
	ce = Classify("create netidentity", NetIdentity, http.StatusBadRequest, []byte(body))
	if ce.Code != exitcode.ContractMismatch || ce.Message != "server returned 400: "+body {
		t.Fatalf("400: %+v", ce)
	}
	ce = Classify("create objective", Objectives, http.StatusServiceUnavailable, []byte(`{"message":"ai_key_store_unavailable"}`))
	if ce.Code != exitcode.Unavailable || ce.Message != "server returned 503: ai_key_store_unavailable" {
		t.Fatalf("503: %+v", ce)
	}
}
