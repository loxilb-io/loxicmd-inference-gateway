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
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

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
An unauthenticated create is accepted only to bootstrap an empty user store.
After creating the first admin, run 'loxicmd set login' to obtain and store a
token; every subsequent user create requires an admin Bearer token. Management
Bearer identities are separate from data-plane X-Api-Key credentials.

ex)
	loxicmd create user --username=admin --password='<your-password>' --role=admin`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if o.Username == "" || o.Password == "" {
				return exitcode.Usagef("--username and --password are required")
			}
			req := api.UserModel{Username: o.Username, Password: o.Password, Role: o.Role}

			client := api.NewLoxiClient(restOptions)
			ctx, cancel := aiContext(restOptions)
			if cancel != nil {
				defer cancel()
			}
			resp, err := client.User().Create(ctx, req)
			if err != nil {
				return exitcode.Unavailablef("create user: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				ce := exitcode.FromHTTPStatus("create user", resp.StatusCode)
				ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
				return ce
			}
			fmt.Printf("User '%s' created.\n", o.Username)
			return nil
		},
	}

	createUserCmd.Flags().StringVar(&o.Username, "username", "", "User name (required)")
	createUserCmd.Flags().StringVar(&o.Password, "password", "", "Password (required)")
	createUserCmd.Flags().StringVar(&o.Role, "role", "", "Role: admin or viewer")

	return createUserCmd
}
