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
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
)

// v2Context builds a request context honoring the shared --timeout flag.
func v2Context(restOptions *api.RESTOptions) (context.Context, context.CancelFunc) {
	if restOptions.Timeout > 0 {
		return context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
	}
	return context.TODO(), func() {}
}

// reportPost checks a mutating call's outcome and prints successMsg on success.
// A 200 or 204 is treated as success; anything else is decoded through
// APIError (which handles both the main Error and the extras SimpleError
// envelopes) and classified for the caller's taxonomy exit. On success and
// when the body is non-empty it is also printed, so server-side confirmation
// payloads (e.g. health checks, restore plans) surface.
func reportPost(resp *http.Response, err error, successMsg string) error {
	if err != nil {
		return exitcode.Unavailablef("%v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		ce := exitcode.FromHTTPStatus("set", resp.StatusCode)
		ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
		return ce
	}
	fmt.Println(successMsg)
	return nil
}
