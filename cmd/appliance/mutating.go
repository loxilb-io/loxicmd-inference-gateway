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
package appliance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/backend"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

// logComponents is the fixed set of components 'appliance logs' may name;
// arbitrary container or unit names never reach the backend.
var logComponents = map[string]bool{
	"gateway": true, "oam": true, "ui": true, "caddy": true,
	"postgres": true, "state": true, "dataplane": true, "management": true,
}

// usernameShape keeps an operator-supplied username from being parsed as
// anything but a value: no flag-shaped or whitespace-bearing usernames
// enter the backend's argv.
var usernameShape = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._@-]*$`)

const (
	// logs bounds: a troubleshooting read, not an export pipeline.
	maxLogSince = 7 * 24 * time.Hour
	maxLogLines = 10000
)

// stdioIsTerminal reports whether both stdin and stdout are character
// devices — the console-only guard for credentials bootstrap. A pipe, a
// file redirect, or capture tooling fails the check.
func stdioIsTerminal() bool {
	for _, f := range []*os.File{os.Stdin, os.Stdout} {
		info, err := f.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}

// dispatchMutating invokes a mutating backend subcommand. The adapter
// performs the contract-version handshake first and refuses — changing no
// host state — when the installed backend does not advertise the
// subcommand.
func dispatchMutating(out io.Writer, restOptions *api.RESTOptions, command string, req *backend.Request) error {
	req.JSON = restOptions.PrintOption == "json"
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	res, err := backend.InvokeMutating(ctx, req)
	if err == nil {
		// mutating: a death mid-flight leaves host state unknown, which the
		// taxonomy reports as PARTIAL rather than as a retryable failure.
		err = backendOutcome(req.Subcommand, res, true)
	}
	if err != nil {
		if req.JSON {
			writeEnvelope(out, command, res, err)
		}
		return err
	}
	if !req.JSON {
		_, werr := out.Write(res.Stdout)
		return werr
	}
	if !json.Valid(res.Stdout) {
		err = &exitcode.CLIError{
			Code:          exitcode.ContractMismatch,
			Message:       fmt.Sprintf("the backend's %s output is not a JSON document", req.Subcommand),
			Origin:        "backend",
			ComponentCode: "contract-invalid",
		}
		writeEnvelope(out, command, res, err)
		return err
	}
	writeEnvelope(out, command, res, nil)
	return nil
}

func publicAddressCmd(restOptions *api.RESTOptions) *cobra.Command {
	parent := &cobra.Command{
		Use:   "public-address",
		Short: "Appliance public address (CSP 1:1 NAT) management",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("appliance public-address needs a subcommand")
			}
			return exitcode.Usagef("unknown command %q for \"loxicmd appliance public-address\"", args[0])
		},
	}
	var noRestart bool
	configure := &cobra.Command{
		Use:   "configure IPV4",
		Short: "Apply a NAT-assigned public IPv4 to edge TLS and the OAM allowed origin",
		Long: `Applies a public IPv4 address (already assigned through the cloud
provider's 1:1 NAT) to edge TLS and the OAM allowed origin: certificate with
public and private SANs, site address, and SNI fallback, updated atomically
by the host backend. It never assigns the address to a NIC and never touches
NAT, subnets, or routes.

Examples:
  loxicmd appliance public-address configure 203.0.113.10
  loxicmd appliance public-address configure 203.0.113.10 --no-restart`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			addr := args[0]
			parsed := net.ParseIP(addr)
			if parsed == nil || parsed.To4() == nil || parsed.String() != addr {
				return exitcode.Invalidf("%q is not a canonical IPv4 address", addr)
			}
			req := &backend.Request{Subcommand: "public-address configure", Args: []string{addr}}
			if noRestart {
				req.Args = append(req.Args, "--no-restart")
			}
			return dispatchMutating(cmd.OutOrStdout(), restOptions, "appliance.public-address.configure", req)
		},
	}
	configure.Flags().BoolVar(&noRestart, "no-restart", false,
		"Record the change and report a pending restart instead of reconciling the management plane now")
	parent.AddCommand(configure)
	return parent
}

func gatewayCmd(restOptions *api.RESTOptions) *cobra.Command {
	parent := &cobra.Command{
		Use:   "gateway",
		Short: "Local gateway registration in OAM",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("appliance gateway needs a subcommand")
			}
			return exitcode.Usagef("unknown command %q for \"loxicmd appliance gateway\"", args[0])
		},
	}
	var username, passwordFile string
	register := &cobra.Command{
		Use:   "register-local",
		Short: "Idempotently register (or repair) the co-located gateway in OAM",
		Long: `Registers the co-located gateway in OAM through the backend, or repairs
an existing registration, and proves the result by exact read-back. The
password is read from a root-only file and streamed to the backend on
stdin — never through a flag value, argv, or the environment.

Examples:
  loxicmd appliance gateway register-local --username admin --password-file /etc/loxilb-appliance/oam.pass`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			if username == "" || passwordFile == "" {
				return exitcode.Usagef("--username and --password-file are both required")
			}
			if !usernameShape.MatchString(username) {
				return exitcode.Invalidf("username %q has a shape this CLI refuses to pass on", username)
			}
			secret, err := backend.ReadSecretFile("password file", passwordFile)
			if err != nil {
				return err
			}
			req := &backend.Request{
				Subcommand: "gateway register-local",
				Args:       []string{"--username", username, "--password-stdin"},
				Secret:     bytes.NewReader(secret),
			}
			return dispatchMutating(cmd.OutOrStdout(), restOptions, "appliance.gateway.register-local", req)
		},
	}
	register.Flags().StringVar(&username, "username", "", "OAM username to register with")
	register.Flags().StringVar(&passwordFile, "password-file", "",
		"Root-only (0600) file holding the OAM password; a literal password option does not exist by design")
	parent.AddCommand(register)
	return parent
}

func credentialsCmd(restOptions *api.RESTOptions) *cobra.Command {
	parent := &cobra.Command{
		Use:   "credentials",
		Short: "Initial appliance credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("appliance credentials needs a subcommand")
			}
			return exitcode.Usagef("unknown command %q for \"loxicmd appliance credentials\"", args[0])
		},
	}
	parent.AddCommand(&cobra.Command{
		Use:   "bootstrap",
		Short: "Show the initial OAM admin credential, once, on a local console only",
		Long: `Shows the fresh image's initial OAM admin credential while it is still
valid, so the operator can log in and change it immediately. Console-only by
design: it refuses to run when the session is not an interactive terminal or
when output is redirected, and it has no JSON mode — the credential must not
land in files, logs, or inventory collection.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			if restOptions.PrintOption == "json" {
				return exitcode.Invalidf("credentials bootstrap has no JSON mode: the credential must not land in machine-collected output")
			}
			if !stdioIsTerminal() {
				return &exitcode.CLIError{
					Code:          exitcode.Precondition,
					Message:       "credentials bootstrap runs only on an interactive local console (stdin and stdout must be a terminal, not redirected)",
					ComponentCode: "console-required",
				}
			}
			req := &backend.Request{Subcommand: "credentials bootstrap"}
			return dispatchMutating(cmd.OutOrStdout(), restOptions, "appliance.credentials.bootstrap", req)
		},
	})
	return parent
}

