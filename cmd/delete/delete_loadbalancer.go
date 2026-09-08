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
package delete

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/cli/exitcode"

	"github.com/spf13/cobra"
)

type DeleteLoadBalancerResult struct {
	Result string `json:"result"`
}

func validation(args []string) error {
	if len(args) > 1 {
		fmt.Println("delete lb command too many args")
		fmt.Println(args)
	} else if len(args) <= 0 {
		return errors.New("delete lb needs <EXTERNAL-IP> arg")
	}

	return nil
}

func NewDeleteLoadBalancerCmd(restOptions *api.RESTOptions) *cobra.Command {
	var tcpPortNumberList string
	var udpPortNumberList string
	var sctpPortNumberList string
	var icmpPortNumberList bool
	var BGP bool
	var Mark uint16
	var Name string
	var Host string

	var PathPrefix string
	var PathMatchMode string
	var ModelName string

	var externalIP string
	//var endpointList []string

	var deleteLbCmd = &cobra.Command{
		Use:   "lb <EXTERNAL-IP> [--tcp portNumber] [--udp portNumber] [--sctp portNumber] [--icmp portNumber] [--bgp] [--mark=<val>] [--name=<service-name>] [--host=<url>] [--path-prefix=<prefix>] [--path-match-mode=<disabled|prefix|exact>] [--model-name=<model>]",
		Short: "Delete a LoadBalancer",
		Long: `Delete a LoadBalancer.

L7/AI rules are keyed by host + path-prefix + path-match-mode + model-name, so a
rule created with those attributes must be deleted with the matching flags (or,
most reliably, by --name). Example:
  loxicmd delete lb 20.20.20.1 --tcp=2020 --host=api.example.com \
    --path-prefix=/v1 --path-match-mode=prefix --model-name=llama3

Tip: creating AI rules with --name lets them be removed with
'loxicmd delete lb --name=<name>' without repeating the key attributes.`,
		RunE: func(cmd *cobra.Command, args []string) error {

			client := api.NewLoxiClient(restOptions)
			ctx := context.TODO()
			var cancel context.CancelFunc
			if restOptions.Timeout > 0 {
				ctx, cancel = context.WithTimeout(ctx, time.Duration(restOptions.Timeout)*time.Second)
				defer cancel()
			}

			if Name != "" {
				subResources := []string{
					"name", Name,
				}
				resp, err := client.LoadBalancer().SubResources(subResources).Delete(ctx)
				if err != nil {
					return exitcode.Unavailablef("Failed to delete LoadBalancer(Name: %s) (%v)", Name, err)
				}
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					PrintDeleteResult(resp, *restOptions)
					return nil
				}
				body, _ := io.ReadAll(resp.Body)
				ce := exitcode.FromHTTPStatus("delete lb", resp.StatusCode)
				ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
				return ce
			}

			if err := validation(args); err != nil {
				return exitcode.Invalidf("not valid EXTERNAL-IP")
			}
			externalIP = args[0]
			PortNumberList := make(map[string][]string)

			if len(tcpPortNumberList) > 0 {
				PortNumberList["tcp"] = strings.Split(tcpPortNumberList, "-")
				if len(PortNumberList["tcp"]) > 2 {
					return exitcode.Invalidf("Too many ports in list to delete LoadBalancer(ExternalIP: %s, tcp, Port:%s)", externalIP, tcpPortNumberList)
				}
			}
			if len(udpPortNumberList) > 0 {
				PortNumberList["udp"] = strings.Split(udpPortNumberList, "-")
				if len(PortNumberList["udp"]) > 2 {
					return exitcode.Invalidf("Too many ports in list to delete LoadBalancer(ExternalIP: %s, udp, Port:%s)", externalIP, udpPortNumberList)
				}
			}
			if len(sctpPortNumberList) > 0 {
				PortNumberList["sctp"] = strings.Split(sctpPortNumberList, "-")
				if len(PortNumberList["sctp"]) > 2 {
					return exitcode.Invalidf("Too many ports in list to delete LoadBalancer(ExternalIP: %s, sctp, Port:%s)", externalIP, sctpPortNumberList)
				}
			}
			if icmpPortNumberList {
				PortNumberList["icmp"] = []string{"0", "0"}
			}
			if Host == "" {
				Host = "any"
			}
			for proto, portNum := range PortNumberList {
				sPortMin := portNum[0]
				sPortMax := "0"
				if len(portNum) > 1 {
					sPortMax = portNum[1]
				}
				_, err := strconv.Atoi(sPortMin)
				if err != nil {
					return exitcode.Invalidf("Invalid port in list to delete LoadBalancer(ExternalIP: %s,Port:%s)", externalIP, sPortMin)
				}
				_, err = strconv.Atoi(sPortMax)
				if err != nil {
					return exitcode.Invalidf("Invalid port in list to delete LoadBalancer(ExternalIP: %s,Port:%s)", externalIP, sPortMax)
				}
				subResources := []string{
					"hosturl", Host,
					"externalipaddress", externalIP,
					"port", sPortMin,
					"portmax", sPortMax,
					"protocol", proto,
				}
				qmap := loadBalancerDeleteQuery(BGP, Mark, PathPrefix, PathMatchMode, ModelName)
				resp, err := client.LoadBalancer().SubResources(subResources).Query(qmap).Delete(ctx)
				if err != nil {
					return exitcode.Unavailablef("Failed to delete LoadBalancer(ExternalIP: %s, Protocol:%s, Port:%v) (%v)", externalIP, proto, portNum, err)
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					body, _ := io.ReadAll(resp.Body)
					ce := exitcode.FromHTTPStatus("delete lb", resp.StatusCode)
					ce.Message = api.NewAPIError(resp.StatusCode, body).Error()
					return ce
				}
				PrintDeleteResult(resp, *restOptions)
			}
			return nil
		},
	}

	deleteLbCmd.Flags().StringVar(&tcpPortNumberList, "tcp", tcpPortNumberList, "TCP port list can be specified as '<portMin>-<portMax>'")
	deleteLbCmd.Flags().StringVar(&udpPortNumberList, "udp", udpPortNumberList, "UDP port list can be specified as '<portMin>-<portMax>...'")
	deleteLbCmd.Flags().StringVar(&sctpPortNumberList, "sctp", sctpPortNumberList, "SCTP port list can be specified as '<portMin>-<portMax>...'")
	deleteLbCmd.Flags().BoolVarP(&icmpPortNumberList, "icmp", "", false, "ICMP port list can't be specified'")
	deleteLbCmd.Flags().BoolVarP(&BGP, "bgp", "", false, "BGP enable information'")
	deleteLbCmd.Flags().Uint16VarP(&Mark, "mark", "", 0, "Specify the mark num to segregate a load-balancer VIP service")
	deleteLbCmd.Flags().StringVarP(&Name, "name", "", Name, "Name for load balancer rule")
	deleteLbCmd.Flags().StringVarP(&Host, "host", "", Host, "Ingress Host URL Path")
	deleteLbCmd.Flags().StringVar(&PathPrefix, "path-prefix", PathPrefix, "L7/AI rule key: path prefix to match (ex) /v1")
	deleteLbCmd.Flags().StringVar(&PathMatchMode, "path-match-mode", PathMatchMode, "L7/AI rule key: path match mode disabled|prefix|exact")
	deleteLbCmd.Flags().StringVar(&ModelName, "model-name", ModelName, "L7/AI rule key: LLM model name")

	return deleteLbCmd
}

