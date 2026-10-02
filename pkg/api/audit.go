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
package api

// AuditStatus is the client for GET /audit/status — the audit writer's
// counters and segment state. Status only: no record content is served over
// the management API, so there is deliberately no client here for reading
// the trail itself.
type AuditStatus struct {
	CommonAPI
}

// AuditSink is the client for GET and POST /audit/sink — the remote sink's
// configuration and session state.
type AuditSink struct {
	CommonAPI
}

// AuditSinks is the client for GET, PUT and DELETE /audit/sinks/{name} — the
// secondary sinks that follow the trail beside the compliance sink of
// /audit/sink. The name is a sub-resource; there is no listing endpoint, the
// configured sinks are named in /audit/status.
type AuditSinks struct {
	CommonAPI
}

// AuditSegmentStatus mirrors the segment the writer is appending to.
type AuditSegmentStatus struct {
	UUID    string `json:"uuid"`
	Opened  string `json:"opened"`
	Records int64  `json:"records"`
	Bytes   int64  `json:"bytes"`
}

// AuditRetentionPolicy mirrors the retention bounds in force. Zero disables
// a bound, so the rendering says "unbounded" rather than printing 0.
type AuditRetentionPolicy struct {
	MaxAgeSeconds int64 `json:"max_age_seconds"`
	MaxBytes      int64 `json:"max_bytes"`
	ReserveBytes  int64 `json:"reserve_bytes"`
}

