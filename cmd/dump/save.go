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
package dump

import (
	"errors"
	"fmt"
	"io"
	"os"

	get "github.com/loxilb-io/loxicmd-inference-gateway/cmd/get"
	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/lifecycle"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"

	"github.com/spf13/cobra"
)

type SaveOptions struct {
	SaveIpConfig      bool
	SaveLBConfig      bool
	SaveSessionConfig bool
	SaveUlClConfig    bool
	SaveFWConfig      bool
	SaveEPConfig      bool
	SaveBFDConfig     bool
	SaveAllConfig     bool
	SaveViaApi        bool
	ConfigPath        string
}

// legacyDomainFlags are the client-side text dumps, by flag name. They predate
// the gateway's own snapshot document and cover a subset of it.
var legacyDomainFlags = []struct {
	name     string
	selected func(*SaveOptions) bool
}{
	{"all", func(o *SaveOptions) bool { return o.SaveAllConfig }},
	{"lb", func(o *SaveOptions) bool { return o.SaveLBConfig }},
	{"session", func(o *SaveOptions) bool { return o.SaveSessionConfig }},
	{"ulcl", func(o *SaveOptions) bool { return o.SaveUlClConfig }},
	{"firewall", func(o *SaveOptions) bool { return o.SaveFWConfig }},
	{"endpoint", func(o *SaveOptions) bool { return o.SaveEPConfig }},
	{"bfd", func(o *SaveOptions) bool { return o.SaveBFDConfig }},
}

// validateSaveOptions rejects the flag combinations that used to be accepted
// and then quietly did something other than what they said.
//
// --api asks the GATEWAY to persist its own configuration; the legacy flags
// select client-side text dumps. Combining them used to run the API persist
// and silently skip every legacy dump but --ip, so 'save --api --all' reported
// success having written none of the files its flag named. The one honest
// combination is --api with --ip: interface configuration is host-level state
// that the snapshot document deliberately excludes, so it still needs a local
// dump.
//
// --config-path is likewise client-local. It names where the text dumps go and
// has no bearing on where the gateway writes snapshot.json - that path is the
// gateway's own --config-path, set on the daemon.
func validateSaveOptions(o *SaveOptions) error {
	if !o.SaveViaApi {
		if !o.SaveIpConfig && !o.SaveAllConfig && !o.SaveLBConfig && !o.SaveSessionConfig &&
			!o.SaveUlClConfig && !o.SaveFWConfig && !o.SaveEPConfig && !o.SaveBFDConfig {
			return &api.LifecycleError{
				Reason:  api.ReasonInvalidArguments,
				Message: "select what to save: --api for the gateway's own snapshot, or one of the local dump flags",
			}
		}
		return nil
	}
	for _, flag := range legacyDomainFlags {
		if flag.selected(o) {
			return &api.LifecycleError{
				Reason: api.ReasonInvalidArguments,
				Message: fmt.Sprintf("--api cannot be combined with --%s: --api asks the gateway to persist its own "+
					"configuration, while --%s writes a local text dump. Run them separately, or use --api with --ip, "+
					"which dumps the interface configuration the snapshot document excludes", flag.name, flag.name),
			}
		}
	}
	if o.ConfigPath != "" && !o.SaveIpConfig {
		return &api.LifecycleError{
			Reason: api.ReasonInvalidArguments,
			Message: "--config-path names a client-local directory for text dumps and does not change where the gateway " +
				"writes snapshot.json; with --api alone there is no local dump to place. The gateway's own --config-path " +
				"decides the snapshot location",
		}
	}
	return nil
}

