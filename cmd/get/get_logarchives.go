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

func NewGetLogArchivesCmd(restOptions *api.RESTOptions) *cobra.Command {
	var file string

	var getLogArchivesCmd = &cobra.Command{
		Use:   "log-archives [NAME]",
		Short: "List the gateway's log files and archives, or download one",
		Long: `List the gateway's active log files and rotated archives
(GET /log-archives) with their size and last-modified time, or download
one by name (GET /log-archives/NAME) into -f FILE.

An archive is transferred as stored: a .gz archive stays compressed. The
file is written through a temporary name and renamed into place with mode
0600, so a failed download never leaves a partial file under the requested
name. -f - streams the archive to stdout instead. With -o json the listing
is the gateway's body verbatim and a download reports the file it wrote.

ex)
	loxicmd get log-archives
	loxicmd get log-archives -o json
	loxicmd get log-archives gateway.log.1.gz -f gateway.log.1.gz
	loxicmd get log-archives gateway.log.1.gz -f - | gunzip | less`,
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return lifecycle.LogArchivesList(restOptions, cmd.OutOrStdout(),
					restOptions.PrintOption == "json")
			}
			return lifecycle.LogArchiveDownload(restOptions, cmd.OutOrStdout(),
				lifecycle.OptionsFrom(restOptions, false), args[0], file)
		},
	}
	getLogArchivesCmd.Flags().StringVarP(&file, "file", "f", "", "Write the downloaded archive to this file (- for stdout)")
	return getLogArchivesCmd
}