// AuditDropCount is one (stream, reason) drop tally. Absent reasons are zero,
// so the gateway sends only the reasons that actually fired.
type AuditDropCount struct {
	Stream string `json:"stream"`
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

// AuditProducerStatus is one producer's accounting. PSeqHigh against the
// writer's accepted count is where a loss shows up, so both are rendered.
type AuditProducerStatus struct {
	ID                string           `json:"id"`
	Stream            string           `json:"stream"`
	PSeqHigh          int64            `json:"pseq_high"`
	Accepted          int64            `json:"accepted"`
	Dropped           map[string]int64 `json:"dropped"`
	DropRingOverflows int64            `json:"drop_ring_overflows"`
}

// AuditStatusResult mirrors the /audit/status contract.
//
// Available is a pointer for the same reason LogPage.HasMore is: a gateway
// whose audit directory was unusable at start still answers, with available
// false, and that answer is the explanation for every refused management
// call. A body without the field is not a status, and false must stay
// distinguishable from absent — reporting "available: false" for a response
// that never said so would invent the one fact an operator acts on.
type AuditStatusResult struct {
	Available *bool  `json:"available"`
	Running   bool   `json:"running"`
	BootID    string `json:"boot_id"`
	SeqHigh   int64  `json:"seq_high"`
	LastWrite string `json:"last_write"`

	Accepted    map[string]int64 `json:"accepted"`
	Dropped     []AuditDropCount `json:"dropped"`
	ResultDrops int64            `json:"result_drops"`

	OriginatorDropped int64 `json:"originator_dropped"`
	DelegationLookups int64 `json:"delegation_lookups"`

	QueueDepth map[string]int64 `json:"queue_depth"`
	QueueHWM   map[string]int64 `json:"queue_hwm"`

	WriteFailures int64 `json:"write_failures"`
	SyncFailures  int64 `json:"sync_failures"`
	MgmtTimeouts  int64 `json:"mgmt_timeouts"`
	Panics        int64 `json:"panics"`
	Restarts      int64 `json:"restarts"`
	Heartbeats    int64 `json:"heartbeats"`
	Rotations     int64 `json:"rotations"`

	PathSanitized int64 `json:"path_sanitized"`
	Unattributed  int64 `json:"unattributed"`
	PermRepaired  int64 `json:"perm_repaired"`

	RotationFailed  int64 `json:"rotation_failed"`
	CompressFailed  int64 `json:"compress_failed"`
	CompressSkipped int64 `json:"compress_skipped"`
	Pruned          int64 `json:"pruned"`

	ReserveBreaches int64 `json:"reserve_breaches"`
	ReserveBreached bool  `json:"reserve_breached"`
	SealedBytes     int64 `json:"sealed_bytes"`

	OrphanedIntents   int64  `json:"orphaned_intents"`
	LastOrphanEventID string `json:"last_orphan_event_id"`

	Segment                *AuditSegmentStatus   `json:"segment"`
	Retention              *AuditRetentionPolicy `json:"retention"`
	ProjectedRetentionDays float64               `json:"projected_retention_days"`
	Producers              []AuditProducerStatus `json:"producers"`

	// ComplianceSink is a pointer because a gateway from before the sinks
	// followed the trail does not send it, and "no compliance sink" is a
	// statement that gateway never made.
	ComplianceSink *bool             `json:"compliance_sink"`
	Sinks          []AuditSinkStatus `json:"sinks"`
}

// AuditSinkCursor is a place in the trail: the last record a sink is past.
type AuditSinkCursor struct {
	SegmentUUID string `json:"segment_uuid"`
	Seq         int64  `json:"seq"`
}

// AuditSinkStatus is one sink's progress through the trail, as
// /audit/status reports it. LagRecords counts only the active segment, so
// it says nothing while InActiveSegment is false.
type AuditSinkStatus struct {
	Name            string           `json:"name"`
	Compliance      bool             `json:"compliance"`
	State           string           `json:"state"`
	Cursor          *AuditSinkCursor `json:"cursor"`
	InActiveSegment bool             `json:"in_active_segment"`
	LagRecords      int64            `json:"lag_records"`
	LagDrops        int64            `json:"lag_drops"`
}

// AuditSinkConfig mirrors the /audit/sink contract, which is both the read
// and the write shape. The five read-only fields are what the gateway
// reports about the current session; POST ignores them.
//
// Certificate material is named by path and never served, so these are
// paths on the gateway's own filesystem, not paths this CLI can resolve.
type AuditSinkConfig struct {
	Enabled        bool   `json:"enabled"`
	Address        string `json:"address"`
	CABundlePath   string `json:"ca_bundle_path"`
	ServerName     string `json:"server_name"`
	ClientCertPath string `json:"client_cert_path"`
	ClientKeyPath  string `json:"client_key_path"`
	MaxFrameBytes  int64  `json:"max_frame_bytes"`
	Facility       int64  `json:"facility"`

	// Read-only session state.
	Connected   bool   `json:"connected"`
	Submitted   int64  `json:"submitted"`
	Truncated   int64  `json:"truncated"`
	WriteErrors int64  `json:"write_errors"`
	LastError   string `json:"last_error"`
}

// AuditSinkRequest is what POST /audit/sink carries. It is a separate type
// from AuditSinkConfig on purpose: the endpoint REPLACES the configuration
// wholesale, and sending back the read-only session counters a GET returned
// would suggest they were part of the desired state. Only the eight
// configurable fields are sent, and all eight are sent every time.
type AuditSinkRequest struct {
	Enabled        bool   `json:"enabled"`
	Address        string `json:"address"`
	CABundlePath   string `json:"ca_bundle_path"`
	ServerName     string `json:"server_name"`
	ClientCertPath string `json:"client_cert_path"`
	ClientKeyPath  string `json:"client_key_path"`
	MaxFrameBytes  int64  `json:"max_frame_bytes"`
	Facility       int64  `json:"facility"`
}

// AuditSinkFilter mirrors what a secondary sink selects from the trail. Each
// field that is set narrows the selection; an empty filter keeps everything.
type AuditSinkFilter struct {
	Streams    []string `json:"streams,omitempty"`
	Services   []string `json:"services,omitempty"`
	Outcome    string   `json:"outcome,omitempty"`
	DataSample int64    `json:"data_sample,omitempty"`
}

// AuditNamedSinkConfig mirrors what GET /audit/sinks/{name} answers: the
// configuration and, read-only, how far the sink has got.
type AuditNamedSinkConfig struct {
	Name             string           `json:"name"`
	Address          string           `json:"address"`
	CABundlePath     string           `json:"ca_bundle_path"`
	ServerName       string           `json:"server_name"`
	ClientCertPath   string           `json:"client_cert_path"`
	ClientKeyPath    string           `json:"client_key_path"`
	MaxFrameBytes    int64            `json:"max_frame_bytes"`
	Facility         int64            `json:"facility"`
	EnterpriseNumber int64            `json:"enterprise_number"`
	Filter           *AuditSinkFilter `json:"filter"`

	// Read-only state.
	State       string           `json:"state"`
	Cursor      *AuditSinkCursor `json:"cursor"`
	XseqHigh    int64            `json:"xseq_high"`
	XseqEpoch   int64            `json:"xseq_epoch"`
	Submitted   int64            `json:"submitted"`
	Filtered    int64            `json:"filtered"`
	Resent      int64            `json:"resent"`
	Poison      int64            `json:"poison"`
	Truncated   int64            `json:"truncated"`
	WriteErrors int64            `json:"write_errors"`
	LagDrops    int64            `json:"lag_drops"`
	LastError   string           `json:"last_error"`
}

// AuditNamedSinkRequest is what PUT /audit/sinks/{name} carries. Like
// AuditSinkRequest it holds the configurable fields only: the endpoint
// replaces the sink's configuration, and the name travels in the path.
type AuditNamedSinkRequest struct {
	Address          string           `json:"address"`
	CABundlePath     string           `json:"ca_bundle_path"`
	ServerName       string           `json:"server_name"`
	ClientCertPath   string           `json:"client_cert_path"`
	ClientKeyPath    string           `json:"client_key_path"`
	MaxFrameBytes    int64            `json:"max_frame_bytes"`
	Facility         int64            `json:"facility"`
	EnterpriseNumber int64            `json:"enterprise_number"`
	Filter           *AuditSinkFilter `json:"filter,omitempty"`
}
