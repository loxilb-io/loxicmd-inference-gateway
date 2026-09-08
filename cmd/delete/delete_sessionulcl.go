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
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

type DeleteSessionUlClOptions struct {
	UserID   string
	UlClArgs []string
}

func DeleteSessionUlClValidation(args []string) error {
	if len(args) > 1 {
		fmt.Println("delete Session command get so many args")
		fmt.Println(args)
	} else if len(args) <= 0 {
		return errors.New("delete Session need <UserID> args")
	}

	return nil
}

func NewDeleteSessionUlClCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := DeleteSessionUlClOptions{}

	var deleteLbCmd = &cobra.Command{
		Use:     "sessionulcl <UserID> --ulclArgs=<UlCLIP>,...",
		Short:   "Delete a Ulcl configuration in the LoxiLB.",
		Long:    `Delete a Ulcl configuration in the LoxiLB.`,
		Aliases: []string{"ulcl", "sessionulcls", "ulcls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return exitcode.Usagef("delete sessionulcl needs its arguments")
			}
			if err := DeleteSessionUlClValidation(args); err != nil {
				return exitcode.Invalidf("not valid <UserID>")
			}

			o.UserID = args[0]
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			for _, ulclIP := range o.UlClArgs {
				subResources := []string{
					"ident", o.UserID,
					"ulclAddress", ulclIP,
				}
				resp, err := client.SessionUlCL().SubResources(subResources).Delete(ctx)
				if err != nil {
					return exitcode.Unavailablef("Failed to delete Session(UserID: %s) (%v)", o.UserID, err)
				}
				defer resp.Body.Close()
				fmt.Printf("Debug: response.StatusCode: %d\n", resp.StatusCode)
				if resp.StatusCode != http.StatusOK {
					return exitcode.FromHTTPStatus("delete sessionulcl", resp.StatusCode)
				}
				PrintDeleteResult(resp, *restOptions)
			}
			return nil
		},
	}
	deleteLbCmd.Flags().StringSliceVar(&o.UlClArgs, "ulclArgs", o.UlClArgs, "UlCl IP address can be specified as '<UlClIP>'. It don't need qfi.")

	return deleteLbCmd
}
