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
package cmd

import (
	"fmt"
	"os"

	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/create"
	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/delete"
	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/dump"
	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/get"
	"github.com/loxilb-io/loxicmd-inference-gateway/cmd/set"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/envelope"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

var Version = "v0.9.8.6"
var BuildInfo string

var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Get a version",
	Long:  `It shows Loxicmd version.`,

	Run: func(cmd *cobra.Command, args []string) {
		// The root's persistent -o/--output flag is visible here through
		// flag inheritance; version has no REST options of its own. The
		// human output below is a released surface and stays byte-identical.
		if out, err := cmd.Flags().GetString("output"); err == nil && out == "json" {
			result := envelope.New("version")
			result.Data = currentBuildIdentity()
			_ = result.Write(cmd.OutOrStdout())
			return
		}
		fmt.Printf("Loxicmd version: %s\nLoxicmd build info: %s\n", Version, BuildInfo)
	},
}

var CompletionCmd = &cobra.Command{
	Use:                   "completion [bash|zsh|fish|powershell]",
	Short:                 "Generate completion script",
	Long:                  "To load completions",
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.ExactValidArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	var rootCmd = &cobra.Command{
		Use:   "loxicmd",
		Short: "loxicmd is the command-line tool for loxilb.",
		Long: `loxicmd is the command-line tool for loxilb. It is equivalent of "kubectl" for loxilb. loxicmd provides the following (currently) :
	- Create/Delete/Get - Service type external load-balancer, Vlan, Vxlan, Qos Policies, Endpoint client,FDB, IPaddress, Neighbor, Route,Firewall, Mirror, Session, UlCl
	- Get Port(interface) dump used by loxilb or its docker
	- Get Connection track (TCP/UDP/ICMP/SCTP) information
loxicmd aim to provide all of the configuation for the loxilb.`,
	}
	restOptions := &api.RESTOptions{}
	saveOptions := &dump.SaveOptions{}
	applyOptions := &dump.ApplyOptions{}

	rootCmd.PersistentFlags().Int16VarP(&restOptions.Timeout, "timeout", "t", 10, "Set timeout")
	rootCmd.PersistentFlags().StringVarP(&restOptions.Protocol, "protocol", "", "http", "Set API server http/https")
	rootCmd.PersistentFlags().StringVarP(&restOptions.PrintOption, "output", "o", "", "Set output layer (ex.) wide, json)")
	rootCmd.PersistentFlags().StringVarP(&restOptions.ServerIP, "apiserver", "s", "127.0.0.1", "Set API server IP address")
	rootCmd.PersistentFlags().IntVarP(&restOptions.ServerPort, "port", "p", 11111, "Set API server port number")
	rootCmd.PersistentFlags().StringVarP(&restOptions.Token, "token", "", "", "Set Token for the API server")
	rootCmd.PersistentFlags().BoolVarP(&restOptions.BearerAuth, "bearer", "", true, "Send the token as 'Authorization: Bearer <token>' (required by the inference gateway; disable for classic loxilb raw-token targets)")
	rootCmd.PersistentFlags().BoolVarP(&restOptions.Insecure, "insecure", "k", false, "Skip TLS certificate verification (https only)")
	rootCmd.PersistentFlags().StringVarP(&restOptions.CACertFile, "cacert", "", "", "CA certificate (PEM) to verify the server (https only)")
	rootCmd.PersistentFlags().StringVarP(&restOptions.ClientCertFile, "cert", "", "", "Client certificate (PEM) for mutual TLS (https only)")
	rootCmd.PersistentFlags().StringVarP(&restOptions.ClientKeyFile, "key", "", "", "Client private key (PEM) for mutual TLS (https only)")

	rootCmd.AddCommand(get.GetCmd(restOptions))
	rootCmd.AddCommand(create.CreateCmd(restOptions))
	rootCmd.AddCommand(delete.DeleteCmd(restOptions))
	rootCmd.AddCommand(set.SetParamCmd(restOptions))

	saveCmd := dump.SaveCmd(saveOptions, restOptions)
	applyCmd := dump.ApplyCmd(applyOptions, restOptions)

	saveCmd.Flags().BoolVarP(&saveOptions.SaveAllConfig, "all", "a", false, "Saves all loxilb configuration")
	saveCmd.Flags().BoolVarP(&saveOptions.SaveIpConfig, "ip", "i", false, "Saves IP configuration")
	saveCmd.Flags().BoolVarP(&saveOptions.SaveLBConfig, "lb", "l", false, "Saves Load Balancer rules configuration")
	saveCmd.Flags().BoolVarP(&saveOptions.SaveSessionConfig, "session", "", false, "Saves session configuration")
	saveCmd.Flags().BoolVarP(&saveOptions.SaveUlClConfig, "ulcl", "", false, "Saves ulcl configuration")
	saveCmd.Flags().BoolVarP(&saveOptions.SaveFWConfig, "firewall", "", false, "Saves firewall configuration")
	saveCmd.Flags().BoolVarP(&saveOptions.SaveEPConfig, "endpoint", "", false, "Saves endpoint configuration")
	saveCmd.Flags().BoolVarP(&saveOptions.SaveBFDConfig, "bfd", "", false, "Saves BFD configuration")
	saveCmd.Flags().BoolVarP(&saveOptions.SaveViaApi, "api", "", false, "Compatibility alias for 'loxicmd create persist': ask the gateway to persist its own running configuration to snapshot.json (POST /config/persist). May be combined with --ip to also dump interface configuration locally, which the snapshot excludes")
	saveCmd.Flags().StringVarP(&saveOptions.ConfigPath, "config-path", "c", "", "Client-local directory for the text dumps this command writes; it does not change where the gateway writes snapshot.json (that is the gateway's own --config-path)")

	saveCmd.MarkFlagsMutuallyExclusive("all", "ip", "lb", "session", "ulcl", "firewall", "endpoint", "bfd")

	applyCmd.Flags().StringVarP(&applyOptions.IpConfigFile, "ip", "i", "", "IP config file to apply")
	applyCmd.Flags().StringVarP(&applyOptions.Intf, "per-intf", "", "", "Apply configuration only for specific interface")
	applyCmd.Flags().BoolVarP(&applyOptions.Route, "ipv4route", "r", false, "Apply route configuration only for specific interface")

	applyCmd.Flags().StringVarP(&applyOptions.ConfigPath, "config-path", "c", "/etc/loxilb/ipconfig/", "Configuration path only for applying per interface config")
	applyCmd.Flags().StringVarP(&applyOptions.LBConfigFile, "lb", "l", "", "Load Balancer config file to apply")
	applyCmd.Flags().StringVarP(&applyOptions.SessionConfigFile, "session", "", "", "Session config file to apply")
	applyCmd.Flags().StringVarP(&applyOptions.SessionUlClConfigFile, "ulcl", "", "", "Ulcl config file to apply")
	applyCmd.Flags().StringVarP(&applyOptions.FWConfigFile, "firewall", "", "", "Firewall config file to apply")
	applyCmd.Flags().StringVarP(&applyOptions.NormalConfigFile, "file", "f", "", "Config file to apply as like K8s")
	applyCmd.Flags().StringVarP(&applyOptions.BFDConfigFile, "bfd", "", "", "BFD Config file to apply")

	rootCmd.AddCommand(saveCmd)
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(CompletionCmd)
	rootCmd.AddCommand(VersionCmd)

	// The single exit point (contracts/exit-codes.md): every failure is
	// classified into the frozen taxonomy and printed exactly once, to
	// stderr. Cobra's own printing is silenced so the classification here
	// is the only reporter — no command calls os.Exit for a failure, and
	// the legacy 1 is never emitted by this binary.
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	err := rootCmd.Execute()
	if err != nil {
		ce := exitcode.Classify(err)
		if ce.ShowHelp {
			// Help shown because the invocation was invalid goes to
			// stderr; only an explicit --help earns stdout and exit 0.
			cmd, _, findErr := rootCmd.Find(os.Args[1:])
			if findErr != nil || cmd == nil {
				cmd = rootCmd
			}
			cmd.SetOut(os.Stderr)
			_ = cmd.Help()
		}
		fmt.Fprintf(os.Stderr, "Error: %s\n", ce.Message)
		os.Exit(int(ce.Code))
	}
}
