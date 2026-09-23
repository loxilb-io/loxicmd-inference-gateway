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

// Logs is the client for GET /logs — one page of the gateway's own log,
// read backwards from the newest line.
type Logs struct {
	CommonAPI
}

// LogArchives is the client for GET /log-archives (the listing) and
// GET /log-archives/{filename} (one archive, transferred as stored).
type LogArchives struct {
	CommonAPI
}

// LogPage mirrors the /logs contract fields the CLI renders. The gateway
// emits has_more on every response, so a body without it is not a page;
// the pointer keeps that distinguishable from false.
type LogPage struct {
	Lines        []string `json:"logs"`
	LogFile      string   `json:"log_file"`
	LogCount     int      `json:"log_count"`
	TotalSize    int64    `json:"total_size"`
	HasMore      *bool    `json:"has_more"`
	NextCursor   string   `json:"next_cursor"`
	ScannedBytes int64    `json:"scanned_bytes"`
}

// LogArchiveInfo mirrors one entry of the per-archive metadata.
type LogArchiveInfo struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	Modified  string `json:"modified"`
}

// LogArchiveList mirrors the /log-archives contract. Archives is the
// original name list; ArchiveInfo carries the same names with size and
// modification time, in the same order.
type LogArchiveList struct {
	Archives    []string         `json:"archives"`
	ArchiveInfo []LogArchiveInfo `json:"archive_info"`
}

// LogArchiveFileResult is where a downloaded archive went.
type LogArchiveFileResult struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}
