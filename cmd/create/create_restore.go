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
package create

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

func NewCreateRestoreCmd(restOptions *api.RESTOptions) *cobra.Command {
	var file string
	var commit bool

	var createRestoreCmd = &cobra.Command{
		Use:   "restore -f <snapshot-file> [--commit]",
		Short: "Restore an instance configuration snapshot",
		Long: `Run the staged restore pipeline on a snapshot document produced by
'loxicmd get snapshot' (/config/restore). The default mode is dry-run, which
validates and returns the plan without mutating anything; pass --commit to
apply the snapshot (with automatic rollback on failure).

ex)
	loxicmd create restore -f snapshot.json
	loxicmd create restore -f snapshot.json --commit`,
		Run: func(cmd *cobra.Command, args []string) {
			if file == "" {
				fmt.Printf("Error: -f/--file is required (the snapshot document to restore)\n")
				return
			}
			data, err := os.ReadFile(file)
			if err != nil {
				fmt.Printf("Error: Failed to read snapshot file: %s\n", err.Error())
				return
			}
			if !json.Valid(data) {
				fmt.Printf("Error: %s is not valid JSON\n", file)
				return
			}

			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}

			mode := "dry-run"
			if commit {
				mode = "commit"
			}
			resp, err := client.Restore().Query(map[string]string{"mode": mode}).Create(ctx, json.RawMessage(data))
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			// The restore endpoint returns RestoreResult for 200/400/500 — print
			// the plan/result either way so dry-run and failures are legible.
			var out bytes.Buffer
			if json.Indent(&out, body, "", "    ") == nil {
				fmt.Println(out.String())
			} else {
				fmt.Println(string(body))
			}
			if resp.StatusCode != http.StatusOK {
				fmt.Printf("(restore returned HTTP %d)\n", resp.StatusCode)
			}
		},
	}
	createRestoreCmd.Flags().StringVarP(&file, "file", "f", "", "Snapshot document to restore (required)")
	createRestoreCmd.Flags().BoolVar(&commit, "commit", false, "Apply the snapshot (default is dry-run)")
	return createRestoreCmd
}
