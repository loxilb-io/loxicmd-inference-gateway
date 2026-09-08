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
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

func NewSetLlamaFirewallCmd(restOptions *api.RESTOptions) *cobra.Command {
	var enable bool
	var disable bool
	var configure bool
	var scanners bool
	var health bool

	// configure flags
	var serverURL string
	var timeoutSec, cacheTTLSec, connPoolSize int64
	var failClosed, cacheEnabled bool
	var blockThreshold float64
	var scanPatterns, skipPatterns []string

	// scanner toggles
	var promptGuard, codeShield, regex, hiddenASCII, agentAlignment, piiDetection bool

	var setCmd = &cobra.Command{
		Use:   "llamafirewall (--enable | --disable | --configure | --scanners | --health)",
		Short: "Configure LlamaFirewall AI-security scanning",
		Long: `Toggle LlamaFirewall scanning (/config/llamafirewall/enable), update its
configuration (/config/llamafirewall/configure), enable/disable individual
scanners (/config/llamafirewall/scanners), or run a health check
(/config/llamafirewall/health). Only the flags you set are sent.

ex)
	loxicmd set llamafirewall --enable
	loxicmd set llamafirewall --configure --server-url localhost:50052 --block-threshold 0.9
	loxicmd set llamafirewall --scanners --prompt-guard --code-shield
	loxicmd set llamafirewall --health`,
		RunE: func(cmd *cobra.Command, args []string) error {
			n := 0
			for _, b := range []bool{enable, disable, configure, scanners, health} {
				if b {
					n++
				}
			}
			if n != 1 {
				return exitcode.Usagef("specify exactly one of --enable, --disable, --configure, --scanners, or --health")
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			switch {
			case enable, disable:
				req := api.LlamaFirewallEnableRequest{Enabled: enable}
				resp, err := client.LlamaFirewall().SubResources([]string{"enable"}).Create(ctx, req)
				if enable {
					return reportPost(resp, err, "LlamaFirewall scanning enabled.")
				}
				return reportPost(resp, err, "LlamaFirewall scanning disabled.")
			case configure:
				cfg := api.LlamaFirewallConfigEntry{}
				if cmd.Flags().Changed("server-url") {
					cfg.ServerURL = &serverURL
				}
				if cmd.Flags().Changed("timeout-sec") {
					cfg.TimeoutSec = &timeoutSec
				}
				if cmd.Flags().Changed("fail-closed") {
					cfg.FailClosed = &failClosed
				}
				if cmd.Flags().Changed("block-threshold") {
					cfg.BlockThreshold = &blockThreshold
				}
				if cmd.Flags().Changed("cache-enabled") {
					cfg.CacheEnabled = &cacheEnabled
				}
				if cmd.Flags().Changed("cache-ttl-sec") {
					cfg.CacheTTLSec = &cacheTTLSec
				}
				if cmd.Flags().Changed("connection-pool-size") {
					cfg.ConnectionPoolSize = &connPoolSize
				}
				if cmd.Flags().Changed("scan-pattern") {
					cfg.ScanPatterns = scanPatterns
				}
				if cmd.Flags().Changed("skip-pattern") {
					cfg.SkipPatterns = skipPatterns
				}
				resp, err := client.LlamaFirewall().SubResources([]string{"configure"}).Create(ctx, cfg)
				return reportPost(resp, err, "LlamaFirewall configuration updated.")
			case scanners:
				sc := api.LlamaFirewallScannersEntry{}
				if cmd.Flags().Changed("prompt-guard") {
					sc.PromptGuard = &promptGuard
				}
				if cmd.Flags().Changed("code-shield") {
					sc.CodeShield = &codeShield
				}
				if cmd.Flags().Changed("regex") {
					sc.Regex = &regex
				}
				if cmd.Flags().Changed("hidden-ascii") {
					sc.HiddenASCII = &hiddenASCII
				}
				if cmd.Flags().Changed("agent-alignment") {
					sc.AgentAlignment = &agentAlignment
				}
				if cmd.Flags().Changed("pii-detection") {
					sc.PIIDetection = &piiDetection
				}
				resp, err := client.LlamaFirewall().SubResources([]string{"scanners"}).Create(ctx, sc)
				return reportPost(resp, err, "LlamaFirewall scanners updated.")
			case health:
				resp, err := client.LlamaFirewall().SubResources([]string{"health"}).Create(ctx, nil)
				return reportPost(resp, err, "LlamaFirewall health check completed.")
			}
			return nil
		},
	}
	f := setCmd.Flags()
	f.BoolVar(&enable, "enable", false, "Enable LlamaFirewall scanning")
	f.BoolVar(&disable, "disable", false, "Disable LlamaFirewall scanning")
	f.BoolVar(&configure, "configure", false, "Update LlamaFirewall configuration")
	f.BoolVar(&scanners, "scanners", false, "Enable/disable individual scanners")
	f.BoolVar(&health, "health", false, "Trigger a health check against the gRPC server")
	f.StringVar(&serverURL, "server-url", "", "LlamaFirewall gRPC server URL")
	f.Int64Var(&timeoutSec, "timeout-sec", 0, "Request timeout in seconds (1-300)")
	f.BoolVar(&failClosed, "fail-closed", false, "Block on scanner error (fail-closed) instead of allow")
	f.Float64Var(&blockThreshold, "block-threshold", 0, "Minimum confidence score to block (0.0-1.0)")
	f.BoolVar(&cacheEnabled, "cache-enabled", false, "Enable response caching for identical requests")
	f.Int64Var(&cacheTTLSec, "cache-ttl-sec", 0, "Cache TTL in seconds")
	f.Int64Var(&connPoolSize, "connection-pool-size", 0, "Number of reusable gRPC connections (1-100)")
	f.StringArrayVar(&scanPatterns, "scan-pattern", nil, "URL pattern to scan (repeatable; empty = scan all)")
	f.StringArrayVar(&skipPatterns, "skip-pattern", nil, "URL pattern to skip scanning (repeatable)")
	f.BoolVar(&promptGuard, "prompt-guard", false, "PromptGuard scanner (prompt-injection detection)")
	f.BoolVar(&codeShield, "code-shield", false, "CodeShield scanner (insecure code detection)")
	f.BoolVar(&regex, "regex", false, "Regex scanner (credential/API-key leak detection)")
	f.BoolVar(&hiddenASCII, "hidden-ascii", false, "HiddenASCII scanner (invisible-character detection)")
	f.BoolVar(&agentAlignment, "agent-alignment", false, "AgentAlignment scanner (agent-misalignment detection)")
	f.BoolVar(&piiDetection, "pii-detection", false, "PII Detection scanner (complementary to Presidio)")
	return setCmd
}
