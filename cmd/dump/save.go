/*
 * Copyright (c) 2022 NetLOX Inc
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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	get "github.com/loxilb-io/loxicmd-inference-gateway/cmd/get"
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

// saveViaApi asks the GATEWAY to persist its own running config to
// {config-path}/snapshot.json (POST /config/persist) instead of dumping
// legacy *.txt files client-side. The gateway is the single writer of
// canonical persisted config: the snapshot it writes is what the next boot
// restores, covers every config domain (not just the *.txt subset), and
// cannot be shadowed by a stale snapshot.json the way a client-side *.txt
// save can. Interface config (--ip) stays a client-side dump either way --
// it is host-level state outside the snapshot document.
func saveViaApi(restOptions *api.RESTOptions) error {
	client := api.NewLoxiClient(restOptions)
	ctx := context.TODO()
	var cancel context.CancelFunc
	if restOptions.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
		defer cancel()
	}

	resp, err := client.Persist().Create(ctx, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return api.NewAPIError(resp.StatusCode, body)
	}
	res := api.PersistResult{}
	if jerr := json.Unmarshal(body, &res); jerr == nil && res.Path != "" {
		fmt.Printf("Configuration persisted by the gateway to %s (checksum %s).\n", res.Path, res.Checksum)
	} else {
		fmt.Println("Configuration persisted by the gateway.")
	}
	return nil
}

// saveCmd represents the save command
func SaveCmd(saveOpts *SaveOptions, restOptions *api.RESTOptions) *cobra.Command {
	saveCmd := &cobra.Command{
		Use:   "save",
		Short: "saves current configuration",
		Long: `saves current configuration in text file

With --api, the gateway itself persists its full running configuration to
{config-path}/snapshot.json (POST /config/persist) -- the canonical form the
gateway restores at boot, covering every configuration domain. The legacy
per-domain text dumps remain available through the existing flags for older
gateways. Interface configuration (--ip) is host-level state outside the
snapshot document and is always saved as a local dump.`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd
			_ = args
			dpath := "/etc/loxilb/"
			if saveOpts.ConfigPath != "" {
				dpath = saveOpts.ConfigPath
			}
			if !saveOpts.SaveViaApi &&
				!saveOpts.SaveIpConfig && !saveOpts.SaveAllConfig &&
				!saveOpts.SaveLBConfig && !saveOpts.SaveSessionConfig &&
				!saveOpts.SaveUlClConfig && !saveOpts.SaveFWConfig &&
				!saveOpts.SaveEPConfig && !saveOpts.SaveBFDConfig {
				fmt.Println("Provide valid options")
				cmd.Help()
				return
			}
			if saveOpts.SaveViaApi {
				// The gateway persists every config domain in one shot;
				// the per-domain legacy *.txt dumps below are its
				// pre-snapshot ancestors. Interface config is the one
				// domain the API does not cover, so --ip is still honored
				// as a local dump alongside the API persist.
				if saveOpts.SaveIpConfig || saveOpts.SaveAllConfig {
					if _, err := os.Stat(dpath); errors.Is(err, os.ErrNotExist) {
						if err := os.Mkdir(dpath, os.ModePerm); err != nil {
							fmt.Printf("Can't create config dir %v\n", dpath)
							return
						}
					}
					file, err := get.Nlpdump(dpath)
					if err != nil {
						fmt.Println(err.Error())
						return
					}
					fmt.Println("IP Configuration saved in", file)
				}
				if err := saveViaApi(restOptions); err != nil {
					fmt.Printf("Error: %s\n", err.Error())
				}
				return
			}
			if _, err := os.Stat(dpath); errors.Is(err, os.ErrNotExist) {
				err := os.Mkdir(dpath, os.ModePerm)
				if err != nil {
					fmt.Printf("Can't create config dir %v\n", dpath)
					return
				}
			}
			if saveOpts.SaveIpConfig || saveOpts.SaveAllConfig {
				file, err := get.Nlpdump(dpath)
				if err != nil {
					fmt.Println(err.Error())
					return
				}
				fmt.Println("IP Configuration saved in", file)
			}
			if saveOpts.SaveLBConfig || saveOpts.SaveAllConfig {
				lbfile, err := get.Lbdump(restOptions, dpath)
				if err != nil {
					fmt.Println(err.Error())
					return
				}
				fmt.Println("LB Configuration saved in", lbfile)
			}
			if saveOpts.SaveSessionConfig || saveOpts.SaveAllConfig {
				sessionFile, err := get.Sessiondump(restOptions, dpath)
				if err != nil {
					fmt.Println(err.Error())
					return
				}
				fmt.Println("Session Configuration saved in", sessionFile)
			}
			if saveOpts.SaveUlClConfig || saveOpts.SaveAllConfig {
				ulclFile, err := get.SessionUlCldump(restOptions, dpath)
				if err != nil {
					fmt.Println(err.Error())
					return
				}
				fmt.Println("UlCl Configuration saved in", ulclFile)
			}
			if saveOpts.SaveFWConfig || saveOpts.SaveAllConfig {
				FWFile, err := get.FWdump(restOptions, dpath)
				if err != nil {
					fmt.Println(err.Error())
					return
				}
				fmt.Println("Firewall Configuration saved in", FWFile)
			}
			if saveOpts.SaveEPConfig || saveOpts.SaveAllConfig {
				EPFile, err := get.EPdump(restOptions, dpath)
				if err != nil {
					fmt.Println(err.Error())
					return
				}
				fmt.Println("EndPoint Configuration saved in", EPFile)
			}
			if saveOpts.SaveBFDConfig || saveOpts.SaveAllConfig {
				fmt.Println("Saving BFD Configuration...")
				BFDFile, err := get.BFDdump(restOptions, dpath)
				if err != nil {
					fmt.Println(err.Error())
					return
				}
				fmt.Println("BFD Configuration saved in", BFDFile)
			}
		},
	}
	return saveCmd
}
