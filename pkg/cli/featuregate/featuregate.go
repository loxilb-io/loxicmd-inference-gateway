/*
 * Copyright (c) 2026 LoxiLB Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at:
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package featuregate decides, before a command sends a request that needs
// a gateway feature, whether the peer offers it (contracts/exit-codes.md,
// FEATURE_UNAVAILABLE). The switch is what the gateway advertises in GET
// /version, read once per invocation: a peer that is not a
// loxilb-inference-gateway is refused; a gateway that does not advertise
// the feature's token is warned about and the request is sent, since
// gateways up to 7a58e59 advertise none of the keys below.
package featuregate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
)

// The feature keys, as the companion design names them. They are the
// tokens the gate looks for in VersionGetEntry.features.
const (
	Bands       = "bands"
	Objectives  = "objectives"
	NetIdentity = "netidentity"
	Reservation = "reservation"
)

// cache holds the /version answer per gateway for the invocation, so a
// command that gates on two features reads it once.
var (
	cacheMu sync.Mutex
	cache   = map[string]api.LBVersionGet{}
)

// Require reads the gateway's /version (once per invocation) and decides
// whether a command needing feature may send its request. A peer that is
// not a loxilb-inference-gateway is refused with FEATURE_UNAVAILABLE and
// nothing is sent. A gateway that advertises features but not this one
// gets one warning line on stderr and the request goes out: the gateway's
// answer decides. An unreachable peer is UNAVAILABLE, as the request
// itself would have been.
func Require(restOptions *api.RESTOptions, feature string, stderr io.Writer) error {
	version, err := read(restOptions)
	if err != nil {
		return err
	}
	if version.Product != api.GatewayProduct {
		return Unavailable(feature)
	}
	for _, f := range version.Features {
		if f == feature {
			return nil
		}
	}
	fmt.Fprintf(stderr, "warning: gateway does not advertise %s; sending anyway, the gateway's answer decides\n", feature)
	return nil
}

// Unavailable is the FEATURE_UNAVAILABLE failure for feature.
func Unavailable(feature string) *exitcode.CLIError {
	return &exitcode.CLIError{
		Code:          exitcode.FeatureUnavailable,
		Message:       fmt.Sprintf("this gateway does not advertise %s; upgrade the gateway or use a build that does", feature),
		Origin:        "gateway",
		ComponentCode: "feature-unavailable",
	}
}

// PathNotFound reports whether a 404 is the gateway's router refusing the
// path itself (a gateway that predates the resource) rather than a named
// item being absent. The router's answer names the path.
func PathNotFound(status int, body []byte) bool {
	if status != http.StatusNotFound {
		return false
	}
	var doc struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return false
	}
	return strings.HasPrefix(doc.Message, "path /") && strings.HasSuffix(doc.Message, "was not found")
}

// Classify turns a non-2xx answer on a feature's path into the taxonomy
// row: FEATURE_UNAVAILABLE when the path itself is not served, otherwise
// the status's own row with the gateway's body.
func Classify(op, feature string, status int, body []byte) *exitcode.CLIError {
	if PathNotFound(status, body) {
		ce := Unavailable(feature)
		ce.HTTPStatus = status
		return ce
	}
	ce := exitcode.FromHTTPStatus(op, status)
	if status == http.StatusBadRequest {
		ce.Message = api.NewAPIError(status, body).Verbatim()
	} else {
		ce.Message = api.NewAPIError(status, body).Error()
	}
	return ce
}

func read(restOptions *api.RESTOptions) (api.LBVersionGet, error) {
	key := restOptions.Protocol + "://" + restOptions.ServerIP + ":" + fmt.Sprint(restOptions.ServerPort)
	cacheMu.Lock()
	cached, ok := cache[key]
	cacheMu.Unlock()
	if ok {
		return cached, nil
	}
	ctx := context.TODO()
	var cancel context.CancelFunc
	if restOptions.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(restOptions.Timeout)*time.Second)
		defer cancel()
	}
	resp, err := api.NewLoxiClient(restOptions).LBVersion().Get(ctx)
	if err != nil {
		return api.LBVersionGet{}, exitcode.Unavailablef("get version: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		ce := exitcode.FromHTTPStatus("get version", resp.StatusCode)
		ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
		return api.LBVersionGet{}, ce
	}
	var version api.LBVersionGet
	if err := json.Unmarshal(body, &version); err != nil {
		return api.LBVersionGet{}, &exitcode.CLIError{Code: exitcode.ContractMismatch, Message: fmt.Sprintf("get version: decode response: %v", err), Origin: "gateway", HTTPStatus: resp.StatusCode}
	}
	cacheMu.Lock()
	cache[key] = version
	cacheMu.Unlock()
	return version, nil
}
