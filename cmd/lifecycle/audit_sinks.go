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
package lifecycle

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// ComplianceSinkName is the name the gateway keeps for the sink of
// /audit/sink. A secondary sink cannot take it.
const ComplianceSinkName = "compliance"

// MaxEnterpriseNumber is the largest private enterprise number the gateway
// takes: the number is carried as 32 bits.
const MaxEnterpriseNumber int64 = 1<<32 - 1

var auditSinkNameRe = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// The streams a filter can name, and the two outcomes it can select.
var auditFilterStreams = []string{"mgmt", "data", "audit_system"}

const (
	auditOutcomeOK     = "ok"
	auditOutcomeFailed = "failed"
)

func invalidArguments(format string, a ...any) error {
	return &api.LifecycleError{
		Reason:  api.ReasonInvalidArguments,
		Message: fmt.Sprintf(format, a...),
	}
}

// checkAuditSinkName refuses a name the gateway would refuse. The gateway
// answers such a name with a bare 400, or a 404 on a read, neither of which
// says what was wrong with it.
func checkAuditSinkName(name string) error {
	if name == ComplianceSinkName {
		return invalidArguments("the name %q is reserved for the compliance sink; "+
			"that sink is read and replaced with 'audit-sink' and no name", ComplianceSinkName)
	}
	if !auditSinkNameRe.MatchString(name) {
		return invalidArguments("a sink name is 1 to 64 of a-z, 0-9, '-' and '_', got %q", name)
	}
	return nil
}

// AuditNamedSinkGet reads one secondary sink (GET /audit/sinks/{name}).
func AuditNamedSinkGet(restOptions *api.RESTOptions, out io.Writer, jsonOut bool, name string) error {
	if err := checkAuditSinkName(name); err != nil {
		return err
	}
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.AuditSinks().SubResources([]string{name}).Get(ctx)
	if err != nil {
		return transportError("audit sink request failed", err)
	}
	defer resp.Body.Close()
	body, err := readBody(resp.Body, "audit sink")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return api.NewStatusError(resp.StatusCode, body)
	}
	// A configured sink always has a receiver. A body without one is not
	// the answer to this question, and rendering it would print an empty
	// sink as though the gateway had described one.
	var cfg api.AuditNamedSinkConfig
	if json.Unmarshal(body, &cfg) != nil || cfg.Address == "" {
		return &api.LifecycleError{
			Reason:  api.ReasonDecodeFailed,
			Message: "the gateway's audit sink response could not be decoded",
			Body:    string(body),
		}
	}
	if jsonOut {
		_, _ = out.Write(append(body, '\n'))
		return nil
	}
	humanAuditNamedSink(out, name, &cfg)
	return nil
}

