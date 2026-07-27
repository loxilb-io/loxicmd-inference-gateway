/*
 * Copyright (c) 2025 LoxiLB Authors
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
	"fmt"
	"strings"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewSetTraceCmd(restOptions *api.RESTOptions) *cobra.Command {
	var enable bool
	var disable bool
	var otlp bool

	var endpoint, protocol string
	var useTLS, tlsSkipVerify bool
	var headers []string

	var setTraceCmd = &cobra.Command{
		Use:   "trace (--enable | --disable | --otlp)",
		Short: "Configure HTTP/HTTPS request tracing",
		Long: `Toggle request tracing (/config/trace/enable|disable) or configure the
OTLP exporter (/config/trace/otlp).

ex)
	loxicmd set trace --enable
	loxicmd set trace --disable
	loxicmd set trace --otlp --otlp-endpoint jaeger.example.com:4317 --otlp-protocol grpc`,
		Run: func(cmd *cobra.Command, args []string) {
			n := 0
			for _, b := range []bool{enable, disable, otlp} {
				if b {
					n++
				}
			}
			if n != 1 {
				fmt.Printf("Error: specify exactly one of --enable, --disable, or --otlp\n")
				return
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			switch {
			case enable:
				resp, err := client.Trace().SubResources([]string{"enable"}).Create(ctx, nil)
				reportPost(resp, err, "HTTP/HTTPS tracing enabled.")
			case disable:
				resp, err := client.Trace().SubResources([]string{"disable"}).Create(ctx, nil)
				reportPost(resp, err, "HTTP/HTTPS tracing disabled.")
			case otlp:
				if endpoint == "" || protocol == "" {
					fmt.Printf("Error: --otlp requires --otlp-endpoint and --otlp-protocol\n")
					return
				}
				cfg := api.TraceOTLPConfig{Endpoint: endpoint, Protocol: protocol}
				if cmd.Flags().Changed("otlp-use-tls") {
					cfg.UseTLS = &useTLS
				}
				if cmd.Flags().Changed("otlp-tls-skip-verify") {
					cfg.TLSSkipVerify = &tlsSkipVerify
				}
				if len(headers) > 0 {
					cfg.Headers = map[string]string{}
					for _, h := range headers {
						kv := strings.SplitN(h, "=", 2)
						if len(kv) != 2 {
							fmt.Printf("Error: --otlp-header must be key=value, got %q\n", h)
							return
						}
						cfg.Headers[kv[0]] = kv[1]
					}
				}
				resp, err := client.Trace().SubResources([]string{"otlp"}).Create(ctx, cfg)
				reportPost(resp, err, "OTLP endpoint configured.")
			}
		},
	}
	f := setTraceCmd.Flags()
	f.BoolVar(&enable, "enable", false, "Enable HTTP/HTTPS tracing")
	f.BoolVar(&disable, "disable", false, "Disable HTTP/HTTPS tracing")
	f.BoolVar(&otlp, "otlp", false, "Configure the OTLP exporter endpoint")
	f.StringVar(&endpoint, "otlp-endpoint", "", "OTLP exporter endpoint (host:port)")
	f.StringVar(&protocol, "otlp-protocol", "", "OTLP protocol: grpc or http")
	f.BoolVar(&useTLS, "otlp-use-tls", true, "Use TLS to the OTLP endpoint")
	f.BoolVar(&tlsSkipVerify, "otlp-tls-skip-verify", false, "Skip TLS verification to the OTLP endpoint")
	f.StringArrayVar(&headers, "otlp-header", nil, "OTLP exporter header key=value (repeatable)")
	return setTraceCmd
}
