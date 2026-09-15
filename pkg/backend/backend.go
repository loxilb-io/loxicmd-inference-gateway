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

// Package backend is the adapter between the `appliance` command family and
// the host lifecycle backend, implementing the invocation side of
// contracts/host-backend-contract.md: fixed executable path, direct argv
// execution against a per-subcommand allowlist, a scrubbed child environment,
// a correlation ID per invocation, the contract-version handshake, and
// lossless preservation of the backend's streams and exit status.
//
// The package implements no lifecycle logic of its own: a backend that is
// absent or does not advertise a function is reported as such, never
// imitated.
package backend

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
)

// executablePath is the host lifecycle backend, fixed at compile time per
// the invocation contract. It is deliberately not a flag, not read from the
// environment, and not resolved through PATH; test builds relocate it only
// through `-ldflags "-X .../pkg/backend.executablePath=..."` (the packaged
// test binaries do exactly that against a fake backend).
var executablePath = "/usr/libexec/loxilb-appliance/loxilb-appliance-backend"

// CodeBackendUnavailable is the componentCode for a backend that is absent
// or cannot be executed — spelled exactly as the invocation contract fixes
// it.
const CodeBackendUnavailable = "BACKEND_UNAVAILABLE"

// CodeBackendForbidden is the componentCode for a backend present but not
// executable by the caller. Distinct from BACKEND_UNAVAILABLE because the
// remedy is: absent may resolve itself, forbidden never does.
const CodeBackendForbidden = "BACKEND_FORBIDDEN"

// backendWaitDelay bounds how long cmd.Wait may spend after the process is
// done waiting on I/O pipes a straggling grandchild still holds. It is a
// backstop, not a timeout: the process-group kill is what normally closes
// them, and this only decides how long the CLI tolerates a straggler that
// escaped it. Generous enough not to truncate a slow final write, short
// enough that automation never perceives a hang.
const backendWaitDelay = 2 * time.Second

// contractAPIPrefix and contractMajor pin the handshake major this CLI
// speaks. A backend advertising any other major is refused before any
// mutating call.
const (
	contractAPIPrefix = "loxilb.io/appliance-backend/v"
	contractMajor     = 1
	payloadSchema     = "appliance-backend-payload/v1"
)

const (
	codeBackendReleaseMarkerMissing = "BACKEND_RELEASE_MARKER_MISSING"
	codeBackendReleaseMarkerInvalid = "BACKEND_RELEASE_MARKER_INVALID"
	codeBackendContractInvalid      = "BACKEND_CONTRACT_INVALID"
	codeBackendHandshakeUnavailable = "BACKEND_HANDSHAKE_UNAVAILABLE"
)

// argvRule is one subcommand's spawn surface: how many positional arguments
// it may carry and which flags it may carry beyond `--json` and
// `--correlation-id <id>`. A flag ending in '=' takes exactly one value
// token; any argv token outside the rule is rejected CLI-side before a
// process starts.
type argvRule struct {
	positionals int
	flags       []string
	// mutating subcommands run only after the contract-version handshake
	// proves the installed backend advertises them.
	mutating bool
	// stdinSecret marks the subcommand as receiving a secret on stdin;
	// it is the only stdin use the invocation contract permits.
	stdinSecret bool
}

// allowedArgv is the complete set of backend subcommands this CLI build may
// spawn. Anything not in this table — subcommand or argv shape — never
// reaches exec.
var allowedArgv = map[string]argvRule{
	"contract-version":         {},
	"status":                   {},
	"network validate":         {},
	"public-address configure": {positionals: 1, flags: []string{"--no-restart"}, mutating: true},
	"gateway register-local":   {flags: []string{"--username=", "--password-stdin"}, mutating: true, stdinSecret: true},
	"credentials bootstrap":    {mutating: true},
	"diagnostics create":       {flags: []string{"--redact", "--output="}, mutating: true},
	"logs":                     {positionals: 1, flags: []string{"--redact", "--since=", "--lines="}, mutating: true},
	"backup key-create":        {flags: []string{"--key-file="}, mutating: true},
	"backup create":            {positionals: 1, flags: []string{"--key-file="}, mutating: true},
	"backup verify":            {positionals: 1, flags: []string{"--key-file="}, mutating: true},
}

