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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
)

const (
	approvedCPCLIAggregate             = "sha256:eb6c0d40639ce4a73726101ffc27e378c755c308056708c03972d48cbbbb50ea"
	approvedFixtureDigest              = "sha256:8bd40ec15ee013e00611d2c6ba6bbcd11f8e1aa3308b27e602fb20dfa6e47acc"
	approvedSelectorDigest             = "sha256:a51a342515460a3c22091b7fc23374b055ce093efd18eb6f0153f78b009671b0"
	approvedSchemaSetDigest            = "sha256:67837a0262cebc43a159c7d3a2b60190e04a8ca282629549f73daa9e06103af1"
	approvedOperationErrorSchemaDigest = "sha256:137b12626e5736a9b2da71730dc8221acde06731b927a16d720c7ec18d9615cc"
)

type wp02PositiveFixture struct {
	ID       string         `json:"id"`
	Command  string         `json:"command"`
	Document map[string]any `json:"document"`
}

type wp02Fixtures struct {
	SchemaVersion           string `json:"schemaVersion"`
	SyntheticSecretSentinel string `json:"syntheticSecretSentinel"`
	Payloads                struct {
		Positive                []wp02PositiveFixture `json:"positive"`
		NegativeMutationClasses []struct {
			Name string `json:"name"`
		} `json:"negativeMutationClasses"`
		NegativeFixtureExpansion struct {
			Cardinality int `json:"cardinality"`
		} `json:"negativeFixtureExpansion"`
	} `json:"payloads"`
	OperationErrors struct {
		Positive []wp02PositiveFixture `json:"positive"`
		Negative []wp02MutationFixture `json:"negative"`
	} `json:"operationErrors"`
	StatusContract struct {
		Positive []wp02PositiveFixture `json:"positive"`
		Negative []wp02MutationFixture `json:"negative"`
	} `json:"statusContract"`
}

type wp02MutationFixture struct {
	ID        string `json:"id"`
	Twin      string `json:"twin"`
	Operation string `json:"operation"`
	Path      string `json:"path"`
	Value     any    `json:"value"`
	ValueFrom string `json:"valueFrom"`
}

type wp02Manifest struct {
	SchemaVersion      string `json:"schemaVersion"`
	VerificationStatus string `json:"verificationStatus"`
	ApprovalInputs     struct {
		CLIA0 string `json:"cliA0ApprovalSha256"`
	} `json:"approvalInputs"`
	CLISource struct {
		BaseRevision string `json:"baseRevision"`
	} `json:"cliSource"`
	Contract struct {
		FixtureSHA256              string         `json:"fixtureSha256"`
		SelectorSHA256             string         `json:"selectorSha256"`
		SelectorCaseCount          int            `json:"selectorCaseCount"`
		FixtureCounts              map[string]int `json:"fixtureCounts"`
		PayloadSchemaSetSHA256     string         `json:"payloadSchemaSetSha256"`
		OperationErrorSchemaSHA256 string         `json:"operationErrorSchemaSha256"`
	} `json:"contract"`
	Bundle struct {
		AggregateSHA256 string `json:"aggregateSha256"`
		Files           []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	} `json:"bundle"`
}

func wp02RepoRoot(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
}