func diagnosticsCmd(restOptions *api.RESTOptions) *cobra.Command {
	parent := &cobra.Command{
		Use:   "diagnostics",
		Short: "Appliance support diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("appliance diagnostics needs a subcommand")
			}
			return exitcode.Usagef("unknown command %q for \"loxicmd appliance diagnostics\"", args[0])
		},
	}
	var redact bool
	var output string
	create := &cobra.Command{
		Use:   "create --redact [--output PATH]",
		Short: "Create a secret-safe support archive with a receipt",
		Long: `Creates a redacted support archive through the backend: allowlisted
identity and health data only, with a post-generation secret scan the
backend must pass before reporting success. --redact is mandatory — an
unredacted mode does not exist.

Examples:
  loxicmd appliance diagnostics create --redact
  loxicmd appliance diagnostics create --redact --output /root/support.tar`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			if !redact {
				return exitcode.Usagef("--redact is required; an unredacted archive does not exist by design")
			}
			reqArgs := []string{"--redact"}
			if output != "" {
				if !filepath.IsAbs(output) {
					return exitcode.Invalidf("--output must be an absolute path, got %q", output)
				}
				reqArgs = append(reqArgs, "--output", output)
			}
			req := &backend.Request{Subcommand: "diagnostics create", Args: reqArgs}
			return dispatchMutating(cmd.OutOrStdout(), restOptions, "appliance.diagnostics.create", req)
		},
	}
	create.Flags().BoolVar(&redact, "redact", false, "Required: produce the redacted archive (the only kind there is)")
	create.Flags().StringVar(&output, "output", "", "Absolute path for the archive (default: the backend's support directory)")
	parent.AddCommand(create)
	return parent
}