// contractCommands is the CLI-WP00-approved CONSERVATIVE_V1 matrix. The
// handshake must advertise these ten entries byte-for-byte in this canonical
// order; a backend command list is contract data, not a discovery hint.
var contractCommands = []ContractCommand{
	{Name: "status", ReadOnly: true, Capabilities: []string{"json-output"}},
	{Name: "network validate", ReadOnly: true, Capabilities: []string{"json-output"}},
	{Name: "public-address configure", Capabilities: []string{"json-output", "no-restart", "operation-receipt"}},
	{Name: "gateway register-local", Capabilities: []string{"json-output", "secret-stdin", "operation-receipt"}},
	{Name: "credentials bootstrap", Capabilities: []string{"console-only"}},
	{Name: "diagnostics create", Capabilities: []string{"json-output", "redaction", "explicit-output", "operation-receipt"}},
	{Name: "logs", Capabilities: []string{"json-output", "redaction", "bounded-window"}},
	{Name: "backup key-create", Capabilities: []string{"json-output", "key-file", "operation-receipt"}},
	{Name: "backup create", Capabilities: []string{"json-output", "key-file", "operation-receipt"}},
	{Name: "backup verify", Capabilities: []string{"json-output", "key-file"}},
}

var capabilityShape = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// validate checks one extra-argv slice against the rule. Positional tokens
// must come first; flags must match the rule exactly, with '='-suffixed
// flags consuming exactly one following value token.
func (r argvRule) validate(args []string) error {
	i := 0
	for i < len(args) && !strings.HasPrefix(args[i], "-") {
		i++
	}
	if i > r.positionals {
		return fmt.Errorf("%d positional argument(s), at most %d allowed", i, r.positionals)
	}
	for i < len(args) {
		token := args[i]
		matched := false
		for _, flag := range r.flags {
			if valued := strings.HasSuffix(flag, "="); valued && token == strings.TrimSuffix(flag, "=") {
				if i+1 >= len(args) {
					return fmt.Errorf("flag %s needs a value", token)
				}
				i += 2
				matched = true
				break
			} else if !valued && token == flag {
				i++
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("token %q is not in the allowlist", token)
		}
	}
	return nil
}

// Contract is the response of `contract-version --json`, mirroring
// contracts/backend-contract.schema.json.
type Contract struct {
	APIVersion     string            `json:"apiVersion"`
	Kind           string            `json:"kind"`
	BackendVersion string            `json:"backendVersion"`
	ProductRelease string            `json:"productRelease"`
	SchemaVersion  string            `json:"schemaVersion"`
	Commands       []ContractCommand `json:"commands"`
}

// ContractCommand is one advertised backend subcommand.
type ContractCommand struct {
	Name         string   `json:"name"`
	ReadOnly     bool     `json:"readOnly"`
	Capabilities []string `json:"capabilities"`
}

// Supports reports whether the backend advertises the subcommand. A
// subcommand absent from the contract is unavailable regardless of what
// this CLI build knows about.
func (c *Contract) Lookup(name string) (ContractCommand, bool) {
	for _, cmd := range c.Commands {
		if cmd.Name == name {
			return cmd, true
		}
	}
	return ContractCommand{}, false
}

// Supports remains the compatibility predicate for callers that need only
// availability. Enforcement paths use Lookup so readOnly and capabilities
// cannot be silently discarded.
func (c *Contract) Supports(name string) bool {
	_, ok := c.Lookup(name)
	return ok
}

// BackendContractError is the exact typed nonzero result of
// `contract-version --json`. Retryable remains available to the composition
// layer even though the current public CLIError predates that field.
type BackendContractError struct {
	SchemaVersion string `json:"schemaVersion"`
	Exit          int    `json:"exit"`
	Code          string `json:"code"`
	ComponentCode string `json:"componentCode"`
	Retryable     bool   `json:"retryable"`
	Message       string `json:"message,omitempty"`
}

func (e *BackendContractError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("backend handshake failed: %s", e.ComponentCode)
}

// Unwrap preserves compatibility with exitcode.Classify while keeping the
// richer typed error available through errors.As.
func (e *BackendContractError) Unwrap() error {
	return &exitcode.CLIError{
		Code:          exitcode.Code(e.Exit),
		Message:       e.Error(),
		Origin:        "backend",
		ComponentCode: e.ComponentCode,
	}
}

// DegradedResult contains only typed observations. In JSON mode Invoke clears
// the unvalidated backend streams before returning this value.
type DegradedResult struct {
	Degraded           bool   `json:"degraded"`
	Reason             string `json:"reason"`
	HandshakeStatus    int    `json:"handshakeStatus"`
	OperationAttempted bool   `json:"operationAttempted"`
	BackendExitStatus  int    `json:"backendExitStatus"`
	RetryGuidance      string `json:"retryGuidance"`
	CorrelationID      string `json:"correlationId"`
}

// Result is one backend invocation, preserved without loss.
type Result struct {
	Stdout        []byte
	Stderr        []byte
	ExitCode      int
	CorrelationID string
	// Signaled reports that the process was killed rather than exiting on
	// its own. The distinction is what separates a definite failure from an
	// unknown outcome: a backend that exits 3 has decided something, a
	// backend that dies mid-write has not.
	Signaled bool
	// TimedOut reports that the invocation's context expired, i.e. the kill
	// was ours. Callers need this to tell "the operator's --timeout bounded
	// a slow operation" from "the backend died on its own".
	TimedOut bool
	// Degraded is set only when a read-only best-effort operation followed a
	// failed handshake. It never contains backend stdout or secret material.
	Degraded *DegradedResult
}

// OperationID is the identifier a caller can recover with. The backend's own
// id wins when it reported one in its JSON document; otherwise the correlation
// id stands in, which the invocation contract requires to be searchable in the
// host journal and is therefore the handle an operator actually needs. The
// distinction matters enough not to blur: this never claims a CLI-side id came
// from the backend, it just guarantees the field is useful when it is set.
func (r *Result) OperationID() string {
	var doc struct {
		OperationID string `json:"operationId"`
	}
	if json.Unmarshal(r.Stdout, &doc) == nil && doc.OperationID != "" {
		return doc.OperationID
	}
	return r.CorrelationID
}

var (
	apiVersionShape  = regexp.MustCompile(`^loxilb\.io/appliance-backend/v[0-9]+$`)
	commandNameShape = regexp.MustCompile(`^[a-z][a-z0-9-]*( [a-z][a-z0-9-]*)*$`)
)

// newCorrelationID builds the per-invocation ID the CLI passes to the
// backend and echoes in the envelope, so one identifier joins the CLI
// output and the host's journal/audit trail.
func newCorrelationID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// A machine whose entropy source fails still deserves a
		// correlation ID; collision resistance degrades, traceability
		// does not disappear.
		return "cli-0000000000000000"
	}
	return "cli-" + hex.EncodeToString(b)
}