func humanAuditNamedSink(out io.Writer, name string, cfg *api.AuditNamedSinkConfig) {
	state := cfg.State
	if state == "" {
		state = "state not reported"
	}
	fmt.Fprintf(out, "Audit sink %s: secondary, %s\n", name, state)
	fmt.Fprintf(out, "  Receiver: %s\n", cfg.Address)
	fmt.Fprintf(out, "  CA bundle: %s\n", cfg.CABundlePath)
	if cfg.ServerName != "" {
		fmt.Fprintf(out, "  Expected server name: %s\n", cfg.ServerName)
	}
	if cfg.ClientCertPath != "" {
		fmt.Fprintf(out, "  Client certificate: %s\n", cfg.ClientCertPath)
		fmt.Fprintf(out, "  Client key: %s\n", cfg.ClientKeyPath)
	}
	fmt.Fprintf(out, "  Facility: %d\n", cfg.Facility)
	fmt.Fprintf(out, "  Max frame: %s\n", boundBytes(cfg.MaxFrameBytes))
	fmt.Fprintf(out, "  Enterprise number: %d\n", cfg.EnterpriseNumber)
	fmt.Fprintf(out, "  Selects: %s\n", describeAuditFilter(cfg.Filter))
	if cfg.Cursor != nil && cfg.Cursor.SegmentUUID != "" {
		fmt.Fprintf(out, "  Past record: seq %d in segment %s\n", cfg.Cursor.Seq, cfg.Cursor.SegmentUUID)
	} else {
		fmt.Fprintln(out, "  Past record: none yet")
	}
	// The export sequence is what the receiver checks for gaps; the epoch
	// tells a count that started again from one that lost numbers.
	fmt.Fprintf(out, "  Export sequence: high %d, epoch %d\n", cfg.XseqHigh, cfg.XseqEpoch)
	fmt.Fprintf(out, "  Submitted to the socket: %d (not a delivery count; the protocol has no acknowledgement)\n",
		cfg.Submitted)
	if cfg.Resent != 0 {
		fmt.Fprintf(out, "  Sent again after a lost session: %d\n", cfg.Resent)
	}
	if cfg.Filtered != 0 {
		fmt.Fprintf(out, "  Kept from this sink by its filter: %d\n", cfg.Filtered)
	}
	if cfg.Truncated != 0 {
		fmt.Fprintf(out, "  Sent shortened to fit the receiver's cap: %d\n", cfg.Truncated)
	}
	if cfg.Poison != 0 {
		fmt.Fprintf(out, "  Skipped because the sink cannot carry them: %d\n", cfg.Poison)
	}
	if cfg.LagDrops != 0 {
		fmt.Fprintf(out, "  Segments LOST to retention before they were sent: %d\n", cfg.LagDrops)
	}
	if cfg.WriteErrors != 0 {
		fmt.Fprintf(out, "  Write errors: %d (each one holds the cursor back)\n", cfg.WriteErrors)
	}
	if cfg.LastError != "" {
		fmt.Fprintf(out, "  Last error: %s\n", cfg.LastError)
	}
}

// describeAuditFilter says what a filter keeps. An absent filter and an
// empty one are the same thing: every record.
func describeAuditFilter(f *api.AuditSinkFilter) string {
	if f == nil {
		return "every record"
	}
	var parts []string
	if len(f.Streams) > 0 {
		parts = append(parts, "streams "+strings.Join(f.Streams, ","))
	}
	if len(f.Services) > 0 {
		parts = append(parts, "data records of services "+strings.Join(f.Services, ","))
	}
	if f.Outcome != "" {
		parts = append(parts, "outcome "+f.Outcome)
	}
	if f.DataSample > 1 {
		parts = append(parts, fmt.Sprintf("one data record in %d", f.DataSample))
	}
	if len(parts) == 0 {
		return "every record"
	}
	return strings.Join(parts, "; ")
}

// AuditNamedSinkSet creates or replaces a secondary sink
// (PUT /audit/sinks/{name}).
//
// The gateway answers a configuration it refuses with a 400 that carries no
// reason, so everything that can be judged here is: the name, the
// enterprise number, the filter's vocabulary. What only the gateway can see,
// such as a CA bundle it cannot read, still comes back as its refusal.
func AuditNamedSinkSet(restOptions *api.RESTOptions, out io.Writer, jsonOut bool, name string, so AuditSinkSetOptions) error {
	req, err := auditNamedSinkRequest(name, so)
	if err != nil {
		return err
	}
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.AuditSinks().SubResources([]string{name}).Put(ctx, req)
	if err != nil {
		return &api.LifecycleError{
			Reason: api.ReasonRecoveryRequired,
			Message: fmt.Sprintf(
				"the audit sink change could not be confirmed and may or may not have been applied: %v; "+
					"verify the actual configuration with 'loxicmd get audit-sink %s' before acting on it", err, name),
		}
	}
	defer resp.Body.Close()
	body, err := readBody(resp.Body, "audit sink")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return api.NewStatusError(resp.StatusCode, body)
	}
	if jsonOut {
		_, _ = out.Write([]byte("{\"result\":\"Success\"}\n"))
		return nil
	}
	fmt.Fprintf(out, "Audit sink %s set: %s, verified against %s; it is sent %s.\n",
		name, req.Address, req.CABundlePath, describeAuditFilter(req.Filter))
	return nil
}

