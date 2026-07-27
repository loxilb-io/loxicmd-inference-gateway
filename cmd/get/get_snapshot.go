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
package get

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewGetSnapshotCmd(restOptions *api.RESTOptions) *cobra.Command {
	var components string
	var file string

	var getSnapshotCmd = &cobra.Command{
		Use:   "snapshot",
		Short: "Download a complete instance configuration snapshot",
		Long: `Download the versioned, checksummed snapshot document covering all v1
configuration domains (/config/snapshot). Restore it later with
'loxicmd create restore -f <file>'.

ex)
	loxicmd get snapshot -f snapshot.json
	loxicmd get snapshot --components loadbalancer,endpoint`,
		Run: func(cmd *cobra.Command, args []string) {
			client := api.NewLoxiClient(restOptions)
			ctx, cancel := v2Context(restOptions)
			defer cancel()

			s := client.Snapshot()
			snap := &s.CommonAPI
			if components != "" {
				snap = snap.Query(map[string]string{"components": components})
			}
			resp, err := snap.Get(ctx)
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusOK {
				fmt.Printf("Error: %s\n", api.NewAPIError(resp.StatusCode, body).Error())
				return
			}
			if file != "" {
				if err := os.WriteFile(file, body, 0600); err != nil {
					fmt.Printf("Error: Failed to write snapshot file: %s\n", err.Error())
					return
				}
				fmt.Printf("Snapshot written to %s (%d bytes).\n", file, len(body))
				return
			}
			fmt.Println(string(body))
		},
	}
	getSnapshotCmd.Flags().StringVar(&components, "components", "", "Comma-separated v1 domains to capture (default: all)")
	getSnapshotCmd.Flags().StringVarP(&file, "file", "f", "", "Write the snapshot to this file instead of stdout")
	return getSnapshotCmd
}