// run spawns the backend with the exact argv given, a scrubbed environment,
// and full stream capture. Only invoke and Handshake call it, after
// allowlist validation.
func run(ctx context.Context, argv []string) (*Result, error) {
	return runWithStdin(ctx, argv, nil)
}

func runWithStdin(ctx context.Context, argv []string, stdin io.Reader) (*Result, error) {
	cmd := exec.CommandContext(ctx, executablePath, argv...)
	cmd.Stdin = stdin
	// The child environment is fixed by the CLI: nothing from the caller
	// is forwarded, so no environment variable can redirect the backend's
	// product root or config/state directories.
	cmd.Env = []string{}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	// Bounding the invocation takes both of the following, because the
	// backend is a process TREE, not a process: the real one shells out to
	// tar, pg_dump, systemctl and journalctl, and every child inherits the
	// stdout/stderr pipes created for the buffers above.
	//
	// 1. Its own process group, killed as a group. exec.CommandContext
	//    cancels by killing the direct child only; a surviving grandchild
	//    both holds the pipes open (so cmd.Wait never returns -- see 2) and
	//    keeps mutating host state after the CLI has already told the caller
	//    the operation failed. An orphaned tar still writing the archive the
	//    caller was just told was not written is the worst version of that.
	// 2. A WaitDelay backstop. cmd.Wait waits for the io-copying goroutines
	//    as well as the process, and a pipe reaches EOF only once EVERY
	//    write end is closed. The group kill above should close them, but a
	//    grandchild that put itself in another group, or one wedged in
	//    uninterruptible sleep, would otherwise hang the CLI forever. Note
	//    this case needs no deadline to bite: a backend that merely
	//    daemonizes a child and exits 0 hangs an invocation that has no
	//    --timeout at all.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Setpgid makes the child a group leader, so its PGID is its PID
		// and -PID addresses the whole tree it started. ESRCH just means
		// the tree is already gone, which is the outcome we wanted.
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}
	cmd.WaitDelay = backendWaitDelay

	err := cmd.Run()
	// ErrWaitDelay means the process finished but its pipes had to be closed
	// out from under a straggler. Whatever the backend managed to write is in
	// the buffers and is reported; the invocation itself did not fail because
	// of it, so it must not be dressed up as a backend error.
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil
	}
	res := &Result{
		Stdout:   stdout.Bytes(),
		Stderr:   stderr.Bytes(),
		TimedOut: ctx.Err() != nil,
	}
	if err == nil {
		return res, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		// ExitCode reports -1 for a process that never exited normally --
		// killed by our cancel, by a signal, or by the OOM killer. That is
		// not an exit status the backend chose, so it must not be reported
		// as one.
		res.Signaled = exitErr.ExitCode() < 0
		return res, nil
	}
	// The process never ran. Absent and unexecutable are different failures
	// with different remedies: an absent backend may appear when the package
	// finishes installing, so bounded retry is sensible; a backend this
	// caller may not execute will answer the same way forever, and the
	// operator has to change who they are, not wait.
	if errors.Is(err, fs.ErrPermission) {
		return nil, &exitcode.CLIError{
			Code: exitcode.Auth,
			Message: fmt.Sprintf(
				"the host lifecycle backend cannot be executed by this user: %v; appliance commands run as root", err),
			Origin:        "os",
			ComponentCode: CodeBackendForbidden,
		}
	}
	return nil, &exitcode.CLIError{
		Code:          exitcode.Unavailable,
		Message:       fmt.Sprintf("the host lifecycle backend could not be executed: %v", err),
		Origin:        "backend",
		ComponentCode: CodeBackendUnavailable,
	}
}

