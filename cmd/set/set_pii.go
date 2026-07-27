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

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewSetPIICmd(restOptions *api.RESTOptions) *cobra.Command {
	var enable bool
	var disable bool
	var configure bool
	var urlPatterns bool

	// configure flags
	var mode, direction, failMode, scanMode string
	var analyzerURL, anonymizerURL string
	var scoreThreshold float64
	var timeoutMs, maxBodySize, minBodySize, batchSize int64
	var enableV2 bool
	var defaultOperator, encryptionKey string

	// url-patterns flags
	var urlMode string
	var includes, excludes []string

	var setPIICmd = &cobra.Command{
		Use:   "pii (--enable | --disable | --configure | --url-patterns)",
		Short: "Configure PII detection (Presidio integration)",
		Long: `Toggle PII detection (/config/pii/enable), update its configuration
(/config/pii/configure), or manage URL scan patterns (/config/pii/url-patterns).
Only the --configure flags you set are sent; the rest keep their server values.

ex)
	loxicmd set pii --enable
	loxicmd set pii --configure --mode mask --score-threshold 0.7 --direction both
	loxicmd set pii --url-patterns --url-mode replace --include /v1/chat/* --exclude /health`,
		Run: func(cmd *cobra.Command, args []string) {
			n := 0
			for _, b := range []bool{enable, disable, configure, urlPatterns} {
				if b {
					n++
				}
			}
			if n != 1 {
				fmt.Printf("Error: specify exactly one of --enable, --disable, --configure, or --url-patterns\n")
				return
			}
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			switch {
			case enable, disable:
				req := api.PIIEnableRequest{Enabled: enable}
				resp, err := client.PII().SubResources([]string{"enable"}).Create(ctx, req)
				if enable {
					reportPost(resp, err, "PII detection enabled.")
				} else {
					reportPost(resp, err, "PII detection disabled.")
				}
			case configure:
				cfg := api.PIIConfigEntry{}
				if cmd.Flags().Changed("mode") {
					cfg.Mode = &mode
				}
				if cmd.Flags().Changed("direction") {
					cfg.Direction = &direction
				}
				if cmd.Flags().Changed("fail-mode") {
					cfg.FailMode = &failMode
				}
				if cmd.Flags().Changed("scan-mode") {
					cfg.ScanMode = &scanMode
				}
				if cmd.Flags().Changed("analyzer-url") {
					cfg.AnalyzerURL = &analyzerURL
				}
				if cmd.Flags().Changed("anonymizer-url") {
					cfg.AnonymizerURL = &anonymizerURL
				}
				if cmd.Flags().Changed("score-threshold") {
					cfg.ScoreThreshold = &scoreThreshold
				}
				if cmd.Flags().Changed("timeout-ms") {
					cfg.TimeoutMs = &timeoutMs
				}
				if cmd.Flags().Changed("max-body-size") {
					cfg.MaxBodySize = &maxBodySize
				}
				if cmd.Flags().Changed("min-body-size") {
					cfg.MinBodySize = &minBodySize
				}
				if cmd.Flags().Changed("enable-v2") {
					cfg.EnableV2 = &enableV2
				}
				if cmd.Flags().Changed("default-operator") {
					cfg.DefaultOper = &defaultOperator
				}
				if cmd.Flags().Changed("encryption-key") {
					cfg.EncryptionKey = &encryptionKey
				}
				if cmd.Flags().Changed("batch-size") {
					cfg.BatchSize = &batchSize
				}
				resp, err := client.PII().SubResources([]string{"configure"}).Create(ctx, cfg)
				reportPost(resp, err, "PII configuration updated.")
			case urlPatterns:
				if urlMode == "" {
					fmt.Printf("Error: --url-mode is required with --url-patterns (add, replace, or clear)\n")
					return
				}
				entry := api.PIIURLPatternsEntry{Mode: urlMode}
				for _, p := range includes {
					entry.Patterns = append(entry.Patterns, api.PIIURLPattern{Pattern: p, IsExclude: false})
				}
				for _, p := range excludes {
					entry.Patterns = append(entry.Patterns, api.PIIURLPattern{Pattern: p, IsExclude: true})
				}
				resp, err := client.PII().SubResources([]string{"url-patterns"}).Create(ctx, entry)
				reportPost(resp, err, "PII URL patterns updated.")
			}
		},
	}
	f := setPIICmd.Flags()
	f.BoolVar(&enable, "enable", false, "Enable PII detection")
	f.BoolVar(&disable, "disable", false, "Disable PII detection")
	f.BoolVar(&configure, "configure", false, "Update PII detection configuration")
	f.BoolVar(&urlPatterns, "url-patterns", false, "Update URL scan patterns")
	f.StringVar(&mode, "mode", "", "Detection mode: detect, mask, redact, anonymize")
	f.StringVar(&direction, "direction", "", "Scan direction: both, request, response")
	f.StringVar(&failMode, "fail-mode", "", "Behavior when Presidio is unavailable: open, closed")
	f.StringVar(&scanMode, "scan-mode", "", "Large body handling: full, truncate")
	f.StringVar(&analyzerURL, "analyzer-url", "", "Presidio analyzer gRPC endpoint")
	f.StringVar(&anonymizerURL, "anonymizer-url", "", "Presidio anonymizer gRPC endpoint")
	f.Float64Var(&scoreThreshold, "score-threshold", 0, "Minimum confidence score (0.0-1.0)")
	f.Int64Var(&timeoutMs, "timeout-ms", 0, "Presidio request timeout (ms)")
	f.Int64Var(&maxBodySize, "max-body-size", 0, "Maximum HTTP body size to scan (bytes)")
	f.Int64Var(&minBodySize, "min-body-size", 0, "Minimum HTTP body size to scan (bytes)")
	f.BoolVar(&enableV2, "enable-v2", false, "Enable Presidio v2 API (combined analyze+anonymize)")
	f.StringVar(&defaultOperator, "default-operator", "", "Default v2 operator: replace, redact, hash, mask, encrypt")
	f.StringVar(&encryptionKey, "encryption-key", "", "Base64 AES-256 key for v2 encrypt operator")
	f.Int64Var(&batchSize, "batch-size", 0, "Batch size for the v2 streaming API (1-100)")
	f.StringVar(&urlMode, "url-mode", "", "URL pattern update mode: add, replace, clear")
	f.StringArrayVar(&includes, "include", nil, "URL pattern to include (repeatable)")
	f.StringArrayVar(&excludes, "exclude", nil, "URL pattern to exclude (repeatable)")
	return setPIICmd
}
