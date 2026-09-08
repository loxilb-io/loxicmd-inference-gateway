/*
 * Copyright (c) 2022 NetLOX Inc
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
package delete

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

func DeleteMirrorValidation(args []string) error {
	if len(args) > 1 {
		fmt.Println("delete Mirror command get so many args")
	}
	return nil
}

func NewDeleteMirrorCmd(restOptions *api.RESTOptions) *cobra.Command {

	var deleteMirrorCmd = &cobra.Command{
		Use:     "mirror <MirrorIdent>",
		Short:   "Delete a Mirror",
		Long:    `Delete a Mirror using MirrorIdent in the LoxiLB.`,
		Aliases: []string{"mirror", "mirr", "mirrors"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("delete mirror needs its arguments")
			}
			if err := DeleteMirrorValidation(args); err != nil {
				return exitcode.Invalidf("not valid <MirrorIdent>")
			}
			MirrorIdent := args[0]

			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			subResources := []string{"ident", MirrorIdent}
			resp, err := client.Mirror().SubResources(subResources).Delete(ctx)
			if err != nil {
				return exitcode.Unavailablef("Failed to delete Mirror : %s (%v)", MirrorIdent, err)
			}
			defer resp.Body.Close()
			fmt.Printf("Debug: response.StatusCode: %d\n", resp.StatusCode)
			if resp.StatusCode == http.StatusOK {
				PrintDeleteResult(resp, *restOptions)
				return nil
			}
			return exitcode.FromHTTPStatus("delete mirror", resp.StatusCode)
		},
	}

	return deleteMirrorCmd
}