func loadBalancerDeleteQuery(bgp bool, mark uint16, pathPrefix, pathMatchMode, modelName string) map[string]string {
	query := map[string]string{
		"bgp":   strconv.FormatBool(bgp),
		"block": strconv.FormatUint(uint64(mark), 10),
	}
	// L7/AI rule key components. The REST delete handler reads these from the
	// query string to target the exact model-aware rule.
	if pathPrefix != "" {
		query["path_prefix"] = pathPrefix
	}
	if pathMatchMode != "" {
		query["path_match_mode"] = pathMatchMode
	}
	if modelName != "" {
		query["model_name"] = modelName
	}
	return query
}

func PrintDeleteResult(resp *http.Response, o api.RESTOptions) {
	result := DeleteLoadBalancerResult{}
	resultByte, err := io.ReadAll(resp.Body)
	//fmt.Printf("Debug: response.Body: %s\n", string(resultByte))

	if err != nil {
		fmt.Printf("Error: Failed to read HTTP response: (%s)\n", err.Error())
		return
	}
	if err := json.Unmarshal(resultByte, &result); err != nil {
		fmt.Printf("Error: Failed to unmarshal HTTP response: (%s)\n", err.Error())
		return
	}

	if o.PrintOption == "json" {
		resultIndent, _ := json.MarshalIndent(resp.Body, "", "\t")
		fmt.Println(string(resultIndent))
		return
	}

	fmt.Printf("%s\n", result.Result)
}
