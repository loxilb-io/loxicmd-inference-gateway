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
	"bytes"
	"context"
	"encoding/json"
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

// printJSONResponse reads an HTTP response and pretty-prints its JSON body.
// The v2 telemetry endpoints (gpu/pii/llamafirewall/trace/l4trace/opa/dpu)
// return richly nested documents, so the CLI renders the raw JSON rather than
// modeling every field. A non-2xx status is decoded through APIError, which
// understands both the main Error and the extras SimpleError envelopes, and
// classified for the caller's taxonomy exit.
func printJSONResponse(resp *http.Response, what string) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		ce := exitcode.FromHTTPStatus(what, resp.StatusCode)
		ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
		return ce
	}
	if len(bytes.TrimSpace(body)) == 0 {
		fmt.Printf("%s: (empty response)\n", what)
		return nil
	}
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "    "); err != nil {
		// Not JSON — print as-is.
		fmt.Println(string(body))
		return nil
	}
	fmt.Println(out.String())
	return nil
}
