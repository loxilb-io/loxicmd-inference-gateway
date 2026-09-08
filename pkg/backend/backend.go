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
	"os/exec"
	"regexp"
	"strings"

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

// contractAPIPrefix and contractMajor pin the handshake major this CLI
// speaks. A backend advertising any other major is refused before any
// mutating call.
const (
	contractAPIPrefix = "loxilb.io/appliance-backend/v"
	contractMajor     = 1
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
	SchemaVersion  int               `json:"schemaVersion"`
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
func (c *Contract) Supports(name string) bool {
	for _, cmd := range c.Commands {
		if cmd.Name == name {
			return true
		}
	}
	return false
}

// Result is one backend invocation, preserved without loss.
type Result struct {
	Stdout        []byte
	Stderr        []byte
	ExitCode      int
	CorrelationID string
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

	err := cmd.Run()
	res := &Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if err == nil {
		return res, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	// The process never ran: absent binary, permission, or context death.
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
// arguments. Kept as the simple entry point for the read-only set.
func Invoke(ctx context.Context, subcommand string, jsonOut bool) (*Result, error) {
	return invoke(ctx, &Request{Subcommand: subcommand, JSON: jsonOut}, false)
}

// InvokeMutating runs one allowlisted mutating subcommand. Per the
// invocation contract it performs the contract-version handshake first and
// refuses — changing no host state — when the installed backend does not
// advertise the subcommand.
func InvokeMutating(ctx context.Context, req *Request) (*Result, error) {
	contract, err := Handshake(ctx)
	if err != nil {
		return nil, err
	}
	if !contract.Supports(req.Subcommand) {
		return nil, &exitcode.CLIError{
			Code:          exitcode.ContractMismatch,
			Message:       fmt.Sprintf("the installed backend does not provide %q (backend %s, release %s)", req.Subcommand, contract.BackendVersion, contract.ProductRelease),
			Origin:        "backend",
			ComponentCode: "command-unavailable",
		}
	}
	return invoke(ctx, req, true)
}

func invoke(ctx context.Context, req *Request, mutating bool) (*Result, error) {
	rule, ok := allowedArgv[req.Subcommand]
	if !ok || rule.mutating != mutating {
		return nil, &exitcode.CLIError{
			Code:          exitcode.ContractMismatch,
			Message:       fmt.Sprintf("subcommand %q is not in this CLI's backend allowlist", req.Subcommand),
			Origin:        "cli",
			ComponentCode: "argv-not-allowlisted",
		}
	}
	if err := rule.validate(req.Args); err != nil {
		return nil, &exitcode.CLIError{
			Code:          exitcode.ContractMismatch,
			Message:       fmt.Sprintf("argv for %q rejected: %v", req.Subcommand, err),
			Origin:        "cli",
			ComponentCode: "argv-not-allowlisted",
		}
	}
	if req.Secret != nil && !rule.stdinSecret {
		return nil, &exitcode.CLIError{
			Code:          exitcode.ContractMismatch,
			Message:       fmt.Sprintf("subcommand %q does not take a secret stream", req.Subcommand),
			Origin:        "cli",
			ComponentCode: "argv-not-allowlisted",
		}
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

// Handshake runs `contract-version --json` and validates the response
// against the frozen schema rules. It is required before any mutating
// subcommand; read-only subcommands may proceed without it for best-effort
// diagnosis.
func Handshake(ctx context.Context) (*Contract, error) {
	res, err := run(ctx, []string{"contract-version", "--json"})
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, &exitcode.CLIError{
			Code:          exitcode.Unavailable,
			Message:       fmt.Sprintf("the backend's contract-version handshake failed (exit %d): %s", res.ExitCode, strings.TrimSpace(string(res.Stderr))),
			Origin:        "backend",
			ComponentCode: CodeBackendUnavailable,
		}
	}
	var contract Contract
	dec := json.NewDecoder(bytes.NewReader(res.Stdout))
	dec.DisallowUnknownFields()
	if derr := dec.Decode(&contract); derr != nil {
		return nil, contractViolation(fmt.Sprintf("the backend's contract document does not parse: %v", derr))
	}
	if verr := contract.validate(); verr != nil {
		return nil, verr
	}
	return &contract, nil
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
	case c.SchemaVersion < 1:
		return contractViolation(fmt.Sprintf("contract schemaVersion %d is invalid", c.SchemaVersion))
	}
	if c.APIVersion != fmt.Sprintf("%s%d", contractAPIPrefix, contractMajor) {
		return &exitcode.CLIError{
			Code:          exitcode.ContractMismatch,
			Message:       fmt.Sprintf("the installed backend speaks %s; this CLI speaks %s%d — use a matching release", c.APIVersion, contractAPIPrefix, contractMajor),
			Origin:        "backend",
			ComponentCode: "contract-major-unsupported",
		}
	}
	for _, cmd := range c.Commands {
		if !commandNameShape.MatchString(cmd.Name) {
			return contractViolation(fmt.Sprintf("contract advertises malformed command name %q", cmd.Name))
		}
	}
	return nil
}

func contractViolation(msg string) *exitcode.CLIError {
	return &exitcode.CLIError{
		Code:          exitcode.ContractMismatch,
		Message:       msg,
		Origin:        "backend",
		ComponentCode: "contract-invalid",
	}
}
