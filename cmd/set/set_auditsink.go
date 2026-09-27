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
package set

import (
	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/lifecycle"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewSetAuditSinkCmd(restOptions *api.RESTOptions) *cobra.Command {
	var opts lifecycle.AuditSinkSetOptions

	var setAuditSinkCmd = &cobra.Command{
		Use:   "audit-sink",
		Short: "Replace the remote audit sink configuration",
		Long: `Replace the configuration of the remote syslog sink the audit trail is
forwarded to (POST /audit/sink).

This REPLACES the configuration; it does not patch it. The endpoint takes a
whole sink, so every invocation sends one: a flag you leave out is sent as
its default, not kept from what the gateway had. To change one field, pass
the other fields again. Read the current configuration first with
'loxicmd get audit-sink'.

--address and --ca-bundle are required to configure a sink. The receiver's
certificate is always verified against the bundle and there is no mode that
disables verification, so there is no default the CLI could supply for it.
The bundle is a path on the GATEWAY's filesystem, not this host's, and the
gateway refuses a bundle it cannot read. --client-cert and --client-key are
for mutual TLS and must be given together or not at all.

--disable removes the sink: the gateway closes the session, forgets the
configuration, and the trail continues locally. Records already written stay
on disk; forwarding simply stops.

--facility takes 1-23. The gateway reads a facility of 0 as "not set" and
substitutes 13, so facility 0 (kernel) cannot be selected through this API;
the flag is documented as 1-23 rather than promising a value that does not
arrive.

If the outcome of the change cannot be confirmed (timeout, broken
connection), the command fails with reason 'recovery-required' and never
claims success - verify with 'loxicmd get audit-sink'.

ex)
	loxicmd set audit-sink --address siem.example.com:6514 --ca-bundle /etc/loxilb/siem-ca.pem
	loxicmd set audit-sink --address siem.example.com:6514 --ca-bundle /etc/loxilb/siem-ca.pem \
		--server-name siem.corp.example.com --facility 13 --max-frame-bytes 8192
	loxicmd set audit-sink --address siem.example.com:6514 --ca-bundle /etc/loxilb/siem-ca.pem \
		--client-cert /etc/loxilb/gw.pem --client-key /etc/loxilb/gw-key.pem
	loxicmd set audit-sink --disable`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return lifecycle.AuditSinkSet(restOptions, cmd.OutOrStdout(),
				restOptions.PrintOption == "json", opts)
		},
	}
	setAuditSinkCmd.Flags().BoolVar(&opts.Disable, "disable", false,
		"Remove the sink: close the session and stop forwarding (the local trail continues)")
	setAuditSinkCmd.Flags().StringVar(&opts.Address, "address", "",
		"Receiver host:port (required unless --disable)")
	setAuditSinkCmd.Flags().StringVar(&opts.CABundlePath, "ca-bundle", "",
		"PEM bundle on the gateway verifying the receiver's certificate (required unless --disable)")
	setAuditSinkCmd.Flags().StringVar(&opts.ServerName, "server-name", "",
		"Name expected in the receiver's certificate (default: the host part of --address)")
	setAuditSinkCmd.Flags().StringVar(&opts.ClientCertPath, "client-cert", "",
		"Client certificate on the gateway for mutual TLS (with --client-key)")
	setAuditSinkCmd.Flags().StringVar(&opts.ClientKeyPath, "client-key", "",
		"Client key on the gateway for mutual TLS (with --client-cert)")
	setAuditSinkCmd.Flags().Int64Var(&opts.MaxFrameBytes, "max-frame-bytes", 0,
		"Largest message the receiver accepts; a record that does not fit is shortened at a field boundary and marked (0 = no limit)")
	setAuditSinkCmd.Flags().Int64Var(&opts.Facility, "facility", lifecycle.DefaultSyslogFacility,
		"Syslog facility 1-23 (default 13, log audit; the gateway reads 0 as unset and uses 13)")
	return setAuditSinkCmd
}
