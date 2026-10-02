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
		Use:   "audit-sink [NAME]",
		Short: "Replace an audit sink's configuration",
		Long: `Replace the configuration of a remote syslog sink the audit trail is
forwarded to.

With no name this is the compliance sink (POST /audit/sink), the one that is
sent every record. With a NAME it creates or replaces that secondary sink
(PUT /audit/sinks/NAME), which follows the trail beside the compliance sink.

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

A secondary sink's NAME is 1 to 64 of a-z, 0-9, '-' and '_'; 'compliance'
is reserved. It numbers what it sends, and that export sequence travels
beside each record under a private enterprise number, so --enterprise-number
is required: none is built in. It may select what it is sent: --stream
(mgmt, data, audit_system) and --service may be repeated, --outcome takes ok
or failed, and --data-sample N keeps one data record in N. --service judges
data records only, and only the data stream can be sampled. A flag left out
selects nothing away. Replacing a secondary sink keeps its place in the
trail and its export sequence. These flags are refused without a NAME: the
compliance sink is sent every record, unnumbered. --disable is refused with
one: a secondary sink is removed with 'loxicmd delete audit-sink NAME'.

If the outcome of the change cannot be confirmed (timeout, broken
connection), the command fails with reason 'recovery-required' and never
claims success - verify with 'loxicmd get audit-sink'.

ex)
	loxicmd set audit-sink --address siem.example.com:6514 --ca-bundle /etc/loxilb/siem-ca.pem
	loxicmd set audit-sink --address siem.example.com:6514 --ca-bundle /etc/loxilb/siem-ca.pem \
		--server-name siem.corp.example.com --facility 13 --max-frame-bytes 8192
	loxicmd set audit-sink --address siem.example.com:6514 --ca-bundle /etc/loxilb/siem-ca.pem \
		--client-cert /etc/loxilb/gw.pem --client-key /etc/loxilb/gw-key.pem
	loxicmd set audit-sink --disable
	loxicmd set audit-sink siem2 --address siem2.example.com:6514 --ca-bundle /etc/loxilb/siem2-ca.pem \
		--enterprise-number 32473 --stream mgmt --stream audit_system
	loxicmd set audit-sink chat-fail --address siem2.example.com:6514 --ca-bundle /etc/loxilb/siem2-ca.pem \
		--enterprise-number 32473 --stream data --service chat --outcome failed --data-sample 10`,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut := restOptions.PrintOption == "json"
			if len(args) == 1 {
				return lifecycle.AuditNamedSinkSet(restOptions, cmd.OutOrStdout(), jsonOut, args[0], opts)
			}
			return lifecycle.AuditSinkSet(restOptions, cmd.OutOrStdout(), jsonOut, opts)
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
	setAuditSinkCmd.Flags().Int64Var(&opts.EnterpriseNumber, "enterprise-number", 0,
		"IANA private enterprise number the export sequence travels under (secondary sink: required)")
	setAuditSinkCmd.Flags().StringArrayVar(&opts.Streams, "stream", nil,
		"Secondary sink: keep records of this stream - mgmt, data or audit_system (repeatable; default all)")
	setAuditSinkCmd.Flags().StringArrayVar(&opts.Services, "service", nil,
		"Secondary sink: keep data records of this service (repeatable; default all)")
	setAuditSinkCmd.Flags().StringVar(&opts.Outcome, "outcome", "",
		"Secondary sink: keep records whose outcome is ok, or failed (default both)")
	setAuditSinkCmd.Flags().Int64Var(&opts.DataSample, "data-sample", 0,
		"Secondary sink: keep one data record in this many (0 and 1 keep all)")
	return setAuditSinkCmd
}
