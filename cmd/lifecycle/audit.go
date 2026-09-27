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
	"sort"
	"strings"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// DefaultSyslogFacility is the facility the gateway documents as its default
// (13, log audit). The CLI sends it explicitly rather than leaving it at
// zero, because the gateway reads a zero facility as "not set" and
// substitutes 13 — so a zero here would arrive as 13 anyway, and sending the
// value the operator will read back is the honest form. It also means
// facility 0 (kernel) is not selectable through this API, which the flag's
// help says rather than advertising a range it cannot deliver.
const DefaultSyslogFacility int64 = 13

// MaxSyslogFacility is the largest facility RFC 5424 table 1 defines and the
// largest the gateway accepts. Bounding it here turns a typo into a local
// refusal naming the range, instead of a bare 400 from the gateway.
const MaxSyslogFacility int64 = 23

// AuditStatusGet reads the audit trail's status (GET /audit/status).
//
// The gateway does not audit this read, by design, so that it can be polled.
// JSON mode prints the gateway's body verbatim, as the other diagnostics
// reads do.
func AuditStatusGet(restOptions *api.RESTOptions, out io.Writer, jsonOut bool) error {
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.AuditStatus().Get(ctx)
	if err != nil {
		return transportError("audit status request failed", err)
	}
	defer resp.Body.Close()
	body, err := readBody(resp.Body, "audit status")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return api.NewStatusError(resp.StatusCode, body)
	}
	var st api.AuditStatusResult
	// available is sent on every answer, including the one that reports an
	// unusable audit directory. A body without it is not a status, and
	// rendering "not available" for a response that never said so would
	// invent the single fact this command exists to report.
	if json.Unmarshal(body, &st) != nil || st.Available == nil {
		return &api.LifecycleError{
			Reason:  api.ReasonDecodeFailed,
			Message: "the gateway's audit status response could not be decoded",
			Body:    string(body),
		}
	}
	if jsonOut {
		_, _ = out.Write(append(body, '\n'))
		return nil
	}
	humanAuditStatus(out, &st)
	return nil
}

func humanAuditStatus(out io.Writer, st *api.AuditStatusResult) {
	if !*st.Available {
		// Every other field describes nothing in this case, so printing
		// them would be printing zeros as if they were measurements.
		fmt.Fprintln(out, "Audit: NOT AVAILABLE - no writer was configured at start.")
		fmt.Fprintln(out, "  The audit directory was unusable; every audited management call is refused.")
		return
	}
	state := "running"
	if !st.Running {
		state = "NOT RUNNING (restarting after a failure)"
	}
	fmt.Fprintf(out, "Audit: available, %s\n", state)
	if st.BootID != "" {
		fmt.Fprintf(out, "  Boot id: %s\n", st.BootID)
	}
	fmt.Fprintf(out, "  Sequence high: %d\n", st.SeqHigh)
	if st.LastWrite != "" {
		fmt.Fprintf(out, "  Last write: %s\n", st.LastWrite)
	} else {
		fmt.Fprintln(out, "  Last write: none yet")
	}

	if len(st.Accepted) > 0 {
		fmt.Fprintf(out, "  Accepted: %s\n", joinCounts(st.Accepted))
	}
	if len(st.Dropped) > 0 {
		fmt.Fprintln(out, "  Dropped:")
		for _, d := range st.Dropped {
			fmt.Fprintf(out, "    %s/%s: %d\n", d.Stream, d.Reason, d.Count)
		}
	}
	// A lost result means a change happened whose outcome is not in the
	// trail. It is never folded into a generic drop total.
	if st.ResultDrops > 0 {
		fmt.Fprintf(out, "  Result records LOST after the change was applied: %d\n", st.ResultDrops)
	}
	if len(st.QueueDepth) > 0 {
		fmt.Fprintf(out, "  Queue depth: %s\n", joinCounts(st.QueueDepth))
	}
	if len(st.QueueHWM) > 0 {
		fmt.Fprintf(out, "  Queue high-water: %s\n", joinCounts(st.QueueHWM))
	}

	if st.Segment != nil {
		fmt.Fprintf(out, "  Active segment: %s\n", st.Segment.UUID)
		fmt.Fprintf(out, "    Opened: %s, %d records, %d bytes\n",
			st.Segment.Opened, st.Segment.Records, st.Segment.Bytes)
	}
	fmt.Fprintf(out, "  Sealed on disk: %d bytes\n", st.SealedBytes)
	if st.Retention != nil {
		r := st.Retention
		fmt.Fprintf(out, "  Retention: age %s, quota %s, reserve %s\n",
			boundSeconds(r.MaxAgeSeconds), boundBytes(r.MaxBytes), boundBytes(r.ReserveBytes))
	}
	if st.ProjectedRetentionDays > 0 {
		fmt.Fprintf(out, "  Projected retention: %.1f days\n", st.ProjectedRetentionDays)
	} else {
		fmt.Fprintln(out, "  Projected retention: not projectable yet")
	}
	if st.ReserveBreached {
		fmt.Fprintln(out, "  RESERVE BREACHED - durable management writes are refused until space is recovered")
	}

	// An orphaned intent is a change from the previous boot whose outcome
	// is unknown; it is the one counter an investigator is handed a lookup
	// key for.
	if st.OrphanedIntents > 0 {
		fmt.Fprintf(out, "  Orphaned intents from the previous boot: %d\n", st.OrphanedIntents)
		if st.LastOrphanEventID != "" {
			fmt.Fprintf(out, "    Most recent event id: %s\n", st.LastOrphanEventID)
		}
	}

	counters := []struct {
		label string
		value int64
	}{
		{"write failures", st.WriteFailures},
		{"sync failures", st.SyncFailures},
		{"mgmt timeouts", st.MgmtTimeouts},
		{"panics", st.Panics},
		{"restarts", st.Restarts},
		{"rotations", st.Rotations},
		{"rotation failures", st.RotationFailed},
		{"compress failures", st.CompressFailed},
		{"compress skipped", st.CompressSkipped},
		{"pruned segments", st.Pruned},
		{"reserve breaches", st.ReserveBreaches},
		{"paths sanitized", st.PathSanitized},
		{"unattributed records", st.Unattributed},
		{"permissions repaired", st.PermRepaired},
		{"originator headers dropped", st.OriginatorDropped},
		{"delegation lookups", st.DelegationLookups},
		{"heartbeats", st.Heartbeats},
	}
	var nonzero []string
	for _, c := range counters {
		if c.value != 0 {
			nonzero = append(nonzero, fmt.Sprintf("%s %d", c.label, c.value))
		}
	}
	if len(nonzero) > 0 {
		fmt.Fprintf(out, "  Counters: %s\n", strings.Join(nonzero, ", "))
	} else {
		fmt.Fprintln(out, "  Counters: all zero")
	}

	if len(st.Producers) > 0 {
		fmt.Fprintln(out, "  Producers:")
		for _, p := range st.Producers {
			fmt.Fprintf(out, "    %s (%s): pseq high %d, accepted %d",
				p.ID, p.Stream, p.PSeqHigh, p.Accepted)
			if len(p.Dropped) > 0 {
				fmt.Fprintf(out, ", dropped %s", joinCounts(p.Dropped))
			}
			if p.DropRingOverflows != 0 {
				fmt.Fprintf(out, ", drop-ring overflows %d", p.DropRingOverflows)
			}
			fmt.Fprintln(out)
		}
	}
}