// Request is one backend invocation as the command layer asks for it.
// Args carries the extra argv beyond the subcommand words; Secret, when
// non-nil, is streamed to the backend's stdin (the invocation contract's
// only stdin use) and never appears in argv or the environment.
type Request struct {
	Subcommand string
	Args       []string
	JSON       bool
	Secret     io.Reader
}

// Invoke runs one allowlisted read-only backend subcommand with no extra
// arguments. It handshakes first. A typed/contract handshake failure may be
// followed by one best-effort read, but JSON bytes from that read are discarded
// and the typed handshake verdict remains the returned error.
func Invoke(ctx context.Context, subcommand string, jsonOut bool) (*Result, error) {
	req := &Request{Subcommand: subcommand, JSON: jsonOut}
	if err := validateRequest(req, false); err != nil {
		return nil, err
	}
	contract, handshakeErr := Handshake(ctx)
	if handshakeErr == nil {
		if err := requireAdvertisedCommand(contract, subcommand, true); err != nil {
			return nil, err
		}
		return invoke(ctx, req, false)
	}
	if !allowsReadOnlyBestEffort(handshakeErr) {
		return nil, handshakeErr
	}

	res, operationErr := invoke(ctx, req, false)
	if res == nil {
		res = &Result{}
	}
	classified := exitcode.Classify(handshakeErr)
	retryGuidance := "Correct the installed backend contract before retrying."
	var typed *BackendContractError
	if errors.As(handshakeErr, &typed) && typed.Retryable {
		retryGuidance = "Retry with bounded backoff after the backend dependency recovers."
	}
	res.Degraded = &DegradedResult{
		Degraded:           true,
		Reason:             classified.ComponentCode,
		HandshakeStatus:    int(classified.Code),
		OperationAttempted: true,
		BackendExitStatus:  res.ExitCode,
		RetryGuidance:      retryGuidance,
		CorrelationID:      res.CorrelationID,
	}
	if operationErr != nil {
		res.Degraded.BackendExitStatus = -1
	}
	if jsonOut {
		res.Stdout = nil
		res.Stderr = nil
	}
	return res, handshakeErr
}