func logsCmd(restOptions *api.RESTOptions) *cobra.Command {
	var redact bool
	var since string
	var lines int
	logs := &cobra.Command{
		Use:   "logs COMPONENT --redact [--since DURATION] [--lines N]",
		Short: "Retrieve bounded, redacted logs for an approved component",
		Long: `Retrieves redacted troubleshooting logs for one approved component
(gateway, oam, ui, caddy, postgres, state, dataplane, management). Bounded
by design: no follow mode, no arbitrary unit names, and --redact is
mandatory.

Examples:
  loxicmd appliance logs gateway --redact
  loxicmd appliance logs oam --redact --since 1h --lines 500`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			component := args[0]
			if !logComponents[component] {
				return exitcode.Invalidf("component %q is not in the allowlist (gateway, oam, ui, caddy, postgres, state, dataplane, management)", component)
			}
			if !redact {
				return exitcode.Usagef("--redact is required; unredacted log export does not exist by design")
			}
			reqArgs := []string{component, "--redact"}
			if since != "" {
				d, derr := time.ParseDuration(since)
				if derr != nil || d <= 0 || d > maxLogSince {
					return exitcode.Invalidf("--since must be a positive duration up to %s, got %q", maxLogSince, since)
				}
				reqArgs = append(reqArgs, "--since", since)
			}
			if lines != 0 {
				if lines < 1 || lines > maxLogLines {
					return exitcode.Invalidf("--lines must be between 1 and %d, got %d", maxLogLines, lines)
				}
				reqArgs = append(reqArgs, "--lines", strconv.Itoa(lines))
			}
			req := &backend.Request{Subcommand: "logs", Args: reqArgs}
			return dispatchMutating(cmd.OutOrStdout(), restOptions, "appliance.logs", req)
		},
	}
	logs.Flags().BoolVar(&redact, "redact", false, "Required: redact tokens, credentials, and customer content (the only mode there is)")
	logs.Flags().StringVar(&since, "since", "", "How far back to read (Go duration, up to 168h)")
	logs.Flags().IntVar(&lines, "lines", 0, "Maximum lines to return (up to 10000)")
	return logs
}

func backupCmd(restOptions *api.RESTOptions) *cobra.Command {
	parent := &cobra.Command{
		Use:   "backup",
		Short: "Encrypted appliance backups",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("appliance backup needs a subcommand")
			}
			return exitcode.Usagef("unknown command %q for \"loxicmd appliance backup\"", args[0])
		},
	}

	var newKeyFile string
	keyCreate := &cobra.Command{
		Use:   "key-create --key-file PATH",
		Short: "Create a new root-only backup encryption key",
		Long: `Creates a cryptographically secure backup key at a new absolute path,
root-only (0600). The command returns the key's path, ID, and fingerprint —
never the key value.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			if newKeyFile == "" {
				return exitcode.Usagef("--key-file is required (the new key's absolute path)")
			}
			if !filepath.IsAbs(newKeyFile) {
				return exitcode.Invalidf("--key-file must be an absolute path, got %q", newKeyFile)
			}
			if _, err := os.Lstat(newKeyFile); err == nil {
				return &exitcode.CLIError{
					Code:          exitcode.Precondition,
					Message:       fmt.Sprintf("%q already exists; key-create refuses to overwrite a key", newKeyFile),
					ComponentCode: "secret-file-refused",
				}
			}
			req := &backend.Request{Subcommand: "backup key-create", Args: []string{"--key-file", newKeyFile}}
			return dispatchMutating(cmd.OutOrStdout(), restOptions, "appliance.backup.key-create", req)
		},
	}
	keyCreate.Flags().StringVar(&newKeyFile, "key-file", "", "Absolute path for the new key; an existing file is never overwritten")
	parent.AddCommand(keyCreate)

	parent.AddCommand(backupArchiveCmd(restOptions, "create", "backup create",
		"Create an authenticated, encrypted backup archive",
		`Packages the OAM database, gateway snapshot, certificates, and an
identity manifest into an authenticated encrypted archive, written
atomically. A backup whose application-consistent scope is incomplete is
reported PARTIAL, never as a complete recovery point.`))
	parent.AddCommand(backupArchiveCmd(restOptions, "verify", "backup verify",
		"Verify a backup archive without changing anything",
		`Validates the archive's authentication, checksums, manifest, and key
match without unpacking plaintext onto persistent disk or changing
operational state.`))
	return parent
}

// backupArchiveCmd builds 'backup create' and 'backup verify', which share
// one shape: an absolute archive path plus a validated --key-file.
func backupArchiveCmd(restOptions *api.RESTOptions, use, subcommand, short, long string) *cobra.Command {
	var keyFile string
	archive := &cobra.Command{
		Use:           use + " ARCHIVE --key-file PATH",
		Short:         short,
		Long:          long,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if !filepath.IsAbs(path) {
				return exitcode.Invalidf("the archive path must be absolute, got %q", path)
			}
			if keyFile == "" {
				return exitcode.Usagef("--key-file is required")
			}
			if err := backend.CheckSecretPath("key file", keyFile); err != nil {
				return err
			}
			req := &backend.Request{Subcommand: subcommand, Args: []string{path, "--key-file", keyFile}}
			return dispatchMutating(cmd.OutOrStdout(), restOptions,
				"appliance."+strings.ReplaceAll(subcommand, " ", "."), req)
		},
	}
	archive.Flags().StringVar(&keyFile, "key-file", "", "Absolute, root-only (0600) regular file holding the backup key; symlinks are refused")
	return archive
}
