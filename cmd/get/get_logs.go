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
package get

import (
	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/lifecycle"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewGetLogsCmd(restOptions *api.RESTOptions) *cobra.Command {
	var opts lifecycle.LogsOptions

	var getLogsCmd = &cobra.Command{
		Use:   "logs",
		Short: "Show the newest lines of the gateway's own log",
		Long: `Show the newest lines of the gateway's own log file (GET /logs),
printed oldest to newest so they read like the tail of the file.

The gateway reads the file backwards from its end. --level and --keyword
are case-sensitive substring filters combined with AND; with a filter set
the gateway keeps scanning back until --lines matching lines are found,
the start of the file is reached, or its per-request scan budget is spent.
When older lines remain, the last line of output names the --file and
--cursor to pass for the next (older) page; keep the filters unchanged
from page to page. --file reads one of the files listed by
'loxicmd get log-archives' instead of the current log. With -o json the
gateway's body is printed verbatim.

ex)
	loxicmd get logs
	loxicmd get logs --lines 500 --level ERROR
	loxicmd get logs --keyword timeout --file gateway.log.1.gz
	loxicmd get logs -o json`,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return lifecycle.LogsGet(restOptions, cmd.OutOrStdout(),
				restOptions.PrintOption == "json", opts)
		},
	}
	getLogsCmd.Flags().IntVar(&opts.Lines, "lines", 100, "Matching lines to return (up to 10000)")
	getLogsCmd.Flags().StringVar(&opts.Level, "level", "", "Keep only lines containing this level text (case-sensitive)")
	getLogsCmd.Flags().StringVar(&opts.Keyword, "keyword", "", "Keep only lines containing this text (case-sensitive)")
	getLogsCmd.Flags().StringVar(&opts.File, "file", "", "Log file to read, by name as listed by 'get log-archives' (default: the current log)")
	getLogsCmd.Flags().StringVar(&opts.Cursor, "cursor", "", "Cursor printed by the previous page, for the next older page")
	return getLogsCmd
}