// InvokeMutating runs one allowlisted mutating subcommand. Per the
// invocation contract it performs the contract-version handshake first and
// refuses — changing no host state — when the installed backend does not
// advertise the subcommand.
func InvokeMutating(ctx context.Context, req *Request) (*Result, error) {
	if err := validateRequest(req, true); err != nil {
		return nil, err
	}
	contract, err := Handshake(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireAdvertisedCommand(contract, req.Subcommand, false); err != nil {
		return nil, err
	}
	return invoke(ctx, req, true)
}

func invoke(ctx context.Context, req *Request, mutating bool) (*Result, error) {
	if err := validateRequest(req, mutating); err != nil {
		return nil, err
	}
	id := newCorrelationID()
	argv := append(strings.Fields(req.Subcommand), req.Args...)
	argv = append(argv, "--correlation-id", id)
	if req.JSON {
		argv = append(argv, "--json")
	}
	res, err := runWithStdin(ctx, argv, req.Secret)
	if err != nil {
		return nil, err
	}
	res.CorrelationID = id
	return res, nil
}

func validateRequest(req *Request, mutating bool) error {
	rule, ok := allowedArgv[req.Subcommand]
	if !ok || rule.mutating != mutating {
		return &exitcode.CLIError{
			Code:          exitcode.ContractMismatch,
			Message:       fmt.Sprintf("subcommand %q is not in this CLI's backend allowlist", req.Subcommand),
			Origin:        "cli",
			ComponentCode: "argv-not-allowlisted",
		}
	}
	if err := rule.validate(req.Args); err != nil {
		return &exitcode.CLIError{
			Code:          exitcode.ContractMismatch,
			Message:       fmt.Sprintf("argv for %q rejected: %v", req.Subcommand, err),
			Origin:        "cli",
			ComponentCode: "argv-not-allowlisted",
		}
	}
	if req.Secret != nil && !rule.stdinSecret {
		return &exitcode.CLIError{
			Code:          exitcode.ContractMismatch,
			Message:       fmt.Sprintf("subcommand %q does not take a secret stream", req.Subcommand),
			Origin:        "cli",
			ComponentCode: "argv-not-allowlisted",
		}
	}
	return nil
}

// Handshake runs `contract-version --json` and validates the response
// against the frozen schema rules. It is required before any mutating
// subcommand. Read-only subcommands also handshake and may perform a single
// typed degraded best-effort read when the executable itself is available.
func Handshake(ctx context.Context) (*Contract, error) {
	res, err := run(ctx, []string{"contract-version", "--json"})
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		typed, typedErr := decodeBackendContractError(res.Stdout)
		if typedErr != nil {
			return nil, contractViolation(fmt.Sprintf("the backend's typed contract error is invalid: %v", typedErr))
		}
		if typed.Exit != res.ExitCode {
			return nil, contractViolation(fmt.Sprintf("typed contract error exit %d does not match process exit %d", typed.Exit, res.ExitCode))
		}
		return nil, typed
	}
	contract, derr := decodeContract(res.Stdout)
	if derr != nil {
		return nil, contractViolation(fmt.Sprintf("the backend's contract document does not parse: %v", derr))
	}
	if verr := contract.validate(); verr != nil {
		return nil, verr
	}
	return contract, nil
}

