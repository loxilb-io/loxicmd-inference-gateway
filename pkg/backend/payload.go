/*
 * Copyright (c) 2026 LoxiLB Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package backend

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
)

// CodeBackendPayloadInvalid is the stable local component code for a backend
// document that does not satisfy its approved command tuple. Plan 04 combines
// this local verdict with execution context before choosing the public exit.
const CodeBackendPayloadInvalid = "BACKEND_PAYLOAD_INVALID"

// PayloadTuple selects exactly one approved backend payload contract.
type PayloadTuple struct {
	ContractMajor int
	SchemaVersion string
	Command       string
}

// ValidatedPayload is the only success value returned by ValidatePayload. Its
// document bytes are copied so the caller cannot mutate validator-owned input.
type ValidatedPayload struct {
	Tuple          PayloadTuple
	Command        string
	Document       json.RawMessage
	OperationID    string
	OperationError *BackendOperationError
}

// BackendOperationError is the validated, structured payload emitted when an
// operation process exits non-zero. The dispatcher still verifies that Exit
// equals the observed process status before exposing this document publicly.
type BackendOperationError struct {
	SchemaVersion    string `json:"schemaVersion"`
	Command          string `json:"command"`
	Result           string `json:"result"`
	Exit             int    `json:"exit"`
	Code             string `json:"code"`
	Origin           string `json:"origin"`
	ComponentCode    string `json:"componentCode"`
	Retryable        bool   `json:"retryable"`
	Message          string `json:"message,omitempty"`
	CorrelationID    string `json:"correlationId,omitempty"`
	OperationID      string `json:"operationId,omitempty"`
	RecoveryGuidance string `json:"recoveryGuidance,omitempty"`
}

// PayloadValidationError preserves the rejected tuple and any execution
// evidence the composition layer attaches. Its local classification is always
// ContractMismatch(6); it deliberately does not decide whether a mutating
// operation makes the final public outcome PARTIAL(8).
type PayloadValidationError struct {
	Tuple               PayloadTuple
	Command             string
	Reason              string
	ObservedBackendExit *int
	CorrelationID       string
	OperationID         string
}

func (e *PayloadValidationError) Error() string {
	return fmt.Sprintf("backend payload for %q is invalid: %s", e.Command, e.Reason)
}

func (e *PayloadValidationError) Unwrap() error {
	return &exitcode.CLIError{
		Code:          exitcode.ContractMismatch,
		Message:       e.Error(),
		Origin:        "backend",
		ComponentCode: CodeBackendPayloadInvalid,
	}
}

// WithExecutionEvidence returns a copy enriched for Plan 04 classification.
// The method cannot change this error's local exit-6 classification.
func (e *PayloadValidationError) WithExecutionEvidence(backendExit int, correlationID, operationID string) *PayloadValidationError {
	clone := *e
	clone.ObservedBackendExit = &backendExit
	clone.CorrelationID = correlationID
	clone.OperationID = operationID
	return &clone
}

type payloadSpec struct {
	Tuple        PayloadTuple
	SchemaPath   string
	SchemaID     string
	RequiredKeys []string
	Validate     func([]byte) error
}

func tuple(command string) PayloadTuple {
	return PayloadTuple{ContractMajor: contractMajor, SchemaVersion: payloadSchema, Command: command}
}

var payloadRegistry = map[PayloadTuple]payloadSpec{
	tuple("status"): {
		Tuple:        tuple("status"),
		SchemaPath:   "contracts/backend-payloads/v1/status.schema.json",
		SchemaID:     "https://loxilb.io/schemas/appliance-backend/v1/status.schema.json",
		RequiredKeys: []string{"schemaVersion", "command", "overallStatus", "productRelease", "initialized", "planes", "networkProfile", "activeOperations", "localGatewayRegistration", "observedAt"},
		Validate:     validateStatusPayload,
	},
	tuple("network validate"): {
		Tuple:        tuple("network validate"),
		SchemaPath:   "contracts/backend-payloads/v1/network-validate.schema.json",
		SchemaID:     "https://loxilb.io/schemas/appliance-backend/v1/network-validate.schema.json",
		RequiredKeys: []string{"schemaVersion", "command", "valid", "profile", "interfaces", "errors", "warnings", "observedAt"},
		Validate:     validateNetworkPayload,
	},
	tuple("public-address configure"): {
		Tuple:        tuple("public-address configure"),
		SchemaPath:   "contracts/backend-payloads/v1/public-address-configure.schema.json",
		SchemaID:     "https://loxilb.io/schemas/appliance-backend/v1/public-address-configure.schema.json",
		RequiredKeys: []string{"schemaVersion", "command", "result", "operationId", "publicAddress", "privateAddressPreserved", "certificate", "restart", "readiness"},
		Validate:     validatePublicAddressPayload,
	},
	tuple("gateway register-local"): {
		Tuple:        tuple("gateway register-local"),
		SchemaPath:   "contracts/backend-payloads/v1/gateway-register-local.schema.json",
		SchemaID:     "https://loxilb.io/schemas/appliance-backend/v1/gateway-register-local.schema.json",
		RequiredKeys: []string{"schemaVersion", "command", "result", "operationId", "installationId", "instanceId", "endpoint", "gatewayIdentity", "verified", "markerUpdated"},
		Validate:     validateGatewayRegistrationPayload,
	},
	tuple("diagnostics create"): {
		Tuple:        tuple("diagnostics create"),
		SchemaPath:   "contracts/backend-payloads/v1/diagnostics-create.schema.json",
		SchemaID:     "https://loxilb.io/schemas/appliance-backend/v1/diagnostics-create.schema.json",
		RequiredKeys: []string{"schemaVersion", "command", "result", "operationId", "archivePath", "sha256", "sizeBytes", "mode", "redactionProfile", "includedFiles", "secretScan", "expiresAt"},
		Validate:     validateDiagnosticsPayload,
	},
	tuple("logs"): {
		Tuple:        tuple("logs"),
		SchemaPath:   "contracts/backend-payloads/v1/logs.schema.json",
		SchemaID:     "https://loxilb.io/schemas/appliance-backend/v1/logs.schema.json",
		RequiredKeys: []string{"schemaVersion", "command", "component", "redacted", "window", "entries", "truncated", "observedAt"},
		Validate:     validateLogsPayload,
	},
	tuple("backup key-create"): {
		Tuple:        tuple("backup key-create"),
		SchemaPath:   "contracts/backend-payloads/v1/backup-key-create.schema.json",
		SchemaID:     "https://loxilb.io/schemas/appliance-backend/v1/backup-key-create.schema.json",
		RequiredKeys: []string{"schemaVersion", "command", "result", "operationId", "keyPath", "keyId", "fingerprint", "mode"},
		Validate:     validateBackupKeyPayload,
	},
	tuple("backup create"): {
		Tuple:        tuple("backup create"),
		SchemaPath:   "contracts/backend-payloads/v1/backup-create.schema.json",
		SchemaID:     "https://loxilb.io/schemas/appliance-backend/v1/backup-create.schema.json",
		RequiredKeys: []string{"schemaVersion", "command", "result", "operationId", "archivePath", "sha256", "sizeBytes", "encrypted", "manifestSchema", "componentChecksums", "consistency"},
		Validate:     validateBackupCreatePayload,
	},
	tuple("backup verify"): {
		Tuple:        tuple("backup verify"),
		SchemaPath:   "contracts/backend-payloads/v1/backup-verify.schema.json",
		SchemaID:     "https://loxilb.io/schemas/appliance-backend/v1/backup-verify.schema.json",
		RequiredKeys: []string{"schemaVersion", "command", "result", "archivePath", "authenticated", "checksumValid", "keyMatch", "manifestSchema", "releaseCompatibility", "componentChecksums", "observedAt"},
		Validate:     validateBackupVerifyPayload,
	},
}

// RegisteredPayloadTuples returns a stable, canonical-order copy of the
// approved registry for parity and composition checks.
func RegisteredPayloadTuples() []PayloadTuple {
	result := make([]PayloadTuple, 0, len(payloadRegistry))
	for tuple := range payloadRegistry {
		result = append(result, tuple)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ContractMajor != result[j].ContractMajor {
			return result[i].ContractMajor < result[j].ContractMajor
		}
		if result[i].SchemaVersion != result[j].SchemaVersion {
			return result[i].SchemaVersion < result[j].SchemaVersion
		}
		return result[i].Command < result[j].Command
	})
	return result
}

// ValidatePayload accepts exactly one object for one registered tuple. It
// rejects structural and semantic drift before returning any document bytes.
func ValidatePayload(selected PayloadTuple, raw []byte) (*ValidatedPayload, error) {
	spec, ok := payloadRegistry[selected]
	if !ok {
		return nil, payloadError(selected, selected.Command, "unsupported contract/schema/command tuple")
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return nil, payloadError(selected, selected.Command, err.Error())
	}
	document, err := decodeExactOne[map[string]json.RawMessage](raw)
	if err != nil {
		return nil, payloadError(selected, selected.Command, fmt.Sprintf("document is not one exact JSON object: %v", err))
	}
	var generic any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&generic); err != nil {
		return nil, payloadError(selected, selected.Command, err.Error())
	}
	if err := inspectPayloadValue(generic); err != nil {
		return nil, payloadError(selected, observedCommand(*document, selected.Command), err.Error())
	}
	command := observedCommand(*document, selected.Command)
	if err := exactStringField(*document, "schemaVersion", selected.SchemaVersion); err != nil {
		return nil, payloadError(selected, command, err.Error())
	}
	if err := exactStringField(*document, "command", selected.Command); err != nil {
		return nil, payloadError(selected, command, err.Error())
	}
	if isOperationErrorDocument(*document) {
		operationError, err := validateOperationErrorPayload(*document, raw)
		if err != nil {
			return nil, payloadError(selected, command, err.Error())
		}
		return &ValidatedPayload{
			Tuple:          selected,
			Command:        command,
			Document:       append(json.RawMessage(nil), raw...),
			OperationID:    operationError.OperationID,
			OperationError: operationError,
		}, nil
	}
	if err := validateRequiredKeySet(*document, spec.RequiredKeys); err != nil {
		return nil, payloadError(selected, command, err.Error())
	}
	if err := spec.Validate(raw); err != nil {
		return nil, payloadError(selected, command, err.Error())
	}
	operationID := optionalStringField(*document, "operationId")
	return &ValidatedPayload{
		Tuple:       selected,
		Command:     command,
		Document:    append(json.RawMessage(nil), raw...),
		OperationID: operationID,
	}, nil
}

var operationErrorRequiredKeys = []string{
	"schemaVersion", "command", "result", "exit", "code", "origin", "componentCode", "retryable",
}

var operationErrorOptionalKeys = map[string]struct{}{
	"message": {}, "correlationId": {}, "operationId": {}, "recoveryGuidance": {},
}

func isOperationErrorDocument(document map[string]json.RawMessage) bool {
	for _, key := range []string{"exit", "code", "origin", "componentCode", "retryable", "recoveryGuidance"} {
		if _, ok := document[key]; ok {
			return true
		}
	}
	return optionalStringField(document, "result") == "ERROR"
}

func validateOperationErrorPayload(document map[string]json.RawMessage, raw []byte) (*BackendOperationError, error) {
	if err := validateRequiredAndAllowedKeySet(document, operationErrorRequiredKeys, operationErrorOptionalKeys); err != nil {
		return nil, err
	}
	doc, err := decodePayload[BackendOperationError](raw)
	if err != nil {
		return nil, err
	}
	expected := map[int]struct {
		result    string
		code      string
		retryable bool
	}{
		2: {"ERROR", "INVALID_ARGUMENT", false},
		3: {"ERROR", "AUTH", false},
		4: {"ERROR", "PRECONDITION", false},
		5: {"ERROR", "UNAVAILABLE", true},
		6: {"ERROR", "CONTRACT_MISMATCH", false},
		7: {"ERROR", "FAILED", false},
		8: {"PARTIAL", "PARTIAL", false},
	}
	tuple, ok := expected[doc.Exit]
	if !ok || doc.Result != tuple.result || doc.Code != tuple.code || doc.Retryable != tuple.retryable {
		return nil, errors.New("operation error result/exit/code/retryable tuple is invalid")
	}
	if doc.Origin != "backend" || !upperCodeShape.MatchString(doc.ComponentCode) || len(doc.ComponentCode) > 128 {
		return nil, errors.New("operation error origin or componentCode is invalid")
	}
	for field, value := range map[string]string{
		"message": doc.Message, "recoveryGuidance": doc.RecoveryGuidance,
	} {
		if value != "" && (len(value) > 512 || containsControl(value)) {
			return nil, fmt.Errorf("operation error field %q is invalid", field)
		}
	}
	for field, value := range map[string]string{
		"correlationId": doc.CorrelationID, "operationId": doc.OperationID,
	} {
		if value != "" && !identifierShape.MatchString(value) {
			return nil, fmt.Errorf("operation error field %q is invalid", field)
		}
	}
	if doc.Exit == 8 && (doc.OperationID == "" || doc.RecoveryGuidance == "") {
		return nil, errors.New("PARTIAL requires operationId and recoveryGuidance")
	}
	return doc, nil
}

func payloadError(tuple PayloadTuple, command, reason string) *PayloadValidationError {
	return &PayloadValidationError{Tuple: tuple, Command: command, Reason: reason}
}

func validateRequiredKeySet(document map[string]json.RawMessage, required []string) error {
	if len(document) != len(required) {
		return fmt.Errorf("object keys differ from the exact required set")
	}
	for _, key := range required {
		if _, ok := document[key]; !ok {
			return fmt.Errorf("required field %q is missing", key)
		}
	}
	return nil
}

func validateRequiredAndAllowedKeySet(document map[string]json.RawMessage, required []string, optional map[string]struct{}) error {
	for _, key := range required {
		if _, ok := document[key]; !ok {
			return fmt.Errorf("required field %q is missing", key)
		}
	}
	for key := range document {
		allowed := false
		for _, requiredKey := range required {
			if key == requiredKey {
				allowed = true
				break
			}
		}
		if _, ok := optional[key]; ok {
			allowed = true
		}
		if !allowed {
			return fmt.Errorf("unknown field %q", key)
		}
	}
	return nil
}

func observedCommand(document map[string]json.RawMessage, fallback string) string {
	var command string
	if raw, ok := document["command"]; ok && json.Unmarshal(raw, &command) == nil &&
		commandNameShape.MatchString(command) && len(command) <= 128 {
		return command
	}
	return fallback
}

func exactStringField(document map[string]json.RawMessage, field, expected string) error {
	var actual string
	if err := json.Unmarshal(document[field], &actual); err != nil {
		return fmt.Errorf("field %q is not a string", field)
	}
	if actual != expected {
		return fmt.Errorf("field %q does not match the approved value", field)
	}
	return nil
}

func optionalStringField(document map[string]json.RawMessage, field string) string {
	var value string
	_ = json.Unmarshal(document[field], &value)
	return value
}

var prohibitedPayloadKeys = map[string]struct{}{
	"password": {}, "token": {}, "apikey": {}, "privatekey": {}, "secret": {},
	"credential": {}, "credentials": {},
}

func isProhibitedPayloadKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
	if normalized == "secretscan" {
		return false
	}
	if _, prohibited := prohibitedPayloadKeys[normalized]; prohibited {
		return true
	}
	for _, fragment := range []string{"password", "token", "apikey", "privatekey", "credential"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func inspectPayloadValue(value any) error {
	switch typed := value.(type) {
	case nil:
		return errors.New("null values are prohibited")
	case map[string]any:
		for key, child := range typed {
			if isProhibitedPayloadKey(key) {
				return fmt.Errorf("secret-bearing key %q is prohibited", key)
			}
			if err := inspectPayloadValue(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := inspectPayloadValue(child); err != nil {
				return err
			}
		}
	case string:
		if strings.Contains(typed, "CLI_WP00_SYNTHETIC_SECRET_DO_NOT_EMIT") || credentialValueShape.MatchString(typed) {
			return errors.New("payload contains prohibited credential material")
		}
	}
	return nil
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("object key is not a string")
				}
				if _, duplicate := seen[key]; duplicate {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return errors.New("unexpected closing JSON delimiter")
		}
	}
	if err := walk(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("document contains a second JSON value")
		}
		return fmt.Errorf("document has trailing content: %w", err)
	}
	return nil
}

func decodePayload[T any](raw []byte) (*T, error) {
	value, err := decodeExactOne[T](raw)
	if err != nil {
		return nil, fmt.Errorf("typed payload decode failed: %w", err)
	}
	return value, nil
}

var (
	upperCodeShape  = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	identifierShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	durationShape   = regexp.MustCompile(`^[1-9][0-9]*(s|m|h)$`)
	digestShape     = regexp.MustCompile(`^[0-9a-f]{64}$`)
	mapKeyShape     = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

func containsControl(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func nonEmpty(value, field string) error {
	if value == "" {
		return fmt.Errorf("field %q must be non-empty", field)
	}
	return nil
}

func utcTimestamp(value, field string) error {
	if !strings.HasSuffix(value, "Z") {
		return fmt.Errorf("field %q is not a UTC timestamp", field)
	}
	if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
		return fmt.Errorf("field %q is not RFC3339: %v", field, err)
	}
	return nil
}

func absolutePath(value, field string) error {
	if len(value) < 2 || !strings.HasPrefix(value, "/") || strings.ContainsRune(value, 0) {
		return fmt.Errorf("field %q is not an absolute non-NUL path", field)
	}
	return nil
}

func ipv4(value, field string) error {
	parsed := net.ParseIP(value)
	if parsed == nil || parsed.To4() == nil || strings.Contains(value, ":") {
		return fmt.Errorf("field %q is not IPv4", field)
	}
	return nil
}

func httpsURI(value, field string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("field %q is not an absolute HTTPS URI", field)
	}
	return nil
}

func uniqueNonEmpty(values []string, field string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if value == "" {
			return fmt.Errorf("field %q contains an empty string", field)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("field %q contains duplicate %q", field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

type statusPayload struct {
	SchemaVersion  string `json:"schemaVersion"`
	Command        string `json:"command"`
	OverallStatus  string `json:"overallStatus"`
	ProductRelease string `json:"productRelease"`
	Initialized    bool   `json:"initialized"`
	Planes         []struct {
		Name       string `json:"name"`
		Live       bool   `json:"live"`
		Ready      bool   `json:"ready"`
		ReasonCode string `json:"reasonCode"`
	} `json:"planes"`
	NetworkProfile struct {
		Name       string `json:"name"`
		Configured bool   `json:"configured"`
	} `json:"networkProfile"`
	ActiveOperations         []string `json:"activeOperations"`
	LocalGatewayRegistration struct {
		Registered     bool   `json:"registered"`
		InstallationID string `json:"installationId"`
		InstanceID     string `json:"instanceId"`
	} `json:"localGatewayRegistration"`
	ObservedAt string `json:"observedAt"`
}

func validateStatusPayload(raw []byte) error {
	doc, err := decodePayload[statusPayload](raw)
	if err != nil {
		return err
	}
	if !oneOf(doc.OverallStatus, "READY", "DEGRADED", "NOT_READY", "UNKNOWN") {
		return errors.New("overallStatus is outside the approved enum")
	}
	if err := nonEmpty(doc.ProductRelease, "productRelease"); err != nil {
		return err
	}
	if len(doc.Planes) == 0 {
		return errors.New("planes must contain at least one item")
	}
	for _, plane := range doc.Planes {
		if err := nonEmpty(plane.Name, "planes.name"); err != nil {
			return err
		}
		if !upperCodeShape.MatchString(plane.ReasonCode) {
			return errors.New("planes.reasonCode is malformed")
		}
	}
	if err := nonEmpty(doc.NetworkProfile.Name, "networkProfile.name"); err != nil {
		return err
	}
	if err := uniqueNonEmpty(doc.ActiveOperations, "activeOperations"); err != nil {
		return err
	}
	return utcTimestamp(doc.ObservedAt, "observedAt")
}

type networkPayload struct {
	SchemaVersion string `json:"schemaVersion"`
	Command       string `json:"command"`
	Valid         bool   `json:"valid"`
	Profile       string `json:"profile"`
	Interfaces    []struct {
		Role    string `json:"role"`
		Name    string `json:"name"`
		Exists  bool   `json:"exists"`
		Address string `json:"address"`
		MTU     int    `json:"mtu"`
	} `json:"interfaces"`
	Errors     []networkFinding `json:"errors"`
	Warnings   []networkFinding `json:"warnings"`
	ObservedAt string           `json:"observedAt"`
}

type networkFinding struct {
	Code        string `json:"code"`
	Remediation string `json:"remediation"`
}

func validateNetworkPayload(raw []byte) error {
	doc, err := decodePayload[networkPayload](raw)
	if err != nil {
		return err
	}
	if err := nonEmpty(doc.Profile, "profile"); err != nil {
		return err
	}
	for _, iface := range doc.Interfaces {
		if !oneOf(iface.Role, "frontend", "backend", "management") || iface.Name == "" || iface.MTU < 576 || iface.MTU > 9216 {
			return errors.New("interfaces contains invalid role, name, or mtu")
		}
	}
	for _, finding := range append(append([]networkFinding(nil), doc.Errors...), doc.Warnings...) {
		if !upperCodeShape.MatchString(finding.Code) || finding.Remediation == "" {
			return errors.New("network finding is malformed")
		}
	}
	return utcTimestamp(doc.ObservedAt, "observedAt")
}

type publicAddressPayload struct {
	SchemaVersion           string `json:"schemaVersion"`
	Command                 string `json:"command"`
	Result                  string `json:"result"`
	OperationID             string `json:"operationId"`
	PublicAddress           string `json:"publicAddress"`
	PrivateAddressPreserved bool   `json:"privateAddressPreserved"`
	Certificate             struct {
		FingerprintSHA256 string   `json:"fingerprintSha256"`
		SANs              []string `json:"sans"`
	} `json:"certificate"`
	Restart   string `json:"restart"`
	Readiness string `json:"readiness"`
}

func validatePublicAddressPayload(raw []byte) error {
	doc, err := decodePayload[publicAddressPayload](raw)
	if err != nil {
		return err
	}
	if !oneOf(doc.Result, "APPLIED", "UNCHANGED", "PENDING_RESTART") || doc.OperationID == "" || !doc.PrivateAddressPreserved {
		return errors.New("public-address result, operationId, or preservation flag is invalid")
	}
	if err := ipv4(doc.PublicAddress, "publicAddress"); err != nil {
		return err
	}
	if !digestShape.MatchString(doc.Certificate.FingerprintSHA256) || len(doc.Certificate.SANs) == 0 {
		return errors.New("certificate fingerprint or SAN set is invalid")
	}
	if err := uniqueNonEmpty(doc.Certificate.SANs, "certificate.sans"); err != nil {
		return err
	}
	if !oneOf(doc.Restart, "RECONCILED", "PENDING") || !oneOf(doc.Readiness, "READY", "NOT_READY", "PENDING_RESTART") {
		return errors.New("restart or readiness is outside the approved enum")
	}
	if doc.Result == "PENDING_RESTART" && (doc.Restart != "PENDING" || doc.Readiness != "PENDING_RESTART") {
		return errors.New("PENDING_RESTART requires pending restart and readiness")
	}
	return nil
}

type gatewayRegistrationPayload struct {
	SchemaVersion   string `json:"schemaVersion"`
	Command         string `json:"command"`
	Result          string `json:"result"`
	OperationID     string `json:"operationId"`
	InstallationID  string `json:"installationId"`
	InstanceID      string `json:"instanceId"`
	Endpoint        string `json:"endpoint"`
	GatewayIdentity struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	} `json:"gatewayIdentity"`
	Verified      bool `json:"verified"`
	MarkerUpdated bool `json:"markerUpdated"`
}

func validateGatewayRegistrationPayload(raw []byte) error {
	doc, err := decodePayload[gatewayRegistrationPayload](raw)
	if err != nil {
		return err
	}
	if !oneOf(doc.Result, "CREATED", "UPDATED", "UNCHANGED") || doc.OperationID == "" || doc.InstallationID == "" || doc.InstanceID == "" || !doc.Verified {
		return errors.New("gateway registration identity or result is invalid")
	}
	if err := httpsURI(doc.Endpoint, "endpoint"); err != nil {
		return err
	}
	if doc.GatewayIdentity.Name == "" {
		return errors.New("gatewayIdentity.name must be non-empty")
	}
	return ipv4(doc.GatewayIdentity.Address, "gatewayIdentity.address")
}

type diagnosticsPayload struct {
	SchemaVersion    string   `json:"schemaVersion"`
	Command          string   `json:"command"`
	Result           string   `json:"result"`
	OperationID      string   `json:"operationId"`
	ArchivePath      string   `json:"archivePath"`
	SHA256           string   `json:"sha256"`
	SizeBytes        int64    `json:"sizeBytes"`
	Mode             string   `json:"mode"`
	RedactionProfile string   `json:"redactionProfile"`
	IncludedFiles    []string `json:"includedFiles"`
	SecretScan       struct {
		Result string `json:"result"`
		Hits   int    `json:"hits"`
	} `json:"secretScan"`
	ExpiresAt string `json:"expiresAt"`
}

func validateDiagnosticsPayload(raw []byte) error {
	doc, err := decodePayload[diagnosticsPayload](raw)
	if err != nil {
		return err
	}
	if doc.Result != "CREATED" || doc.OperationID == "" || !digestShape.MatchString(doc.SHA256) || doc.SizeBytes < 1 || doc.Mode != "0600" || doc.RedactionProfile == "" {
		return errors.New("diagnostics result metadata is invalid")
	}
	if err := absolutePath(doc.ArchivePath, "archivePath"); err != nil {
		return err
	}
	if err := uniqueNonEmpty(doc.IncludedFiles, "includedFiles"); err != nil {
		return err
	}
	for _, file := range doc.IncludedFiles {
		if err := absolutePath(file, "includedFiles"); err != nil {
			return err
		}
	}
	if doc.SecretScan.Result != "PASS" || doc.SecretScan.Hits != 0 {
		return errors.New("secretScan must report PASS with zero hits")
	}
	return utcTimestamp(doc.ExpiresAt, "expiresAt")
}

type logsPayload struct {
	SchemaVersion string `json:"schemaVersion"`
	Command       string `json:"command"`
	Component     string `json:"component"`
	Redacted      bool   `json:"redacted"`
	Window        struct {
		Since string `json:"since"`
		Lines int    `json:"lines"`
	} `json:"window"`
	Entries []struct {
		Timestamp                string `json:"timestamp"`
		Severity                 string `json:"severity"`
		CorrelationOrOperationID string `json:"correlationOrOperationId"`
		Message                  string `json:"message"`
	} `json:"entries"`
	Truncated  bool   `json:"truncated"`
	ObservedAt string `json:"observedAt"`
}

func validateLogsPayload(raw []byte) error {
	doc, err := decodePayload[logsPayload](raw)
	if err != nil {
		return err
	}
	if !oneOf(doc.Component, "gateway", "oam", "ui", "caddy", "postgres", "state", "dataplane", "management") || !doc.Redacted {
		return errors.New("logs component or redacted flag is invalid")
	}
	if !durationShape.MatchString(doc.Window.Since) || doc.Window.Lines < 1 || doc.Window.Lines > 10000 {
		return errors.New("logs window is invalid")
	}
	for _, entry := range doc.Entries {
		if err := utcTimestamp(entry.Timestamp, "entries.timestamp"); err != nil {
			return err
		}
		if !oneOf(entry.Severity, "DEBUG", "INFO", "WARN", "ERROR") {
			return errors.New("entries.severity is outside the approved enum")
		}
	}
	return utcTimestamp(doc.ObservedAt, "observedAt")
}

type backupKeyPayload struct {
	SchemaVersion string `json:"schemaVersion"`
	Command       string `json:"command"`
	Result        string `json:"result"`
	OperationID   string `json:"operationId"`
	KeyPath       string `json:"keyPath"`
	KeyID         string `json:"keyId"`
	Fingerprint   string `json:"fingerprint"`
	Mode          string `json:"mode"`
}

func validateBackupKeyPayload(raw []byte) error {
	doc, err := decodePayload[backupKeyPayload](raw)
	if err != nil {
		return err
	}
	if doc.Result != "CREATED" || doc.OperationID == "" || doc.KeyID == "" || !digestShape.MatchString(doc.Fingerprint) || doc.Mode != "0600" {
		return errors.New("backup key result metadata is invalid")
	}
	return absolutePath(doc.KeyPath, "keyPath")
}

type backupCreatePayload struct {
	SchemaVersion      string            `json:"schemaVersion"`
	Command            string            `json:"command"`
	Result             string            `json:"result"`
	OperationID        string            `json:"operationId"`
	ArchivePath        string            `json:"archivePath"`
	SHA256             string            `json:"sha256"`
	SizeBytes          int64             `json:"sizeBytes"`
	Encrypted          bool              `json:"encrypted"`
	ManifestSchema     string            `json:"manifestSchema"`
	ComponentChecksums map[string]string `json:"componentChecksums"`
	Consistency        string            `json:"consistency"`
}

func validateBackupCreatePayload(raw []byte) error {
	doc, err := decodePayload[backupCreatePayload](raw)
	if err != nil {
		return err
	}
	if !oneOf(doc.Result, "CREATED", "PARTIAL") || doc.OperationID == "" || !digestShape.MatchString(doc.SHA256) || doc.SizeBytes < 1 || !doc.Encrypted || doc.ManifestSchema == "" {
		return errors.New("backup create result metadata is invalid")
	}
	if err := absolutePath(doc.ArchivePath, "archivePath"); err != nil {
		return err
	}
	if err := validateChecksums(doc.ComponentChecksums); err != nil {
		return err
	}
	if (doc.Result == "CREATED" && doc.Consistency != "APPLICATION_CONSISTENT") || (doc.Result == "PARTIAL" && doc.Consistency != "INCOMPLETE") {
		return errors.New("backup result and consistency disagree")
	}
	return nil
}

type backupVerifyPayload struct {
	SchemaVersion        string            `json:"schemaVersion"`
	Command              string            `json:"command"`
	Result               string            `json:"result"`
	ArchivePath          string            `json:"archivePath"`
	Authenticated        bool              `json:"authenticated"`
	ChecksumValid        bool              `json:"checksumValid"`
	KeyMatch             bool              `json:"keyMatch"`
	ManifestSchema       string            `json:"manifestSchema"`
	ReleaseCompatibility string            `json:"releaseCompatibility"`
	ComponentChecksums   map[string]string `json:"componentChecksums"`
	ObservedAt           string            `json:"observedAt"`
}

func validateBackupVerifyPayload(raw []byte) error {
	doc, err := decodePayload[backupVerifyPayload](raw)
	if err != nil {
		return err
	}
	if doc.Result != "VERIFIED" || !doc.Authenticated || !doc.ChecksumValid || !doc.KeyMatch || doc.ManifestSchema == "" || doc.ReleaseCompatibility != "COMPATIBLE" {
		return errors.New("backup verification result is invalid")
	}
	if err := absolutePath(doc.ArchivePath, "archivePath"); err != nil {
		return err
	}
	if err := validateChecksums(doc.ComponentChecksums); err != nil {
		return err
	}
	return utcTimestamp(doc.ObservedAt, "observedAt")
}

func validateChecksums(checksums map[string]string) error {
	if len(checksums) == 0 {
		return errors.New("componentChecksums must not be empty")
	}
	for component, digest := range checksums {
		if !mapKeyShape.MatchString(component) || !digestShape.MatchString(digest) {
			return errors.New("componentChecksums contains an invalid key or digest")
		}
	}
	return nil
}
