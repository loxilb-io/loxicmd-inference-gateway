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
	"context"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// aiContext builds a context with the standard REST timeout.
func aiContext(restOptions *api.RESTOptions) (context.Context, context.CancelFunc) {
	if restOptions.Timeout > 0 {
		return context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
	}
	return context.TODO(), nil
}
