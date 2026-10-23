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
package get

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/featuregate"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func NewGetLoadBalancerCmd(restOptions *api.RESTOptions) *cobra.Command {
	var bands bool
	var GetLbCmd = &cobra.Command{
		Use:     "loadbalancer",
		Short:   "Get a LoadBalancer",
		Aliases: []string{"lb", "loadbalancers", "lbs"},
		Long: `It shows Load balancer Information

-o wide adds the capacity gate in force, including the priority bands'
wake policy as configured and as run. --bands prints, for each rule (one
with --servName), the per-band table the gateway reports in fc_effective:
inflight, queued, wait window, queue share, floor and weight in force, with
where each knob came from. -o json carries the whole fc_effective.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			if bands {
				if err := featuregate.Require(restOptions, featuregate.Bands, cmd.ErrOrStderr()); err != nil {
					return err
				}
			}
			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}
			resp, err := client.LoadBalancerAll().Get(ctx)
			if err != nil {
				return exitcode.Unavailablef("get lb: %v", err)
			}
			if resp.StatusCode == http.StatusOK {
				if bands && restOptions.PrintOption != "json" {
					return PrintGetLbBands(resp, *restOptions)
				}
				PrintGetLbResult(resp, *restOptions)
				return nil
			}
			return exitcode.FromHTTPStatus("get lb", resp.StatusCode)
		},
	}
	GetLbCmd.Flags().StringVarP(&restOptions.ServiceName, "servName", "", restOptions.ServiceName, "Name for load balancer rule")
	GetLbCmd.Flags().BoolVar(&bands, "bands", false, "Print the per-band table (band, inflight, queued, wait_ms, queue_pct, min_share_pct, floor_units, weight, weight_pct, source) of each rule, one with --servName")
	return GetLbCmd
}

func NumToSelect(sel int) string {
	var ret string
	switch sel {
	case 0:
		ret = "rr"
	case 1:
		ret = "hash"
	case 2:
		ret = "priority"
	case 3:
		ret = "persist"
	case 4:
		ret = "lc"
	case 5:
		ret = "n2"
	case 6:
		ret = "n3"
	case 8:
		ret = "chwbl"
	case 9:
		ret = "gpuaware"
	case 10:
		ret = "wrr-hash"
	default:
		ret = "rr"
	}
	return ret
}

func NumToSecurty(sec int) string {
	var ret string
	switch sec {
	case 0:
		ret = ""
	case 1:
		ret = "https"
	case 2:
		ret = "e2ehttps"
	default:
		ret = ""
	}
	return ret
}

func NumToMode(mode int, ppv2 bool, egress bool) string {
	var ret string
	switch mode {
	case 1:
		ret = "onearm"
	case 2:
		ret = "fullnat"
	case 3:
		ret = "dsr"
	case 4:
		ret = "fullproxy"
	case 5:
		ret = "hostonearm"
	default:
		ret = "default"
	}
	if ppv2 {
		ret += ":ppv2"
	}
	if egress {
		ret += ":egress"
	}
	return ret
}

func BoolToMon(mon bool) string {
	var ret string
	switch mon {
	case false:
		ret = "Off"
	case true:
		ret = "On"
	}
	return ret
}

func PrintGetLbResult(resp *http.Response, o api.RESTOptions) {
	lbresp := api.LbRuleModGet{}
	var data [][]string
	resultByte, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error: Failed to read HTTP response: (%s)\n", err.Error())
		return
	}

	if err := json.Unmarshal(resultByte, &lbresp); err != nil {
		fmt.Printf("Error: Failed to unmarshal HTTP response: (%s)\n", err.Error())
		return
	}

	// if json options enable, it print as a json format.
	if o.PrintOption == "json" {
		resultIndent, _ := json.MarshalIndent(lbresp, "", "    ")
		fmt.Println(string(resultIndent))
		return
	}

	lbresp.Sort()

	// Table Init
	table := TableInit()

	// Making load balance data
	for _, lbrule := range lbresp.LbRules {
		if o.ServiceName != "" && o.ServiceName != lbrule.Service.Name || lbrule.Service.Snat {
			continue
		}
		protocolStr := lbrule.Service.Protocol
		if lbrule.Service.Security != 0 {
			protocolStr += fmt.Sprintf(":%s", NumToSecurty(int(lbrule.Service.Security)))
		}
		var effQueueDepth, effQueueWaitMs uint32
		if eff := lbrule.Service.FcEffective; eff != nil {
			effQueueDepth, effQueueWaitMs = eff.QueueDepth, eff.QueueWaitMs
		}
		gateCell, maxOutCell := fcGateCells(lbrule.Service.FcEffective)
		adaptCell := fcAdaptCell(lbrule.Service.FcEffective)
		tenantCell := fcTenantCell(lbrule.Service.FcEffective)
		headersCell := fcHeadersCell(lbrule.Service.FcEffective)
		backendTLSCell := backendTLSCell(lbrule.Service.BackendTLSEffective)
		bandsCell := fcBandsCell(lbrule.Service.FcEffective)
		if o.PrintOption == "wide" {
			table.SetHeader(LOADBALANCER_WIDE_TITLE)
			secIPs := ""
			if len(lbrule.SecondaryIPs) > 0 {
				secIPs = lbrule.SecondaryIPs[0].SecondaryIP
				for i := 1; i < len(lbrule.SecondaryIPs); i++ {
					secIPs = secIPs + ", " + lbrule.SecondaryIPs[i].SecondaryIP
				}
			}

			sources := ""
			if len(lbrule.SrcIPs) > 0 {
				sources = lbrule.SrcIPs[0].Prefix
				for i := 1; i < len(lbrule.SecondaryIPs); i++ {
					sources = sources + ", " + lbrule.SrcIPs[i].Prefix
				}
			}

			if lbrule.Service.Monitor {
				for i, eps := range lbrule.Endpoints {
					if i == 0 {
						if lbrule.Service.PortMax == 0 {
							data = append(data, []string{lbrule.Service.ExternalIP, secIPs, sources, lbrule.Service.Host, fmt.Sprintf("%d", lbrule.Service.Port), protocolStr, lbrule.Service.Name, fmt.Sprintf("%d", lbrule.Service.Block), NumToSelect(int(lbrule.Service.Sel)), NumToMode(int(lbrule.Service.Mode), lbrule.Service.PpV2, lbrule.Service.Egress),
								eps.EndpointIP, fmt.Sprintf("%d", eps.TargetPort), fmt.Sprintf("%d", eps.Weight), eps.State, eps.Counter, gateCell, maxOutCell, adaptCell, tenantCell, headersCell, backendTLSCell, bandsCell})
						} else {
							data = append(data, []string{lbrule.Service.ExternalIP, secIPs, sources, lbrule.Service.Host, fmt.Sprintf("%d-%d", lbrule.Service.Port, lbrule.Service.PortMax), protocolStr, lbrule.Service.Name, fmt.Sprintf("%d", lbrule.Service.Block), NumToSelect(int(lbrule.Service.Sel)), NumToMode(int(lbrule.Service.Mode), lbrule.Service.PpV2, lbrule.Service.Egress),
								eps.EndpointIP, fmt.Sprintf("%d", eps.TargetPort), fmt.Sprintf("%d", eps.Weight), eps.State, eps.Counter, gateCell, maxOutCell, adaptCell, tenantCell, headersCell, backendTLSCell, bandsCell})
						}
					} else {
						data = append(data, []string{"", "", "", "", "", "", "", "", "", "", eps.EndpointIP, fmt.Sprintf("%d", eps.TargetPort), fmt.Sprintf("%d", eps.Weight), eps.State, eps.Counter, "", "", "", "", "", "", ""})
					}
				}
			} else {
				for i, eps := range lbrule.Endpoints {
					if i == 0 {
						if lbrule.Service.PortMax == 0 {
							data = append(data, []string{lbrule.Service.ExternalIP, secIPs, sources, lbrule.Service.Host, fmt.Sprintf("%d", lbrule.Service.Port), protocolStr, lbrule.Service.Name, fmt.Sprintf("%d", lbrule.Service.Block), NumToSelect(int(lbrule.Service.Sel)), NumToMode(int(lbrule.Service.Mode), lbrule.Service.PpV2, lbrule.Service.Egress),
								eps.EndpointIP, fmt.Sprintf("%d", eps.TargetPort), fmt.Sprintf("%d", eps.Weight), "-", eps.Counter, gateCell, maxOutCell, adaptCell, tenantCell, headersCell, backendTLSCell, bandsCell})
						} else {
							data = append(data, []string{lbrule.Service.ExternalIP, secIPs, sources, lbrule.Service.Host, fmt.Sprintf("%d-%d", lbrule.Service.Port, lbrule.Service.PortMax), protocolStr, lbrule.Service.Name, fmt.Sprintf("%d", lbrule.Service.Block), NumToSelect(int(lbrule.Service.Sel)), NumToMode(int(lbrule.Service.Mode), lbrule.Service.PpV2, lbrule.Service.Egress),
								eps.EndpointIP, fmt.Sprintf("%d", eps.TargetPort), fmt.Sprintf("%d", eps.Weight), "-", eps.Counter, gateCell, maxOutCell, adaptCell, tenantCell, headersCell, backendTLSCell, bandsCell})
						}
					} else {
						data = append(data, []string{"", "", "", "", "", "", "", "", "", "", eps.EndpointIP, fmt.Sprintf("%d", eps.TargetPort), fmt.Sprintf("%d", eps.Weight), "-", eps.Counter, "", "", "", "", "", "", ""})
					}
				}
			}
		} else {
			table.SetHeader(LOADBALANCER_TITLE)
			if lbrule.Service.PortMax == 0 {
				data = append(data, []string{lbrule.Service.ExternalIP, fmt.Sprintf("%d", lbrule.Service.Port), protocolStr, lbrule.Service.Name, fmt.Sprintf("%d", lbrule.Service.Block), NumToSelect(int(lbrule.Service.Sel)), NumToMode(int(lbrule.Service.Mode), lbrule.Service.PpV2, lbrule.Service.Egress), fmt.Sprintf("%d", len(lbrule.Endpoints)), fmt.Sprintf("%v", lbrule.Service.Timeout), BoolToMon(lbrule.Service.Monitor), fmt.Sprintf("%d", lbrule.Service.ConnectionLimit), fcQueueCell(u32Val(lbrule.Service.FcMaxQueueDepth), effQueueDepth), fcQueueCell(u32Val(lbrule.Service.FcMaxQueueWaitMs), effQueueWaitMs)})
			} else {
				data = append(data, []string{lbrule.Service.ExternalIP, fmt.Sprintf("%d-%d", lbrule.Service.Port, lbrule.Service.PortMax), protocolStr, lbrule.Service.Name, fmt.Sprintf("%d", lbrule.Service.Block), NumToSelect(int(lbrule.Service.Sel)), NumToMode(int(lbrule.Service.Mode), lbrule.Service.PpV2, lbrule.Service.Egress), fmt.Sprintf("%d", len(lbrule.Endpoints)), fmt.Sprintf("%v", lbrule.Service.Timeout), BoolToMon(lbrule.Service.Monitor), fmt.Sprintf("%d", lbrule.Service.ConnectionLimit), fcQueueCell(u32Val(lbrule.Service.FcMaxQueueDepth), effQueueDepth), fcQueueCell(u32Val(lbrule.Service.FcMaxQueueWaitMs), effQueueWaitMs)})
			}
		}
	}

	// Rendering the load balance data to table
	TableShow(data, table)
}

func LoadbalancerAPICall(restOptions *api.RESTOptions) (*http.Response, error) {
	client := api.NewLoxiClient(restOptions)
	ctx := context.TODO()
	var cancel context.CancelFunc
	if restOptions.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
		defer cancel()
	}
	resp, err := client.LoadBalancerAll().Get(ctx)
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		return nil, err
	}
	return resp, nil
}

func Lbdump(restOptions *api.RESTOptions, path string) (string, error) {
	lbresp := api.LbRuleModGet{}
	dresp := api.LbRuleModGet{}
	// File Open
	fileP := []string{"/tmp/lbconfig_", ".txt"}
	t := time.Now()
	file := strings.Join(fileP, t.Local().Format("2006-01-02_15:04:05"))
	f, err := os.Create(file)
	if err != nil {
		return "", fmt.Errorf("can't create dump file: %w", err)
	}
	defer os.Remove(f.Name())

	// API Call
	client := api.NewLoxiClient(restOptions)
	ctx := context.TODO()
	var cancel context.CancelFunc
	if restOptions.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
		defer cancel()
	}
	resp, err := client.LoadBalancerAll().Get(ctx)
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		return "", err
	}

	resultByte, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error: Failed to read HTTP response: (%s)\n", err.Error())
	}

	if err := json.Unmarshal(resultByte, &lbresp); err != nil {
		fmt.Printf("Error: Failed to unmarshal HTTP response: (%s)\n", err.Error())
		return "", err
	}

	for _, lbrule := range lbresp.LbRules {
		if !lbrule.Service.Managed && !lbrule.Service.Snat && !strings.Contains(lbrule.Service.Name, "ipvs") {
			for i := range lbrule.Endpoints {
				lbacts := &lbrule.Endpoints[i]
				lbacts.Counter = ""
			}
			dresp.LbRules = append(dresp.LbRules, lbrule)
		}
	}

	dumpBytes, err := json.Marshal(dresp)
	if err != nil {
		fmt.Printf("Error: Failed to marshal dump LB rules (%s)\n", err.Error())
		return "", err
	}

	// Write
	_, err = f.Write(dumpBytes)
	if err != nil {
		fmt.Println("File write error")
	}
	cfile := path + "lbconfig.txt"
	if _, err := os.Stat(cfile); errors.Is(err, os.ErrNotExist) {
		if err != nil {
			fmt.Println("There is no saved config file")
		}
	} else {
		command := "mv " + cfile + " " + cfile + ".bk"
		cmd := exec.Command("bash", "-c", command)
		_, err := cmd.Output()
		if err != nil {
			fmt.Println("Can't backup ", cfile)
			return file, err
		}
	}
	command := "cp -R " + file + " " + cfile
	cmd := exec.Command("bash", "-c", command)
	fmt.Println(cmd)
	_, err = cmd.Output()
	if err != nil {
		fmt.Println("Failed copy file to", cfile)
		return file, err
	}
	return file, nil
}

// fcQueueCell renders a declared capacity-queue field. A service that
// declares none (0) runs on the process default, which GET reports in
// fc_effective: it is shown beside the 0, so the cell never reads as "no
// queue" while one is in force.
func fcQueueCell(declared, effective uint32) string {
	if declared == 0 && effective != 0 {
		return fmt.Sprintf("0 (default %d)", effective)
	}
	return fmt.Sprintf("%d", declared)
}

// u32Val reads an optional numeric field, 0 when the gateway left it out.
func u32Val(p *uint32) uint32 {
	if p == nil {
		return 0
	}
	return *p
}

// fcGateCells renders the capacity gate in force on a service for the wide
// view: its mode and its pool-wide ceiling, each with where it came from
// (rule, env or default). "-" when the gateway reports no gate state (a
// service that is not an AI gateway, or a gateway that predates it).
func fcGateCells(eff *api.FcEffective) (string, string) {
	if eff == nil || eff.Mode == "" {
		return "-", "-"
	}
	src := func(s string) string {
		if s == "" {
			return ""
		}
		return " (" + s + ")"
	}
	var modeSrc, maxSrc string
	if eff.Source != nil {
		modeSrc, maxSrc = eff.Source.Mode, eff.Source.MaxOutstanding
	}
	return eff.Mode + src(modeSrc), fmt.Sprintf("%d", eff.MaxOutstanding) + src(maxSrc)
}

// fcAdaptCell renders the adaptive ceiling for the wide view: its state, the
// ceiling it holds against the configured one, why it moved, and how many
// endpoints are still ramping up ("tightened 2/8 (ttft)", "open 8/8, 2
// warming"). "off" when the rule does not adapt; "-" when the gateway reports
// no adaptive state (no gate, or a gateway that predates it).
func fcAdaptCell(eff *api.FcEffective) string {
	if eff == nil || eff.AdaptState == "" {
		return "-"
	}
	if eff.AdaptState == "off" {
		return "off"
	}
	cell := fmt.Sprintf("%s %d/%d", eff.AdaptState, eff.EffectiveMaxOutstanding, eff.MaxOutstanding)
	if eff.AdaptReason != "" && eff.AdaptReason != "none" {
		cell += " (" + eff.AdaptReason + ")"
	}
	if eff.WarmingEndpoints > 0 {
		cell += fmt.Sprintf(", %d warming", eff.WarmingEndpoints)
	}
	return cell
}

// fcTenantCell renders the tenant share for the wide view: the share in force,
// where it came from, and the tenants holding a unit or waiting now ("25%
// (rule), 3 active"). "-" when no share holds (0 or 100) or the gateway
// reports none.
func fcTenantCell(eff *api.FcEffective) string {
	if eff == nil || eff.TenantMaxSharePct == 0 || eff.TenantMaxSharePct >= 100 {
		return "-"
	}
	cell := fmt.Sprintf("%d%%", eff.TenantMaxSharePct)
	if eff.Source != nil && eff.Source.TenantMaxSharePct != "" {
		cell += " (" + eff.Source.TenantMaxSharePct + ")"
	}
	return cell + fmt.Sprintf(", %d active", eff.TenantsActive)
}

// fcHeadersCell renders the admission headers switch for the wide view: "on"
// or "off" in force, with where it came from ("on (rule)"). "-" when the
// gateway reports none.
func fcHeadersCell(eff *api.FcEffective) string {
	if eff == nil || eff.ExposeHeaders == "" {
		return "-"
	}
	if eff.Source != nil && eff.Source.ExposeHeaders != "" {
		return eff.ExposeHeaders + " (" + eff.Source.ExposeHeaders + ")"
	}
	return eff.ExposeHeaders
}

// backendTLSCell renders the backend TLS policy a rule's listener has
// installed for the wide view: the status, then what the listener runs
// ("applied: verify, client, name"). A leg that is not verified reads
// "no verify". The certificate IDs and the name are in the json output; the
// cell stays within the width the table wraps at. "-" when the gateway
// reports none, which is every rule whose backend leg is not TLS.
func backendTLSCell(eff *api.BackendTLSEffective) string {
	if eff == nil || eff.Status == "" {
		return "-"
	}
	if eff.Status != "applied" && eff.Status != "failed" {
		return eff.Status
	}
	policy := "no verify"
	if eff.Verify {
		policy = "verify"
	}
	if eff.ClientCert {
		policy += ", client"
	}
	if eff.ServerName != "" {
		policy += ", name"
	}
	return eff.Status + ": " + policy
}

// fcBandsCell renders the priority bands' wake policy for the wide view: as
// configured, and beside it what the gate runs when that differs, with the
// gateway's reason ("weighted (effective: strict, reason: zero_weight)"). A
// fallback that leaves the policy as configured still names its reason. "-"
// when the gateway reports no band state (a gateway that predates it).
func fcBandsCell(eff *api.FcEffective) string {
	if eff == nil || eff.WakePolicy == "" {
		return "-"
	}
	return fcEffectiveCell(eff.WakePolicy, eff.WakePolicyEffective, eff.BandFallbackReason)
}

// fcEffectiveCell renders a configured value beside the effective one when
// they differ, with the reason the gateway gives.
func fcEffectiveCell(configured, effective, reason string) string {
	if configured == "" {
		return "-"
	}
	hasReason := reason != "" && reason != "none"
	switch {
	case effective != "" && effective != configured && hasReason:
		return fmt.Sprintf("%s (effective: %s, reason: %s)", configured, effective, reason)
	case effective != "" && effective != configured:
		return fmt.Sprintf("%s (effective: %s)", configured, effective)
	case hasReason:
		return fmt.Sprintf("%s (reason: %s)", configured, reason)
	}
	return configured
}

// PrintGetLbBands prints, per rule, the priority bands the gateway reports
// in fc_effective: a line with the policies in force and the critical
// reserve, then one row per band. A rule the gateway reports no bands for
// prints "-" in every cell, never 0.
func PrintGetLbBands(resp *http.Response, o api.RESTOptions) error {
	lbresp := api.LbRuleModGet{}
	resultByte, err := io.ReadAll(resp.Body)
	if err != nil {
		return exitcode.Unavailablef("get lb: read response: %v", err)
	}
	if err := json.Unmarshal(resultByte, &lbresp); err != nil {
		return &exitcode.CLIError{Code: exitcode.ContractMismatch, Message: fmt.Sprintf("get lb: decode response: %v", err), Origin: "gateway", HTTPStatus: resp.StatusCode}
	}
	lbresp.Sort()
	for _, lbrule := range lbresp.LbRules {
		if o.ServiceName != "" && o.ServiceName != lbrule.Service.Name || lbrule.Service.Snat {
			continue
		}
		svc := lbrule.Service
		fmt.Println(fcBandsHeading(svc))
		table := TableInit()
		table.SetHeader(LOADBALANCER_BANDS_TITLE)
		TableShow(fcBandRows(svc.FcEffective), table)
	}
	return nil
}

// fcBandsHeading names the rule and the band policies in force on it.
func fcBandsHeading(svc api.LoadBalancerService) string {
	name := svc.Name
	if name == "" {
		name = "-"
	}
	eff := svc.FcEffective
	if eff == nil || eff.WakePolicy == "" {
		return fmt.Sprintf("%s %s:%d/%s  bands: -", name, svc.ExternalIP, svc.Port, svc.Protocol)
	}
	reserve := "-"
	if eff.Bands != nil {
		reserve = fmt.Sprintf("%d%% (%d units)", eff.Bands.CriticalReservePct, eff.Bands.CriticalReserveUnits)
	}
	configSource := eff.ConfigSource
	if configSource == "" {
		configSource = "-"
	}
	return fmt.Sprintf("%s %s:%d/%s  wake policy: %s  shed queue: %s  tenant wake: %s  critical reserve: %s  config source: %s",
		name, svc.ExternalIP, svc.Port, svc.Protocol,
		fcEffectiveCell(eff.WakePolicy, eff.WakePolicyEffective, eff.BandFallbackReason),
		fcEffectiveCell(eff.ShedQueue, eff.ShedQueueEffective, ""),
		fcEffectiveCell(eff.TenantWake, eff.TenantWakeEffective, ""),
		reserve, configSource)
}

// fcBandRows renders the three bands. The source column is where the
// weight, floor, wait window and queue share came from, in that order.
func fcBandRows(eff *api.FcEffective) [][]string {
	names := []string{"critical", "standard", "sheddable"}
	if eff == nil || eff.Bands == nil {
		rows := make([][]string, 0, 3)
		for _, n := range names {
			rows = append(rows, []string{n, "-", "-", "-", "-", "-", "-", "-", "-", "-"})
		}
		return rows
	}
	source := "-"
	if src := eff.Source; src != nil {
		dash := func(s string) string {
			if s == "" {
				return "-"
			}
			return s
		}
		source = fmt.Sprintf("%s/%s/%s/%s", dash(src.BandWeights), dash(src.BandMinSharePct), dash(src.BandWaitMs), dash(src.BandQueuePct))
	}
	rows := make([][]string, 0, 3)
	for i, b := range []api.FcBandEffective{eff.Bands.Critical, eff.Bands.Standard, eff.Bands.Sheddable} {
		rows = append(rows, []string{names[i],
			fmt.Sprintf("%d", b.Inflight), fmt.Sprintf("%d", b.Queued), fmt.Sprintf("%d", b.WaitMs),
			fmt.Sprintf("%d", b.QueuePct), fmt.Sprintf("%d", b.MinSharePct), fmt.Sprintf("%d", b.FloorUnits),
			fmt.Sprintf("%d", b.Weight), fmt.Sprintf("%d", b.WeightPct), source})
	}
	return rows
}