// auditNamedSinkRequest turns the flags into the body the endpoint replaces
// the sink's configuration with.
func auditNamedSinkRequest(name string, so AuditSinkSetOptions) (api.AuditNamedSinkRequest, error) {
	none := api.AuditNamedSinkRequest{}
	if err := checkAuditSinkName(name); err != nil {
		return none, err
	}
	if so.Disable {
		return none, invalidArguments("--disable removes the compliance sink; "+
			"a secondary sink is removed with 'loxicmd delete audit-sink %s'", name)
	}
	t, err := auditSinkTransport(so)
	if err != nil {
		return none, err
	}
	// The export sequence travels in an element only a private enterprise
	// number can qualify, and no number is built in.
	if so.EnterpriseNumber < 1 || so.EnterpriseNumber > MaxEnterpriseNumber {
		return none, invalidArguments("--enterprise-number is required for a secondary sink and must be between 1 and %d, got %d",
			MaxEnterpriseNumber, so.EnterpriseNumber)
	}
	for _, s := range so.Streams {
		known := false
		for _, k := range auditFilterStreams {
			known = known || s == k
		}
		if !known {
			return none, invalidArguments("--stream takes %s, got %q", strings.Join(auditFilterStreams, ", "), s)
		}
	}
	for _, s := range so.Services {
		if strings.TrimSpace(s) == "" {
			return none, invalidArguments("--service cannot be empty")
		}
	}
	switch so.Outcome {
	case "", auditOutcomeOK, auditOutcomeFailed:
	default:
		return none, invalidArguments("--outcome takes %s or %s, got %q", auditOutcomeOK, auditOutcomeFailed, so.Outcome)
	}
	if so.DataSample < 0 {
		return none, invalidArguments("--data-sample cannot be negative, got %d", so.DataSample)
	}
	req := api.AuditNamedSinkRequest{
		Address:          t.Address,
		CABundlePath:     t.CABundlePath,
		ServerName:       t.ServerName,
		ClientCertPath:   t.ClientCertPath,
		ClientKeyPath:    t.ClientKeyPath,
		MaxFrameBytes:    t.MaxFrameBytes,
		Facility:         t.Facility,
		EnterpriseNumber: so.EnterpriseNumber,
	}
	// Sampling one in one keeps everything, as zero does; it is not sent
	// as a filter.
	if len(so.Streams) > 0 || len(so.Services) > 0 || so.Outcome != "" || so.DataSample > 1 {
		req.Filter = &api.AuditSinkFilter{
			Streams: so.Streams, Services: so.Services, Outcome: so.Outcome,
		}
		if so.DataSample > 1 {
			req.Filter.DataSample = so.DataSample
		}
	}
	return req, nil
}

// AuditNamedSinkDelete removes a secondary sink (DELETE /audit/sinks/{name}).
// The gateway keeps the sink's place in the trail and its export sequence,
// so a sink set again under the same name continues both.
func AuditNamedSinkDelete(restOptions *api.RESTOptions, out io.Writer, jsonOut bool, name string) error {
	if err := checkAuditSinkName(name); err != nil {
		return err
	}
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.AuditSinks().SubResources([]string{name}).Delete(ctx)
	if err != nil {
		return &api.LifecycleError{
			Reason: api.ReasonRecoveryRequired,
			Message: fmt.Sprintf(
				"the removal of audit sink %s could not be confirmed and may or may not have been applied: %v; "+
					"verify with 'loxicmd get audit-sink %s' before acting on it", name, err, name),
		}
	}
	defer resp.Body.Close()
	body, err := readBody(resp.Body, "audit sink")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return api.NewStatusError(resp.StatusCode, body)
	}
	if jsonOut {
		_, _ = out.Write([]byte("{\"result\":\"Success\"}\n"))
		return nil
	}
	fmt.Fprintf(out, "Audit sink %s removed. Its place in the trail and its export sequence are kept;"+
		" a sink set under the same name continues both.\n", name)
	return nil
}
