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

// allowedArgv is the complete set of backend subcommands this CLI build may
// spawn, each with the exact extra argv tokens permitted beyond the
// subcommand words, `--json`, and `--correlation-id <id>`. Anything not in
// this table is rejected CLI-side before a process starts.
var allowedArgv = map[string][]string{
	"contract-version": {},
	"status":           {},
	"network validate": {},
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
// and full stream capture. Only Invoke and Handshake call it, after
// allowlist validation.
func run(ctx context.Context, argv []string) (*Result, error) {
	cmd := exec.CommandContext(ctx, executablePath, argv...)
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

// Invoke runs one allowlisted backend subcommand. jsonOut appends the
// backend's `--json` selector; a correlation ID is generated, passed to the
// backend, and returned with the result. Argv outside the allowlist is
// rejected here, before any process is started.
func Invoke(ctx context.Context, subcommand string, jsonOut bool) (*Result, error) {
	extra, ok := allowedArgv[subcommand]
	if !ok || len(extra) != 0 {
		// len(extra) != 0 cannot happen with the current table; the
		// check keeps a future table edit from silently widening the
		// spawn surface without widening this validation.
		return nil, &exitcode.CLIError{
			Code:          exitcode.ContractMismatch,
			Message:       fmt.Sprintf("subcommand %q is not in this CLI's backend allowlist", subcommand),
			Origin:        "cli",
			ComponentCode: "argv-not-allowlisted",
		}
	}
	id := newCorrelationID()
	argv := append(strings.Fields(subcommand), "--correlation-id", id)
	if jsonOut {
		argv = append(argv, "--json")
	}
	res, err := run(ctx, argv)
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
