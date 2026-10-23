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
package api

// AIObjective is the client for the named request priorities
// (/config/ai/objective): POST creates or replaces one, GET lists them,
// DELETE /{name} removes one. There is no PUT or PATCH.
type AIObjective struct {
	CommonAPI
}

// AIObjectiveEntry is the swagger AIObjectiveEntry: a named priority a
// request can be assigned. priority is a signed integer, positive the
// critical class, negative sheddable, zero standard; it is always sent.
// services limits the objective to the listed "VIP:port" services, empty
// means every service. updated_at is read back only.
type AIObjectiveEntry struct {
	Name      string   `json:"name"`
	Services  []string `json:"services,omitempty"`
	Priority  int32    `json:"priority"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

// AIObjectiveList is the GET /config/ai/objective answer: the objectives
// in name order, where the list came from (store, or the last-known-good
// file during an outage) and the key-store read it reflects.
type AIObjectiveList struct {
	Objectives      []AIObjectiveEntry `json:"objectiveAttr"`
	Source          string             `json:"source,omitempty"`
	SourceTimestamp string             `json:"source_timestamp,omitempty"`
	StoreReachable  bool               `json:"store_reachable"`
	StoreLastRead   string             `json:"store_last_read,omitempty"`
}