// validate enforces the schema rules the CLI depends on. A violation is a
// contract mismatch: the CLI refuses to guess what the backend meant.
func (c *Contract) validate() error {
	switch {
	case c.Kind != "BackendContract":
		return contractViolation(fmt.Sprintf("contract kind is %q, want BackendContract", c.Kind))
	case !apiVersionShape.MatchString(c.APIVersion):
		return contractViolation(fmt.Sprintf("contract apiVersion %q is malformed", c.APIVersion))
	case c.BackendVersion == "" || c.ProductRelease == "":
		return contractViolation("contract omits backendVersion or productRelease")
	case c.SchemaVersion != payloadSchema:
		return contractViolation(fmt.Sprintf("contract schemaVersion %q is invalid", c.SchemaVersion))
	}
	if c.APIVersion != fmt.Sprintf("%s%d", contractAPIPrefix, contractMajor) {
		return &exitcode.CLIError{
			Code:          exitcode.ContractMismatch,
			Message:       fmt.Sprintf("the installed backend speaks %s; this CLI speaks %s%d — use a matching release", c.APIVersion, contractAPIPrefix, contractMajor),
			Origin:        "backend",
			ComponentCode: "contract-major-unsupported",
		}
	}
	if len(c.Commands) != len(contractCommands) {
		return contractViolation(fmt.Sprintf("contract advertises %d commands, want %d", len(c.Commands), len(contractCommands)))
	}
	seenCommands := make(map[string]struct{}, len(c.Commands))
	for index, cmd := range c.Commands {
		if !commandNameShape.MatchString(cmd.Name) {
			return contractViolation(fmt.Sprintf("contract advertises malformed command name %q", cmd.Name))
		}
		if _, exists := seenCommands[cmd.Name]; exists {
			return contractViolation(fmt.Sprintf("contract advertises duplicate command %q", cmd.Name))
		}
		seenCommands[cmd.Name] = struct{}{}
		seenCapabilities := make(map[string]struct{}, len(cmd.Capabilities))
		for _, capability := range cmd.Capabilities {
			if !capabilityShape.MatchString(capability) {
				return contractViolation(fmt.Sprintf("command %q advertises malformed capability %q", cmd.Name, capability))
			}
			if _, exists := seenCapabilities[capability]; exists {
				return contractViolation(fmt.Sprintf("command %q advertises duplicate capability %q", cmd.Name, capability))
			}
			seenCapabilities[capability] = struct{}{}
		}
		want := contractCommands[index]
		if cmd.Name != want.Name || cmd.ReadOnly != want.ReadOnly || !equalStrings(cmd.Capabilities, want.Capabilities) {
			return contractViolation(fmt.Sprintf("command metadata at index %d differs from the approved matrix", index))
		}
	}
	return nil
}

func requireAdvertisedCommand(contract *Contract, name string, readOnly bool) error {
	advertised, ok := contract.Lookup(name)
	if !ok {
		return contractViolation(fmt.Sprintf("the installed backend does not advertise %q", name))
	}
	var expected *ContractCommand
	for index := range contractCommands {
		if contractCommands[index].Name == name {
			expected = &contractCommands[index]
			break
		}
	}
	if expected == nil || expected.ReadOnly != readOnly || advertised.ReadOnly != expected.ReadOnly || !equalStrings(advertised.Capabilities, expected.Capabilities) {
		return contractViolation(fmt.Sprintf("the installed backend metadata for %q differs from the approved matrix", name))
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func allowsReadOnlyBestEffort(err error) bool {
	var classified *exitcode.CLIError
	if !errors.As(err, &classified) {
		return true
	}
	return classified.ComponentCode != CodeBackendUnavailable && classified.ComponentCode != CodeBackendForbidden
}

func decodeExactOne[T any](raw []byte) (*T, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var value *T
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, errors.New("document is null")
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("document contains a second JSON value")
		}
		return nil, fmt.Errorf("document has trailing content: %w", err)
	}
	return value, nil
}

type backendContractErrorWire struct {
	SchemaVersion *string `json:"schemaVersion"`
	Exit          *int    `json:"exit"`
	Code          *string `json:"code"`
	ComponentCode *string `json:"componentCode"`
	Retryable     *bool   `json:"retryable"`
	Message       *string `json:"message,omitempty"`
}

type contractWire struct {
	APIVersion     *string                `json:"apiVersion"`
	Kind           *string                `json:"kind"`
	BackendVersion *string                `json:"backendVersion"`
	ProductRelease *string                `json:"productRelease"`
	SchemaVersion  *string                `json:"schemaVersion"`
	Commands       *[]contractCommandWire `json:"commands"`
}

