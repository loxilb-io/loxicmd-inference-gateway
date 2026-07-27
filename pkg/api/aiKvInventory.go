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

// AIKvInventory is the client for the read-only KV-cache block-hash inventory
// (/config/ai/kv/inventory, swagger-extras raw middleware).
type AIKvInventory struct {
	CommonAPI
}

// AIKvInventoryBlock is one block entry. block_idx is a synthetic
// map-iteration sequence index, not a semantic position.
type AIKvInventoryBlock struct {
	BlockIdx   int64  `json:"block_idx"`
	HashUint64 uint64 `json:"hash_uint64"`
}

// AIKvInventoryResponse is the GET /config/ai/kv/inventory response.
type AIKvInventoryResponse struct {
	ServiceID int64                `json:"service_id"`
	EpIdx     int64                `json:"ep_idx"`
	HashAlgo  string               `json:"hash_algo"`
	Blocks    []AIKvInventoryBlock `json:"blocks"`
	Total     int64                `json:"total"`
}