func loadWP02Fixtures(t *testing.T) (wp02Fixtures, []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(wp02RepoRoot(t), "testdata/backend-contract/fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures wp02Fixtures
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	return fixtures, raw
}

func loadWP02Manifest(t *testing.T) wp02Manifest {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(wp02RepoRoot(t), "testdata/backend-contract/cli-contract-candidate.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest wp02Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func payloadSchemaPaths(root string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(root, "contracts/backend-payloads/v1/*.schema.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func digestBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func manifestDigestMap(manifest wp02Manifest) map[string]string {
	result := make(map[string]string, len(manifest.Bundle.Files))
	for _, file := range manifest.Bundle.Files {
		result[file.Path] = file.SHA256
	}
	return result
}

func calculatedSchemaSetDigest(root string, paths []string) (string, error) {
	h := sha256.New()
	for _, absolute := range paths {
		relative, err := filepath.Rel(root, absolute)
		if err != nil {
			return "", err
		}
		raw, err := os.ReadFile(absolute)
		if err != nil {
			return "", err
		}
		_, _ = h.Write([]byte(filepath.ToSlash(relative)))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(digestBytes(raw)))
		_, _ = h.Write([]byte{'\n'})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func validateWP02Manifest(root string, manifest wp02Manifest, fixtureRaw []byte) error {
	if manifest.SchemaVersion != "cli-appliance-contract-candidate/v1" || manifest.VerificationStatus != "DEVELOPMENT_UNVERIFIED" {
		return errors.New("manifest identity is invalid")
	}
	if manifest.CLISource.BaseRevision != "847055d5b04c65aa574bb815bf61f7eab235af4d" {
		return errors.New("CLI source identity differs")
	}
	if manifest.ApprovalInputs.CLIA0 != "sha256:a6a71470e77b4487598679d45255830e4e64a2b51064a808b741a1de93ab7a1f" {
		return errors.New("approval identity differs")
	}
	if manifest.Bundle.AggregateSHA256 != approvedCPCLIAggregate || manifest.Contract.FixtureSHA256 != approvedFixtureDigest ||
		manifest.Contract.SelectorSHA256 != approvedSelectorDigest || manifest.Contract.SelectorCaseCount != 143 ||
		manifest.Contract.PayloadSchemaSetSHA256 != approvedSchemaSetDigest ||
		manifest.Contract.OperationErrorSchemaSHA256 != approvedOperationErrorSchemaDigest {
		return errors.New("approved bundle or fixture identity differs")
	}
	wantCounts := map[string]int{"payloadPositive": 9, "payloadNegative": 72, "statusContract": 9, "backendContractError": 10, "operationError": 18, "operationReceipt": 9}
	if len(manifest.Contract.FixtureCounts) != len(wantCounts) {
		return errors.New("fixture count key set differs")
	}
	for name, want := range wantCounts {
		if manifest.Contract.FixtureCounts[name] != want {
			return fmt.Errorf("fixture count %q differs", name)
		}
	}
	if digestBytes(fixtureRaw) != manifest.Contract.FixtureSHA256 {
		return errors.New("fixture bytes differ from manifest")
	}
	paths, err := payloadSchemaPaths(root)
	if err != nil || len(paths) != 9 {
		return fmt.Errorf("schema path set: %w", err)
	}
	recorded := manifestDigestMap(manifest)
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if recorded[filepath.ToSlash(relative)] != digestBytes(raw) {
			return fmt.Errorf("schema digest differs: %s", relative)
		}
	}
	setDigest, err := calculatedSchemaSetDigest(root, paths)
	if err != nil {
		return err
	}
	if setDigest != approvedSchemaSetDigest {
		return fmt.Errorf("schema set digest = %s", setDigest)
	}
	operationErrorRaw, err := os.ReadFile(filepath.Join(root, "contracts/backend-errors/v1/backend-operation-error.schema.json"))
	if err != nil {
		return err
	}
	if digestBytes(operationErrorRaw) != manifest.Contract.OperationErrorSchemaSHA256 {
		return errors.New("operation error schema digest differs")
	}
	return nil
}

func TestCLIWP02ManifestPreservesApprovedSchemaAndFixtureDigests(t *testing.T) {
	_, fixtureRaw := loadWP02Fixtures(t)
	if err := validateWP02Manifest(wp02RepoRoot(t), loadWP02Manifest(t), fixtureRaw); err != nil {
		t.Fatal(err)
	}
}

func TestCLIWP02ManifestDetectsIdentityMutants(t *testing.T) {
	_, fixtureRaw := loadWP02Fixtures(t)
	for name, mutate := range map[string]func(*wp02Manifest){
		"wrong CLI source":     func(m *wp02Manifest) { m.CLISource.BaseRevision = strings.Repeat("0", 40) },
		"wrong approval":       func(m *wp02Manifest) { m.ApprovalInputs.CLIA0 = "sha256:" + strings.Repeat("0", 64) },
		"wrong aggregate":      func(m *wp02Manifest) { m.Bundle.AggregateSHA256 = "sha256:" + strings.Repeat("0", 64) },
		"wrong fixture":        func(m *wp02Manifest) { m.Contract.FixtureSHA256 = "sha256:" + strings.Repeat("0", 64) },
		"wrong selector":       func(m *wp02Manifest) { m.Contract.SelectorSHA256 = "sha256:" + strings.Repeat("0", 64) },
		"wrong selector count": func(m *wp02Manifest) { m.Contract.SelectorCaseCount-- },
		"wrong fixture count":  func(m *wp02Manifest) { m.Contract.FixtureCounts["statusContract"]-- },
		"wrong schema set":     func(m *wp02Manifest) { m.Contract.PayloadSchemaSetSHA256 = "sha256:" + strings.Repeat("0", 64) },
		"wrong operation error schema": func(m *wp02Manifest) {
			m.Contract.OperationErrorSchemaSHA256 = "sha256:" + strings.Repeat("0", 64)
		},
		"wrong schema": func(m *wp02Manifest) {
			for i := range m.Bundle.Files {
				if strings.HasSuffix(m.Bundle.Files[i].Path, "status.schema.json") {
					m.Bundle.Files[i].SHA256 = "sha256:" + strings.Repeat("0", 64)
				}
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			manifest := loadWP02Manifest(t)
			mutate(&manifest)
			if validateWP02Manifest(wp02RepoRoot(t), manifest, fixtureRaw) == nil {
				t.Fatal("identity mutant survived")
			}
		})
	}
}

func tupleForCommand(command string) PayloadTuple {
	return PayloadTuple{ContractMajor: 1, SchemaVersion: payloadSchema, Command: command}
}

func fixtureRaw(t *testing.T, fixture wp02PositiveFixture) []byte {
	t.Helper()
	raw, err := json.Marshal(fixture.Document)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func requirePayloadValidationError(t *testing.T, err error) *PayloadValidationError {
	t.Helper()
	var typed *PayloadValidationError
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *PayloadValidationError (%v)", err, err)
	}
	classified := exitcode.Classify(err)
	if classified.Code != exitcode.ContractMismatch || classified.ComponentCode != CodeBackendPayloadInvalid {
		t.Fatalf("classification = %+v", classified)
	}
	return typed
}

func TestCLIWP02DecoderAcceptsExactPositiveDocument(t *testing.T) {
	fixtures, _ := loadWP02Fixtures(t)
	fixture := fixtures.Payloads.Positive[0]
	validated, err := ValidatePayload(tupleForCommand(fixture.Command), fixtureRaw(t, fixture))
	if err != nil {
		t.Fatal(err)
	}
	if validated.Command != fixture.Command || !json.Valid(validated.Document) {
		t.Fatalf("validated payload lost identity: %+v", validated)
	}
}

func TestCLIWP02DecoderRejectsStructuralMutants(t *testing.T) {
	fixtures, _ := loadWP02Fixtures(t)
	fixture := fixtures.Payloads.Positive[0]
	valid := fixtureRaw(t, fixture)
	mutants := map[string][]byte{
		"prefix prose":        append([]byte("prefix "), valid...),
		"suffix prose":        append(append([]byte(nil), valid...), []byte(" suffix")...),
		"second document":     append(append([]byte(nil), valid...), []byte(`{}`)...),
		"null document":       []byte(`null`),
		"array document":      []byte(`[]`),
		"nested envelope":     []byte(`{"apiVersion":"loxilb.io/appliance/v1","kind":"CommandResult","data":{}}`),
		"duplicate command":   []byte(`{"schemaVersion":"appliance-backend-payload/v1","command":"status","command":"status"}`),
		"unknown tuple major": valid,
	}
	for name, raw := range mutants {
		t.Run(name, func(t *testing.T) {
			tuple := tupleForCommand(fixture.Command)
			if name == "unknown tuple major" {
				tuple.ContractMajor = 2
			}
			_, err := ValidatePayload(tuple, raw)
			requirePayloadValidationError(t, err)
		})
	}
}

func TestCLIWP02DecoderPreservesTypedEvidence(t *testing.T) {
	tuple := tupleForCommand("status")
	_, err := ValidatePayload(tuple, []byte(`{"schemaVersion":"appliance-backend-payload/v1","command":"wrong"}`))
	typed := requirePayloadValidationError(t, err)
	withEvidence := typed.WithExecutionEvidence(8, "cli-correlation-01", "op-01")
	if withEvidence.Tuple != tuple || withEvidence.Command != "wrong" || withEvidence.ObservedBackendExit == nil ||
		*withEvidence.ObservedBackendExit != 8 || withEvidence.CorrelationID != "cli-correlation-01" || withEvidence.OperationID != "op-01" {
		t.Fatalf("typed evidence lost: %+v", withEvidence)
	}
	if exitcode.Classify(withEvidence).Code != exitcode.ContractMismatch {
		t.Fatal("execution evidence prematurely changed local classification")
	}
}

func TestCLIWP02ValidatorsAcceptNinePositiveFixtures(t *testing.T) {
	fixtures, _ := loadWP02Fixtures(t)
	if len(fixtures.Payloads.Positive) != 9 {
		t.Fatalf("positive fixtures = %d", len(fixtures.Payloads.Positive))
	}
	seen := map[string]struct{}{}
	for _, fixture := range fixtures.Payloads.Positive {
		t.Run(fixture.ID, func(t *testing.T) {
			validated, err := ValidatePayload(tupleForCommand(fixture.Command), fixtureRaw(t, fixture))
			if err != nil {
				t.Fatal(err)
			}
			if validated.Command != fixture.Command {
				t.Fatalf("command = %q", validated.Command)
			}
		})
		seen[fixture.Command] = struct{}{}
	}
	if len(seen) != 9 {
		t.Fatalf("unique positive commands = %d", len(seen))
	}
}

func statusFixture(t *testing.T, fixtures wp02Fixtures) wp02PositiveFixture {
	t.Helper()
	for _, fixture := range fixtures.Payloads.Positive {
		if fixture.Command == "status" {
			return fixture
		}
	}
	t.Fatal("canonical status fixture is missing")
	return wp02PositiveFixture{}
}

func TestCLIWP05StatusCanonicalFixtureIncludesProductFacts(t *testing.T) {
	fixtures, _ := loadWP02Fixtures(t)
	document := statusFixture(t, fixtures).Document
	if document["reasonCode"] != "DATAPLANE_DEGRADED" {
		t.Fatalf("top-level reasonCode = %#v", document["reasonCode"])
	}
	planeValues, ok := document["planes"].([]any)
	if !ok || len(planeValues) != 3 {
		t.Fatalf("planes = %#v, want exact state/dataplane/management set", document["planes"])
	}
	wantNames := []string{"state", "dataplane", "management"}
	for i, wantName := range wantNames {
		plane, ok := planeValues[i].(map[string]any)
		if !ok || plane["name"] != wantName || plane["status"] == nil || plane["observedAt"] == nil {
			t.Fatalf("plane %d = %#v", i, planeValues[i])
		}
	}
	tls, ok := document["publicAddressTls"].(map[string]any)
	if !ok || tls["configured"] != true || len(tls) != 1 {
		t.Fatalf("publicAddressTls = %#v", document["publicAddressTls"])
	}
}

func TestCLIWP05StatusAcceptsAdditiveOptionalLegacyPayload(t *testing.T) {
	fixtures, _ := loadWP02Fixtures(t)
	if len(fixtures.StatusContract.Positive) != 1 {
		t.Fatalf("status compatibility positives = %d", len(fixtures.StatusContract.Positive))
	}
	fixture := fixtures.StatusContract.Positive[0]
	if _, err := ValidatePayload(tupleForCommand("status"), fixtureRaw(t, fixture)); err != nil {
		t.Fatalf("v1 payload without CLI-WP05 optional fields was rejected: %v", err)
	}
}

func applyStatusMutation(t *testing.T, document map[string]any, mutation wp02MutationFixture, sentinel string) []byte {
	t.Helper()
	value := mutation.Value
	if mutation.ValueFrom == "syntheticSecretSentinel" {
		value = sentinel
	}
	var parent map[string]any
	var key string
	switch mutation.Path {
	case "/planes/0/status":
		parent, key = document["planes"].([]any)[0].(map[string]any), "status"
	case "/planes/0/observedAt":
		parent, key = document["planes"].([]any)[0].(map[string]any), "observedAt"
	case "/reasonCode":
		parent, key = document, "reasonCode"
	case "/publicAddressTls/configured":
		parent, key = document["publicAddressTls"].(map[string]any), "configured"
	case "/publicAddressTls/certificate":
		parent, key = document["publicAddressTls"].(map[string]any), "certificate"
	case "/publicAddressTls/privateKey":
		parent, key = document["publicAddressTls"].(map[string]any), "privateKey"
	default:
		t.Fatalf("unsupported status mutation path %q", mutation.Path)
	}
	if mutation.Operation == "remove" {
		delete(parent, key)
	} else {
		parent[key] = value
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCLIWP05StatusRejectsFieldRedTwins(t *testing.T) {
	fixtures, _ := loadWP02Fixtures(t)
	canonical := statusFixture(t, fixtures)
	if len(fixtures.StatusContract.Negative) != 8 {
		t.Fatalf("status negative red twins = %d", len(fixtures.StatusContract.Negative))
	}
	for _, mutation := range fixtures.StatusContract.Negative {
		t.Run(mutation.ID, func(t *testing.T) {
			if mutation.Twin != canonical.ID {
				t.Fatalf("twin = %q, want %q", mutation.Twin, canonical.ID)
			}
			raw := applyStatusMutation(t, cloneDocument(t, canonical.Document), mutation, fixtures.SyntheticSecretSentinel)
			validated, err := ValidatePayload(tupleForCommand("status"), raw)
			if validated != nil {
				t.Fatalf("status red twin escaped validation: %s", validated.Document)
			}
			requirePayloadValidationError(t, err)
		})
	}
}

var wp02CommandSpecificField = map[string]string{
	"status":                   "overallStatus",
	"network validate":         "valid",
	"public-address configure": "result",
	"gateway register-local":   "result",
	"diagnostics create":       "result",
	"logs":                     "component",
	"backup key-create":        "result",
	"backup create":            "result",
	"backup verify":            "result",
}

func cloneDocument(t *testing.T, document map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func semanticMutant(t *testing.T, fixture wp02PositiveFixture, class, sentinel string) []byte {
	t.Helper()
	doc := cloneDocument(t, fixture.Document)
	field := wp02CommandSpecificField[fixture.Command]
	switch class {
	case "wrong-command":
		doc["command"] = "renamed command"
	case "missing-required":
		delete(doc, field)
	case "unknown-key":
		doc["unexpected"] = true
	case "wrong-type-or-enum":
		doc[field] = map[string]any{"invalid": true}
	case "null-value":
		doc[field] = nil
	case "nested-command-result":
		doc = map[string]any{"apiVersion": "loxilb.io/appliance/v1", "kind": "CommandResult", "data": doc}
	case "secret-bearing-key":
		doc["password"] = sentinel
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if class == "second-json-document" {
		raw = append(raw, []byte(`{}`)...)
	}
	return raw
}

func TestCLIWP02ValidatorsRejectExpandedSemanticMutants(t *testing.T) {
	fixtures, _ := loadWP02Fixtures(t)
	classes := make([]string, 0, len(fixtures.Payloads.NegativeMutationClasses))
	for _, mutation := range fixtures.Payloads.NegativeMutationClasses {
		classes = append(classes, mutation.Name)
	}
	if got := len(fixtures.Payloads.Positive) * len(classes); got != fixtures.Payloads.NegativeFixtureExpansion.Cardinality || got != 72 {
		t.Fatalf("expanded negatives = %d, manifest = %d", got, fixtures.Payloads.NegativeFixtureExpansion.Cardinality)
	}
	for _, fixture := range fixtures.Payloads.Positive {
		for _, class := range classes {
			t.Run(fixture.ID+"/"+class, func(t *testing.T) {
				validated, err := ValidatePayload(tupleForCommand(fixture.Command), semanticMutant(t, fixture, class, fixtures.SyntheticSecretSentinel))
				if validated != nil {
					t.Fatalf("invalid payload escaped as success: %+v", validated)
				}
				typed := requirePayloadValidationError(t, err)
				if typed.Tuple.Command != fixture.Command {
					t.Fatalf("tuple lost: %+v", typed)
				}
			})
		}
	}
}

func TestCLIWP02ValidatorsAcceptSchemaDefinedPartial(t *testing.T) {
	fixtures, _ := loadWP02Fixtures(t)
	var backup wp02PositiveFixture
	for _, fixture := range fixtures.Payloads.Positive {
		if fixture.Command == "backup create" {
			backup = fixture
			break
		}
	}
	if backup.Command == "" {
		t.Fatal("backup create positive fixture is missing")
	}
	document := cloneDocument(t, backup.Document)
	document["result"] = "PARTIAL"
	document["consistency"] = "INCOMPLETE"
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	validated, err := ValidatePayload(tupleForCommand(backup.Command), raw)
	if err != nil || validated == nil || validated.OperationID == "" {
		t.Fatalf("schema-defined partial rejected or lost evidence: payload=%+v err=%v", validated, err)
	}
}

func TestCLIWP02ValidatorsAcceptOperationErrorFixtures(t *testing.T) {
	fixtures, _ := loadWP02Fixtures(t)
	if len(fixtures.OperationErrors.Positive) != 9 {
		t.Fatalf("operation error positives = %d", len(fixtures.OperationErrors.Positive))
	}
	seen := map[string]struct{}{}
	for _, fixture := range fixtures.OperationErrors.Positive {
		t.Run(fixture.ID, func(t *testing.T) {
			validated, err := ValidatePayload(tupleForCommand(fixture.Command), fixtureRaw(t, fixture))
			if err != nil {
				t.Fatal(err)
			}
			if validated.OperationError == nil || validated.OperationError.Command != fixture.Command ||
				validated.OperationError.Exit < 2 || validated.OperationError.Exit > 8 {
				t.Fatalf("operation error metadata lost: %+v", validated)
			}
			if validated.OperationID != validated.OperationError.OperationID {
				t.Fatalf("operation ID differs: %+v", validated)
			}
		})
		seen[fixture.Command] = struct{}{}
	}
	if len(seen) != 9 {
		t.Fatalf("operation error commands = %d", len(seen))
	}
}

func TestCLIWP02ValidatorsRejectOperationErrorMutants(t *testing.T) {
	fixtures, _ := loadWP02Fixtures(t)
	positives := map[string]wp02PositiveFixture{}
	for _, fixture := range fixtures.OperationErrors.Positive {
		positives[fixture.ID] = fixture
	}
	if len(fixtures.OperationErrors.Negative) != 9 {
		t.Fatalf("operation error negatives = %d", len(fixtures.OperationErrors.Negative))
	}
	for _, mutation := range fixtures.OperationErrors.Negative {
		t.Run(mutation.ID, func(t *testing.T) {
			twin, ok := positives[mutation.Twin]
			if !ok {
				t.Fatalf("unknown twin %q", mutation.Twin)
			}
			document := cloneDocument(t, twin.Document)
			key := strings.TrimPrefix(mutation.Path, "/")
			switch mutation.Operation {
			case "remove":
				delete(document, key)
			case "add", "replace":
				if mutation.ValueFrom == "syntheticSecretSentinel" {
					document[key] = fixtures.SyntheticSecretSentinel
				} else {
					document[key] = mutation.Value
				}
			case "append-json-document":
			default:
				t.Fatalf("unsupported mutation %q", mutation.Operation)
			}
			raw, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if mutation.Operation == "append-json-document" {
				raw = append(raw, []byte(`{}`)...)
			}
			validated, err := ValidatePayload(tupleForCommand(twin.Command), raw)
			if validated != nil {
				t.Fatalf("operation error mutant escaped: %+v", validated)
			}
			requirePayloadValidationError(t, err)
		})
	}
}

func setNested(document map[string]any, object, field string, value any) {
	document[object].(map[string]any)[field] = value
}

func mutateFirstObject(document map[string]any, array, field string, value any) {
	document[array].([]any)[0].(map[string]any)[field] = value
}

func TestCLIWP02ValidatorsRejectSchemaSemantics(t *testing.T) {
	fixtures, _ := loadWP02Fixtures(t)
	byCommand := map[string]wp02PositiveFixture{}
	for _, fixture := range fixtures.Payloads.Positive {
		byCommand[fixture.Command] = fixture
	}
	type semanticCase struct {
		command string
		mutate  func(map[string]any)
	}
	cases := map[string]semanticCase{
		"status enum":             {"status", func(d map[string]any) { d["overallStatus"] = "HEALTHY" }},
		"status empty planes":     {"status", func(d map[string]any) { d["planes"] = []any{} }},
		"status reason code":      {"status", func(d map[string]any) { mutateFirstObject(d, "planes", "reasonCode", "bad") }},
		"status timestamp":        {"status", func(d map[string]any) { d["observedAt"] = "2026-09-15 00:00:00" }},
		"network role":            {"network validate", func(d map[string]any) { mutateFirstObject(d, "interfaces", "role", "outside") }},
		"network mtu":             {"network validate", func(d map[string]any) { mutateFirstObject(d, "interfaces", "mtu", 100) }},
		"network nested unknown":  {"network validate", func(d map[string]any) { mutateFirstObject(d, "interfaces", "passwordHint", "none") }},
		"network finding":         {"network validate", func(d map[string]any) { mutateFirstObject(d, "warnings", "code", "bad") }},
		"public ipv4":             {"public-address configure", func(d map[string]any) { d["publicAddress"] = "2001:db8::1" }},
		"public pending mismatch": {"public-address configure", func(d map[string]any) { d["restart"] = "RECONCILED" }},
		"public duplicate san": {"public-address configure", func(d map[string]any) {
			setNested(d, "certificate", "sans", []any{"203.0.113.10", "203.0.113.10"})
		}},
		"gateway endpoint":       {"gateway register-local", func(d map[string]any) { d["endpoint"] = "http://127.0.0.1" }},
		"gateway identity":       {"gateway register-local", func(d map[string]any) { setNested(d, "gatewayIdentity", "address", "invalid") }},
		"gateway not verified":   {"gateway register-local", func(d map[string]any) { d["verified"] = false }},
		"diagnostics path":       {"diagnostics create", func(d map[string]any) { d["archivePath"] = "relative.tar" }},
		"diagnostics size":       {"diagnostics create", func(d map[string]any) { d["sizeBytes"] = 0 }},
		"diagnostics secret hit": {"diagnostics create", func(d map[string]any) { setNested(d, "secretScan", "hits", 1) }},
		"diagnostics expiry":     {"diagnostics create", func(d map[string]any) { d["expiresAt"] = "tomorrow" }},
		"logs component":         {"logs", func(d map[string]any) { d["component"] = "unknown" }},
		"logs redaction":         {"logs", func(d map[string]any) { d["redacted"] = false }},
		"logs window":            {"logs", func(d map[string]any) { setNested(d, "window", "lines", 10001) }},
		"logs entry timestamp":   {"logs", func(d map[string]any) { mutateFirstObject(d, "entries", "timestamp", "not-a-time") }},
		"logs credential value":  {"logs", func(d map[string]any) { mutateFirstObject(d, "entries", "message", "access_token=example") }},
		"backup key path":        {"backup key-create", func(d map[string]any) { d["keyPath"] = "relative.key" }},
		"backup key fingerprint": {"backup key-create", func(d map[string]any) { d["fingerprint"] = "ABC" }},
		"backup key mode":        {"backup key-create", func(d map[string]any) { d["mode"] = "0644" }},
		"backup create partial": {"backup create", func(d map[string]any) {
			d["result"] = "PARTIAL"
			d["consistency"] = "APPLICATION_CONSISTENT"
		}},
		"backup create encryption": {"backup create", func(d map[string]any) { d["encrypted"] = false }},
		"backup create checksums":  {"backup create", func(d map[string]any) { d["componentChecksums"] = map[string]any{} }},
		"backup verify auth":       {"backup verify", func(d map[string]any) { d["authenticated"] = false }},
		"backup verify release":    {"backup verify", func(d map[string]any) { d["releaseCompatibility"] = "UNKNOWN" }},
		"backup verify timestamp":  {"backup verify", func(d map[string]any) { d["observedAt"] = "2026-09-15T00:00:02+09:00" }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture, ok := byCommand[tc.command]
			if !ok {
				t.Fatalf("fixture missing for %q", tc.command)
			}
			document := cloneDocument(t, fixture.Document)
			tc.mutate(document)
			raw, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			validated, err := ValidatePayload(tupleForCommand(tc.command), raw)
			if validated != nil {
				t.Fatalf("semantic mutant escaped as success: %+v", validated)
			}
			requirePayloadValidationError(t, err)
		})
	}
}

type wp02Schema struct {
	ID                   string                     `json:"$id"`
	AdditionalProperties bool                       `json:"additionalProperties"`
	Required             []string                   `json:"required"`
	Properties           map[string]json.RawMessage `json:"properties"`
}

func validateSchemaAgainstSpec(raw []byte, spec payloadSpec) error {
	var schema wp02Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return err
	}
	if schema.ID != spec.SchemaID || schema.AdditionalProperties || len(schema.Required) != len(spec.RequiredKeys) {
		return errors.New("schema identity or required set differs")
	}
	if strings.Join(schema.Required, "\x00") != strings.Join(spec.RequiredKeys, "\x00") {
		return errors.New("schema required order differs")
	}
	allowed := stringSet(append(append([]string(nil), spec.RequiredKeys...), spec.OptionalKeys...))
	if len(schema.Properties) != len(allowed) {
		return errors.New("schema properties are not the exact required and optional set")
	}
	var schemaVersion, command struct {
		Const string `json:"const"`
	}
	if err := json.Unmarshal(schema.Properties["schemaVersion"], &schemaVersion); err != nil || schemaVersion.Const != spec.Tuple.SchemaVersion {
		return errors.New("schemaVersion const differs")
	}
	if err := json.Unmarshal(schema.Properties["command"], &command); err != nil || command.Const != spec.Tuple.Command {
		return errors.New("command const differs")
	}
	for key := range schema.Properties {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("schema exposes unknown property %q", key)
		}
		if isProhibitedPayloadKey(key) {
			return fmt.Errorf("schema exposes prohibited key %q", key)
		}
	}
	for _, key := range spec.OptionalKeys {
		if _, ok := schema.Properties[key]; !ok {
			return fmt.Errorf("schema optional property %q is missing", key)
		}
	}
	return nil
}

func TestCLIWP02ParityMatchesSchemasFixturesAndRegistry(t *testing.T) {
	fixtures, _ := loadWP02Fixtures(t)
	registered := RegisteredPayloadTuples()
	fixtureCommands := map[string]struct{}{}
	for _, fixture := range fixtures.Payloads.Positive {
		fixtureCommands[fixture.Command] = struct{}{}
	}
	for _, tuple := range registered {
		if _, legacy := fixtureCommands[tuple.Command]; !legacy {
			continue
		}
		spec := payloadRegistry[tuple]
		raw, err := os.ReadFile(filepath.Join(wp02RepoRoot(t), filepath.FromSlash(spec.SchemaPath)))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateSchemaAgainstSpec(raw, spec); err != nil {
			t.Fatalf("%s: %v", tuple.Command, err)
		}
	}
	if len(fixtureCommands) != 9 {
		t.Fatalf("legacy WP-02 fixture count = %d, want 9", len(fixtureCommands))
	}
}

func TestNCPPhase2LifecycleSchemasMatchRegistry(t *testing.T) {
	commands := []string{
		"restore plan", "restore execute", "update plan", "update execute", "update status",
		"rollback plan", "rollback execute", "rollback status", "factory-reset plan", "factory-reset execute",
	}
	for _, command := range commands {
		spec, ok := payloadRegistry[tupleForCommand(command)]
		if !ok {
			t.Fatalf("lifecycle command %q is absent from registry", command)
		}
		raw, err := os.ReadFile(filepath.Join(wp02RepoRoot(t), filepath.FromSlash(spec.SchemaPath)))
		if err != nil {
			t.Fatal(err)
		}
		if err := validateSchemaAgainstSpec(raw, spec); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
	}
}

func TestCLIWP02ParityRejectsOneSidedDrift(t *testing.T) {
	spec := payloadRegistry[tupleForCommand("status")]
	raw, err := os.ReadFile(filepath.Join(wp02RepoRoot(t), filepath.FromSlash(spec.SchemaPath)))
	if err != nil {
		t.Fatal(err)
	}
	for name, mutant := range map[string][]byte{
		"schema id":       []byte(strings.Replace(string(raw), spec.SchemaID, spec.SchemaID+"-drift", 1)),
		"schema version":  []byte(strings.Replace(string(raw), payloadSchema, payloadSchema+"-drift", 1)),
		"command const":   []byte(strings.Replace(string(raw), `"const": "status"`, `"const": "other"`, 1)),
		"required field":  []byte(strings.Replace(string(raw), `"overallStatus", `, "", 1)),
		"secret property": []byte(strings.Replace(string(raw), `"properties": {`, `"properties": {"password":{"type":"string"},`, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if validateSchemaAgainstSpec(mutant, spec) == nil {
				t.Fatal("one-sided schema drift survived")
			}
		})
	}
}
