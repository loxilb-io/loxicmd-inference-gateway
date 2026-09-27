/*
 * Copyright (c) 2026 NetLOX Inc
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

import "testing"

// A declared value is shown as is; an undeclared one (0) with a process
// default in force shows the default beside it; with neither, 0.
func TestFcQueueCell(t *testing.T) {
	cases := []struct {
		declared, effective uint32
		want                string
	}{
		{4, 4, "4"},
		{4, 16, "4"},
		{0, 16, "0 (default 16)"},
		{0, 0, "0"},
	}
	for _, c := range cases {
		if got := fcQueueCell(c.declared, c.effective); got != c.want {
			t.Errorf("fcQueueCell(%d, %d) = %q, want %q", c.declared, c.effective, got, c.want)
		}
	}
}