// SaveCmd is the legacy client-side configuration dump, plus --api, the
// compatibility alias for the canonical 'loxicmd create persist'.
func SaveCmd(saveOpts *SaveOptions, restOptions *api.RESTOptions) *cobra.Command {
	saveCmd := &cobra.Command{
		Use:   "save",
		Short: "saves current configuration",
		Long: `saves current configuration in text file

With --api, the gateway persists its own running configuration to
{config-path}/snapshot.json (POST /config/persist) -- the canonical form the
gateway replays at boot. This is a compatibility alias: the canonical command
is 'loxicmd create persist', which takes the same call and reports the same
result. The persisted document declares which configuration domains it
captured and which it excludes; the command prints both rather than claiming
blanket coverage.

The remaining flags write the older client-side text dumps, which cover a
subset of the snapshot's domains and are not what the gateway replays at boot.
Interface configuration (--ip) is host-level state that the snapshot document
excludes, so --api may be combined with --ip to dump it locally as well.

--config-path names the client-local directory for those text dumps. It does
not affect where the gateway writes snapshot.json; that is the gateway's own
--config-path.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return runSave(cmd, saveOpts, restOptions)
		},
	}
	return saveCmd
}

func runSave(cmd *cobra.Command, saveOpts *SaveOptions, restOptions *api.RESTOptions) error {
	out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
	if err := validateSaveOptions(saveOpts); err != nil {
		if restOptions.PrintOption == "json" {
			_ = api.WriteLifecycleReport(out, api.NewFailureReport("save", err))
		} else {
			fmt.Fprintf(errOut, "Error: %s\n", err.Error())
		}
		return err
	}

	dpath := "/etc/loxilb/"
	if saveOpts.ConfigPath != "" {
		dpath = saveOpts.ConfigPath
	}

	if saveOpts.SaveViaApi {
		if saveOpts.SaveIpConfig {
			if err := saveIPConfig(out, dpath); err != nil {
				fmt.Fprintf(errOut, "Error: %s\n", err.Error())
				return err
			}
		}
		// One implementation, so the alias cannot drift from the
		// canonical command in what it reports or how it exits.
		return lifecycle.Persist(restOptions, out, errOut,
			lifecycle.OptionsFrom(restOptions, false), "save --api")
	}

	if err := ensureConfigDir(dpath); err != nil {
		fmt.Fprintf(errOut, "Error: %s\n", err.Error())
		return err
	}

	dumps := []struct {
		selected bool
		label    string
		dump     func() (string, error)
	}{
		{saveOpts.SaveIpConfig || saveOpts.SaveAllConfig, "IP", func() (string, error) { return get.Nlpdump(dpath) }},
		{saveOpts.SaveLBConfig || saveOpts.SaveAllConfig, "LB", func() (string, error) { return get.Lbdump(restOptions, dpath) }},
		{saveOpts.SaveSessionConfig || saveOpts.SaveAllConfig, "Session", func() (string, error) { return get.Sessiondump(restOptions, dpath) }},
		{saveOpts.SaveUlClConfig || saveOpts.SaveAllConfig, "UlCl", func() (string, error) { return get.SessionUlCldump(restOptions, dpath) }},
		{saveOpts.SaveFWConfig || saveOpts.SaveAllConfig, "Firewall", func() (string, error) { return get.FWdump(restOptions, dpath) }},
		{saveOpts.SaveEPConfig || saveOpts.SaveAllConfig, "EndPoint", func() (string, error) { return get.EPdump(restOptions, dpath) }},
		{saveOpts.SaveBFDConfig || saveOpts.SaveAllConfig, "BFD", func() (string, error) { return get.BFDdump(restOptions, dpath) }},
	}
	for _, d := range dumps {
		if !d.selected {
			continue
		}
		file, err := d.dump()
		if err != nil {
			// A dump that failed is a save that did not happen:
			// report it and exit non-zero rather than leave the
			// caller believing the earlier lines cover everything.
			wrapped := &api.LifecycleError{
				Reason:  api.ReasonFileWrite,
				Message: fmt.Sprintf("%s configuration was not saved: %v", d.label, err),
			}
			fmt.Fprintf(errOut, "Error: %s\n", wrapped.Error())
			return wrapped
		}
		fmt.Fprintf(out, "%s Configuration saved in %s\n", d.label, file)
	}
	return nil
}

// saveIPConfig dumps host interface configuration locally. It is the one
// domain the snapshot document does not cover, so it stays a client-side dump
// even alongside --api.
func saveIPConfig(out io.Writer, dpath string) error {
	if err := ensureConfigDir(dpath); err != nil {
		return err
	}
	file, err := get.Nlpdump(dpath)
	if err != nil {
		return &api.LifecycleError{
			Reason:  api.ReasonFileWrite,
			Message: fmt.Sprintf("IP configuration was not saved: %v", err),
		}
	}
	fmt.Fprintln(out, "IP Configuration saved in", file)
	return nil
}

func ensureConfigDir(dpath string) error {
	if _, err := os.Stat(dpath); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(dpath, os.ModePerm); err != nil {
			return &api.LifecycleError{
				Reason:  api.ReasonFileWrite,
				Message: fmt.Sprintf("cannot create the config directory %s: %v", dpath, err),
			}
		}
	}
	return nil
}