// joinCounts renders a stream-keyed map in a stable order, so a rendering
// pinned by a golden does not move with Go's map iteration.
func joinCounts(m map[string]int64) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}

// boundSeconds and boundBytes say "unbounded" for the zero the contract
// defines as "no limit", rather than printing a 0 that reads like a limit of
// nothing.
func boundSeconds(v int64) string {
	if v == 0 {
		return "unbounded"
	}
	return fmt.Sprintf("%ds", v)
}

func boundBytes(v int64) string {
	if v == 0 {
		return "unbounded"
	}
	return fmt.Sprintf("%d bytes", v)
}

// AuditSinkGet reads the remote sink's configuration and session state
// (GET /audit/sink). Certificate material is named by path and never served,
// so what comes back are paths on the gateway's filesystem.
func AuditSinkGet(restOptions *api.RESTOptions, out io.Writer, jsonOut bool) error {
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.AuditSink().Get(ctx)
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
	// A sink that is not configured is a legitimate answer in which every
	// field is absent, so the only shape check available is "a JSON
	// object" — the same reasoning the log-archives listing uses.
	var probe map[string]json.RawMessage
	var cfg api.AuditSinkConfig
	if json.Unmarshal(body, &probe) != nil || json.Unmarshal(body, &cfg) != nil {
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
	humanAuditSink(out, &cfg)
	return nil
}

func humanAuditSink(out io.Writer, cfg *api.AuditSinkConfig) {
	if !cfg.Enabled {
		fmt.Fprintln(out, "Audit sink: not configured - the trail is local only.")
		return
	}
	session := "not connected"
	if cfg.Connected {
		session = "connected"
	}
	fmt.Fprintf(out, "Audit sink: enabled, %s\n", session)
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
	// Submitted counts writes to the socket. Syslog over TLS carries no
	// acknowledgement, so calling this "delivered" would overstate what
	// the gateway can know.
	fmt.Fprintf(out, "  Submitted to the socket: %d (not a delivery count; the protocol has no acknowledgement)\n",
		cfg.Submitted)
	if cfg.Truncated != 0 {
		fmt.Fprintf(out, "  Sent shortened to fit the receiver's cap: %d\n", cfg.Truncated)
	}
	if cfg.WriteErrors != 0 {
		fmt.Fprintf(out, "  Write errors: %d (each one holds the cursor back)\n", cfg.WriteErrors)
	}
	if cfg.LastError != "" {
		fmt.Fprintf(out, "  Last error: %s\n", cfg.LastError)
	}
}

// AuditSinkSetOptions is the complete sink configuration a set sends.
//
// There is no "unchanged" here on purpose: POST /audit/sink replaces the
// configuration wholesale, so the CLI mirrors that and sends all eight
// fields every time. A flag the operator omits takes its documented default,
// it does not keep whatever the gateway had.
type AuditSinkSetOptions struct {
	// Disable tears the sink down. With it set, every other field is
	// irrelevant and the gateway closes the session and forgets the
	// configuration.
	Disable bool

	Address        string
	CABundlePath   string
	ServerName     string
	ClientCertPath string
	ClientKeyPath  string
	MaxFrameBytes  int64
	Facility       int64
}

// AuditSinkSet replaces the remote sink configuration (POST /audit/sink).
//
// The arguments are validated here, before the request, for the cases where
// a local refusal is strictly more useful than the gateway's 400: a missing
// receiver or trust anchor, half a client keypair, a facility or frame cap
// outside its range. The gateway refuses the same things — it is the
// authority, and it also refuses what it alone can see, such as a CA bundle
// it cannot read — but a refusal that names the flag is what an operator can
// act on.
//
// Like every other state-changing call, an outcome this process cannot know
// is reported as recovery-required, never as success: the sink may or may not
// have been replaced, and only 'get audit-sink' can say.
func AuditSinkSet(restOptions *api.RESTOptions, out io.Writer, jsonOut bool, so AuditSinkSetOptions) error {
	req, err := auditSinkRequest(so)
	if err != nil {
		return err
	}
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.AuditSink().Create(ctx, req)
	if err != nil {
		return &api.LifecycleError{
			Reason: api.ReasonRecoveryRequired,
			Message: fmt.Sprintf(
				"the audit sink change could not be confirmed and may or may not have been applied: %v; "+
					"verify the actual configuration with 'loxicmd get audit-sink' before acting on it", err),
		}
	}
	defer resp.Body.Close()
	body, err := readBody(resp.Body, "audit sink")
	if err != nil {
		return err
	}
	// The endpoint answers 204 with no body. Anything else is the
	// gateway's refusal, carried through verbatim.
	if resp.StatusCode != http.StatusNoContent {
		return api.NewStatusError(resp.StatusCode, body)
	}
	if jsonOut {
		_, _ = out.Write([]byte("{\"result\":\"Success\"}\n"))
		return nil
	}
	if so.Disable {
		fmt.Fprintln(out, "Audit sink removed. The trail is local only.")
		return nil
	}
	fmt.Fprintf(out, "Audit sink replaced: %s, verified against %s.\n", req.Address, req.CABundlePath)
	return nil
}

// auditSinkRequest turns the flags into the complete body the endpoint
// replaces the configuration with.
func auditSinkRequest(so AuditSinkSetOptions) (api.AuditSinkRequest, error) {
	if so.Disable {
		// Enabled false is the whole request: the gateway closes the
		// session and drops the configuration, and reads nothing else.
		return api.AuditSinkRequest{Enabled: false}, nil
	}
	invalid := func(format string, a ...any) (api.AuditSinkRequest, error) {
		return api.AuditSinkRequest{}, &api.LifecycleError{
			Reason:  api.ReasonInvalidArguments,
			Message: fmt.Sprintf(format, a...),
		}
	}
	if strings.TrimSpace(so.Address) == "" {
		return invalid("--address is required to configure a sink (use --disable to remove one)")
	}
	if strings.TrimSpace(so.CABundlePath) == "" {
		// There is no unverified mode, so this is not a default the CLI
		// could pick on the operator's behalf.
		return invalid("--ca-bundle is required: the receiver's certificate is always verified and there is no mode that disables it")
	}
	// Both or neither: half a keypair is a sink that cannot present a
	// client certificate while looking as though it can.
	if (so.ClientCertPath == "") != (so.ClientKeyPath == "") {
		return invalid("--client-cert and --client-key must be given together, or neither")
	}
	if so.Facility < 0 || so.Facility > MaxSyslogFacility {
		return invalid("--facility must be between 0 and %d, got %d", MaxSyslogFacility, so.Facility)
	}
	if so.MaxFrameBytes < 0 {
		return invalid("--max-frame-bytes cannot be negative, got %d", so.MaxFrameBytes)
	}
	return api.AuditSinkRequest{
		Enabled:        true,
		Address:        so.Address,
		CABundlePath:   so.CABundlePath,
		ServerName:     so.ServerName,
		ClientCertPath: so.ClientCertPath,
		ClientKeyPath:  so.ClientKeyPath,
		MaxFrameBytes:  so.MaxFrameBytes,
		Facility:       so.Facility,
	}, nil
}
