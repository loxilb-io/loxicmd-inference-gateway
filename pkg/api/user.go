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
package api

// User is the client for management-plane user accounts (/auth/users). These
// accounts obtain the bearer JWT used for all authenticated calls, including
// the AI CRUD endpoints. Requires the gateway started with --userservice.
type User struct {
	CommonAPI
}

// UserModel is the /auth/users body/response (User). role is "admin" or "viewer".
type UserModel struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	Role      string `json:"role,omitempty"`
	ID        int    `json:"id,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}