type contractCommandWire struct {
	Name         *string   `json:"name"`
	ReadOnly     *bool     `json:"readOnly"`
	Capabilities *[]string `json:"capabilities"`
}

func decodeContract(raw []byte) (*Contract, error) {
	wire, err := decodeExactOne[contractWire](raw)
	if err != nil {
		return nil, err
	}
	if wire.APIVersion == nil || wire.Kind == nil || wire.BackendVersion == nil || wire.ProductRelease == nil || wire.SchemaVersion == nil || wire.Commands == nil {
		return nil, errors.New("contract omits or nulls a required field")
	}
	contract := &Contract{
		APIVersion:     *wire.APIVersion,
		Kind:           *wire.Kind,
		BackendVersion: *wire.BackendVersion,
		ProductRelease: *wire.ProductRelease,
		SchemaVersion:  *wire.SchemaVersion,
		Commands:       make([]ContractCommand, 0, len(*wire.Commands)),
	}
	for index, command := range *wire.Commands {
		if command.Name == nil || command.ReadOnly == nil || command.Capabilities == nil {
			return nil, fmt.Errorf("contract command at index %d omits or nulls a required field", index)
		}
		contract.Commands = append(contract.Commands, ContractCommand{
			Name:         *command.Name,
			ReadOnly:     *command.ReadOnly,
			Capabilities: append([]string(nil), (*command.Capabilities)...),
		})
	}
	return contract, nil
}

var credentialValueShape = regexp.MustCompile(`(?i)(password|token|api[_-]?key|private[_-]?key)\s*[:=]`)

func decodeBackendContractError(raw []byte) (*BackendContractError, error) {
	wire, err := decodeExactOne[backendContractErrorWire](raw)
	if err != nil {
		return nil, err
	}
	if wire.SchemaVersion == nil || wire.Exit == nil || wire.Code == nil || wire.ComponentCode == nil || wire.Retryable == nil {
		return nil, errors.New("typed contract error omits a required field")
	}
	result := &BackendContractError{
		SchemaVersion: *wire.SchemaVersion,
		Exit:          *wire.Exit,
		Code:          *wire.Code,
		ComponentCode: *wire.ComponentCode,
		Retryable:     *wire.Retryable,
	}
	if wire.Message != nil {
		result.Message = *wire.Message
	}
	if result.SchemaVersion != "backend-contract-error/v1" {
		return nil, fmt.Errorf("typed contract error schemaVersion %q is unsupported", result.SchemaVersion)
	}
	if result.Message != "" {
		if len([]byte(result.Message)) > 512 {
			return nil, errors.New("typed contract error message exceeds 512 UTF-8 bytes")
		}
		for _, r := range result.Message {
			if unicode.IsControl(r) {
				return nil, errors.New("typed contract error message contains a control character")
			}
		}
		if credentialValueShape.MatchString(result.Message) || strings.Contains(result.Message, "CLI_WP00_SYNTHETIC_SECRET_DO_NOT_EMIT") {
			return nil, errors.New("typed contract error message contains prohibited credential material")
		}
	} else if wire.Message != nil {
		return nil, errors.New("typed contract error message is empty")
	}
	wantCode := ""
	wantRetryable := false
	switch result.ComponentCode {
	case codeBackendReleaseMarkerMissing:
		if result.Exit == int(exitcode.Precondition) {
			wantCode = exitcode.Precondition.Label()
		}
	case codeBackendReleaseMarkerInvalid, codeBackendContractInvalid:
		if result.Exit == int(exitcode.ContractMismatch) {
			wantCode = exitcode.ContractMismatch.Label()
		}
	case codeBackendHandshakeUnavailable:
		if result.Exit == int(exitcode.Unavailable) {
			wantCode = exitcode.Unavailable.Label()
			wantRetryable = true
		}
	}
	if wantCode == "" || result.Code != wantCode || result.Retryable != wantRetryable {
		return nil, errors.New("typed contract error tuple is not one of the approved failures")
	}
	return result, nil
}

func contractViolation(msg string) *exitcode.CLIError {
	return &exitcode.CLIError{
		Code:          exitcode.ContractMismatch,
		Message:       msg,
		Origin:        "backend",
		ComponentCode: codeBackendContractInvalid,
	}
}
