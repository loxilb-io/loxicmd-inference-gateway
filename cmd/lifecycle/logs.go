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
package lifecycle

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
)

// MaxLogLines bounds one page of the gateway log. The gateway applies no
// upper bound of its own, so the CLI holds the same ceiling the appliance
// log command does.
const MaxLogLines = 10000

// LogsOptions are the /logs query inputs.
type LogsOptions struct {
	// Lines is the number of matching lines wanted on this page.
	Lines int
	// Level and Keyword are case-sensitive substring filters, combined
	// with AND by the gateway.
	Level   string
	Keyword string
	// File selects one of the files the gateway lists under /log-archives
	// instead of the current log.
	File string
	// Cursor continues a previous page towards older lines.
	Cursor string
}

// checkLogName refuses anything but a bare file name. The gateway resolves
// the name inside its own log directories; a path here can only be an
// attempt to point it elsewhere, so it is refused before any request.
func checkLogName(name, what string) error {
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, `/\`) || name != path.Base(name) {
		return &api.LifecycleError{
			Reason:  api.ReasonInvalidArguments,
			Message: fmt.Sprintf("%s must be a bare file name, got %q", what, name),
		}
	}
	return nil
}

// LogsGet reads one page of the gateway's own log (GET /logs). The gateway
// serves the page newest first; the human rendering prints it oldest first
// so it reads like the tail of the file. JSON mode prints the gateway's
// body verbatim.
func LogsGet(restOptions *api.RESTOptions, out io.Writer, jsonOut bool, lo LogsOptions) error {
	if lo.Lines < 1 || lo.Lines > MaxLogLines {
		return &api.LifecycleError{
			Reason:  api.ReasonInvalidArguments,
			Message: fmt.Sprintf("--lines must be between 1 and %d, got %d", MaxLogLines, lo.Lines),
		}
	}
	if lo.File != "" {
		if err := checkLogName(lo.File, "--file"); err != nil {
			return err
		}
	}
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	query := map[string]string{"lines": strconv.Itoa(lo.Lines)}
	for k, v := range map[string]string{
		"level": lo.Level, "keyword": lo.Keyword, "file": lo.File, "cursor": lo.Cursor,
	} {
		if v != "" {
			query[k] = v
		}
	}
	client := api.NewLoxiClient(restOptions)
	resp, err := client.Logs().Query(query).Get(ctx)
	if err != nil {
		return transportError("logs request failed", err)
	}
	defer resp.Body.Close()
	body, err := readBody(resp.Body, "logs")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return api.NewStatusError(resp.StatusCode, body)
	}
	var page api.LogPage
	if json.Unmarshal(body, &page) != nil || page.HasMore == nil {
		return &api.LifecycleError{
			Reason:  api.ReasonDecodeFailed,
			Message: "the gateway's logs response could not be decoded",
			Body:    string(body),
		}
	}
	if jsonOut {
		_, _ = out.Write(append(body, '\n'))
		return nil
	}
	humanLogs(out, &page, lo)
	return nil
}

func humanLogs(out io.Writer, page *api.LogPage, lo LogsOptions) {
	for i := len(page.Lines) - 1; i >= 0; i-- {
		fmt.Fprintln(out, page.Lines[i])
	}
	file := page.LogFile
	if file == "" {
		file = lo.File
	}
	if len(page.Lines) == 0 {
		if file != "" {
			fmt.Fprintf(out, "No matching lines in %s.\n", file)
		} else {
			fmt.Fprintln(out, "No matching lines.")
		}
	}
	if *page.HasMore && page.NextCursor != "" {
		// The cursor does not carry the file or the filters, so the hint
		// repeats them: the next page must ask the same question.
		next := []string{"loxicmd get logs"}
		if file != "" {
			next = append(next, "--file "+file)
		}
		if lo.Level != "" {
			next = append(next, "--level "+lo.Level)
		}
		if lo.Keyword != "" {
			next = append(next, "--keyword "+lo.Keyword)
		}
		next = append(next, "--cursor "+page.NextCursor)
		fmt.Fprintf(out, "-- older lines remain: %s\n", strings.Join(next, " "))
	}
}

// LogArchivesList lists the gateway's log files and rotated archives
// (GET /log-archives). JSON mode prints the gateway's body verbatim.
func LogArchivesList(restOptions *api.RESTOptions, out io.Writer, jsonOut bool) error {
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.LogArchives().Get(ctx)
	if err != nil {
		return transportError("log archives request failed", err)
	}
	defer resp.Body.Close()
	body, err := readBody(resp.Body, "log archives")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return api.NewStatusError(resp.StatusCode, body)
	}
	// An empty listing is a legitimate answer with every field absent, so
	// the only shape check possible is "a JSON object".
	var probe map[string]json.RawMessage
	var list api.LogArchiveList
	if json.Unmarshal(body, &probe) != nil || json.Unmarshal(body, &list) != nil {
		return &api.LifecycleError{
			Reason:  api.ReasonDecodeFailed,
			Message: "the gateway's log archives response could not be decoded",
			Body:    string(body),
		}
	}
	if jsonOut {
		_, _ = out.Write(append(body, '\n'))
		return nil
	}
	humanLogArchives(out, &list)
	return nil
}

func humanLogArchives(out io.Writer, list *api.LogArchiveList) {
	names := list.Archives
	if len(names) == 0 {
		for _, a := range list.ArchiveInfo {
			names = append(names, a.Name)
		}
	}
	if len(names) == 0 {
		fmt.Fprintln(out, "No log archives.")
		return
	}
	info := make(map[string]api.LogArchiveInfo, len(list.ArchiveInfo))
	for _, a := range list.ArchiveInfo {
		info[a.Name] = a
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSIZE\tMODIFIED")
	for _, n := range names {
		size, modified := "-", "-"
		if a, ok := info[n]; ok {
			size = strconv.FormatInt(a.SizeBytes, 10)
			if a.Modified != "" {
				modified = a.Modified
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", n, size, modified)
	}
	w.Flush()
}

// readTracker remembers the first error a reader produced, so a download
// that stops because the gateway's response broke off is reported as the
// transport failure it is, not as a local write failure.
type readTracker struct {
	r   io.Reader
	err error
}

func (t *readTracker) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if err != nil && err != io.EOF && t.err == nil {
		t.err = err
	}
	return n, err
}

// LogArchiveDownload fetches one log file or archive by name
// (GET /log-archives/{filename}) and stores it at file, or streams it onto
// out when file is "-". The archive is transferred as stored.
func LogArchiveDownload(restOptions *api.RESTOptions, out io.Writer, o Options, name, file string) error {
	const command = "get.log-archives"
	report := &api.LifecycleReport{}
	if err := checkLogName(name, "the archive name"); err != nil {
		return render(out, o, command, report, nil, err)
	}
	if file == "" {
		return render(out, o, command, report, nil, &api.LifecycleError{
			Reason:  api.ReasonInvalidArguments,
			Message: "an archive is downloaded into a file: pass -f FILE, or -f - to stream it to stdout",
		})
	}
	if file == "-" && o.JSON {
		return render(out, o, command, report, nil, &api.LifecycleError{
			Reason:  api.ReasonInvalidArguments,
			Message: "-f - streams the archive to stdout, where -o json writes its result; choose one",
		})
	}
	ctx, cancel := requestContext(restOptions)
	defer cancel()

	client := api.NewLoxiClient(restOptions)
	resp, err := client.LogArchives().SubResources([]string{name}).Get(ctx)
	if err != nil {
		return render(out, o, command, report, nil, transportError("log archive request failed", err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, rerr := readBody(resp.Body, "log archive")
		if rerr != nil {
			return render(out, o, command, report, nil, rerr)
		}
		return render(out, o, command, report, nil, api.NewStatusError(resp.StatusCode, body))
	}

	tracked := &readTracker{r: resp.Body}
	if file == "-" {
		// The archive is the whole of stdout: nothing else is printed.
		if _, err := io.Copy(out, tracked); err != nil {
			if tracked.err != nil {
				return transportError("cannot read the log archive response", tracked.err)
			}
			return &api.LifecycleError{
				Reason:  api.ReasonFileWrite,
				Message: fmt.Sprintf("cannot write the archive to stdout: %v", err),
			}
		}
		return nil
	}
	n, err := api.WriteFileAtomic(file, tracked)
	if err != nil {
		if tracked.err != nil {
			err = transportError("cannot read the log archive response", tracked.err)
		}
		return render(out, o, command, report, nil, err)
	}
	report.Archive = &api.LogArchiveFileResult{Name: name, Path: file, Bytes: n}
	return render(out, o, command, report, humanLogArchive, nil)
}

func humanLogArchive(out io.Writer, report *api.LifecycleReport) error {
	a := report.Archive
	if a == nil {
		return nil
	}
	fmt.Fprintf(out, "Archive %s written to %s (%d bytes).\n", a.Name, a.Path, a.Bytes)
	return nil
}
