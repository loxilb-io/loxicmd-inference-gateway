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
	"fmt"
	"io"
	"net/http"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

type CreateUserOptions struct {
	Username string
	Password string
	Role     string
}

func NewCreateUserCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := CreateUserOptions{}

	var createUserCmd = &cobra.Command{
		Use:   "user --username=<name> --password=<pass> [--role=admin|viewer]",
		Short: "Create a management-plane user account",
		Long: `Create a user account used to obtain the bearer JWT for authenticated calls.

Requires the gateway started with --userservice and a database backend.
After creating a user, run 'loxicmd set login' to obtain and store a token.

ex)
	loxicmd create user --username=admin --password='<your-password>' --role=admin`,
		Run: func(cmd *cobra.Command, args []string) {
			if o.Username == "" || o.Password == "" {
				fmt.Printf("Error: --username and --password are required\n")
				return
			}
			req := api.UserModel{Username: o.Username, Password: o.Password, Role: o.Role}

			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.User().Create(ctx, req)
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				fmt.Printf("Error: %s\n", api.NewAPIError(resp.StatusCode, body).Error())
				return
			}
			fmt.Printf("User '%s' created.\n", o.Username)
		},
	}

	createUserCmd.Flags().StringVar(&o.Username, "username", "", "User name (required)")
	createUserCmd.Flags().StringVar(&o.Password, "password", "", "Password (required)")
	createUserCmd.Flags().StringVar(&o.Role, "role", "", "Role: admin or viewer")

	return createUserCmd
}
