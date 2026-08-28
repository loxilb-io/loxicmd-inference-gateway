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
package create

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/loxilb-io/loxicmd-inference-gateway/pkg/api"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type CreatePolicyOptions struct {
	Ident   string
	Rate    string
	Block   string
	Target  string
	Color   bool
	PolType int
}

type CreatePolicyResult struct {
	Result string `json:"result"`
}

func ReadCreatePolicyOptions(o *CreatePolicyOptions, args []string) error {
	if len(args) > 1 {
		fmt.Println("create Pol command get so many args")
		fmt.Println(args)
	} else if len(args) <= 0 {
		return errors.New("create Pol need Ident args")
	}
	o.Ident = args[0]
	return nil
}

func GetRatePair(body *api.PolMod, RateBlock string) error {
	if RateBlock == "" {
		return nil
	}
	RatePair := strings.Split(RateBlock, ":")
	if len(RatePair) != 2 {
		return errors.New("lots of args for rate")
	}
	Peak, err := strconv.Atoi(RatePair[0])
	if err != nil {
		return fmt.Errorf("peak '%s' is not integer", RatePair[0])
	}

	Committed, err := strconv.Atoi(RatePair[1])
	if err != nil {
		return fmt.Errorf("committed '%s' is not integer", RatePair[1])
	}
	body.Info.CommittedInfoRate = uint64(Committed)
	body.Info.PeakInfoRate = uint64(Peak)
	return nil
}

func GetBlockPair(body *api.PolMod, Block string) error {
	if Block == "" {
		return nil
	}
	BlockPair := strings.Split(Block, ":")
	if len(BlockPair) != 2 {
		return errors.New("error in block size args")
	}
	Excess, err := strconv.Atoi(BlockPair[0])
	if err != nil {
		return fmt.Errorf("excess '%s' is not integer", BlockPair[0])
	}

	Committed, err := strconv.Atoi(BlockPair[1])
	if err != nil {
		return fmt.Errorf("committed '%s' is not integer", BlockPair[1])
	}
	body.Info.ExcessBlkSize = uint64(Excess)
	body.Info.CommittedBlkSize = uint64(Committed)
	return nil
}

func GetTargetPair(body *api.PolMod, Block string) error {
	separator := strings.LastIndex(Block, ":")
	if separator <= 0 || separator == len(Block)-1 {
		return errors.New("target must use '<object>:<rule|port|egress-port>' (numeric 0|1|2 is also accepted)")
	}
	objectName := Block[:separator]
	attachment, err := policyAttachment(Block[separator+1:])
	if err != nil {
		return err
	}
	if attachment == 0 {
		if err := validatePolicyRuleTarget(objectName); err != nil {
			return err
		}
	} else if strings.Contains(objectName, ":") {
		return fmt.Errorf("port attachment target %q must be a port name", objectName)
	}
	body.Target.PolObjName = objectName
	body.Target.AttachMent = api.PolObjType(attachment)
	return nil
}

func policyAttachment(value string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "0", "rule":
		return 0, nil
	case "1", "port":
		return 1, nil
	case "2", "egress-port":
		return 2, nil
	default:
		return 0, fmt.Errorf("attachment %q must be rule|port|egress-port or 0|1|2", value)
	}
}

func validatePolicyRuleTarget(target string) error {
	protocolSeparator := strings.LastIndex(target, ":")
	if protocolSeparator <= 0 || protocolSeparator == len(target)-1 {
		return fmt.Errorf("rule target %q must use VIP:PORT:PROTO or [IPv6]:PORT:PROTO", target)
	}
	hostPort := target[:protocolSeparator]
	protocol := strings.ToLower(target[protocolSeparator+1:])
	switch protocol {
	case "tcp", "udp", "sctp", "icmp":
	default:
		return fmt.Errorf("rule target protocol %q must be tcp|udp|sctp|icmp", protocol)
	}
	host, rawPort, err := net.SplitHostPort(hostPort)
	if err != nil {
		return fmt.Errorf("rule target %q must bracket IPv6 and use VIP:PORT:PROTO", target)
	}
	if net.ParseIP(host) == nil {
		return fmt.Errorf("rule target VIP %q is not a valid IP address", host)
	}
	if _, err := strconv.ParseUint(rawPort, 10, 16); err != nil {
		return fmt.Errorf("rule target port %q must be within 0..65535", rawPort)
	}
	return nil
}

func NewCreatePolicyCmd(restOptions *api.RESTOptions) *cobra.Command {
	o := CreatePolicyOptions{}

	var createPolCmd = &cobra.Command{
		Use:   "policy IDENT --rate=<Peak>:<Committed> --target=<ObjectName>:<rule|port|egress-port> [--block-size=<Excess>:<Committed>] [--color] [--pol-type=<policy type>]",
		Short: "Create a Policy",
		Long: `Create a Policy 
Ex) loxicmd create policy pol-hs0 --rate=100:100 --target=hs0:1
    loxicmd create policy pol-hs1 --rate=100:100 --target=hs0:1 --block-size=12000:6000
    loxicmd create policy pol-hs1 --rate=100:100 --target=hs0:1 --color
    loxicmd create policy pol-hs1 --rate=100:100 --target=hs0:1 --color --pol-type 0

rate unit : Mbps
block-size unit : bps
Policy type(pol-type) 0 : TrTCM,  1 : SrTCM

	`,
		Aliases: []string{"pol", "policys", "pols", "polices"},
		PreRun: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				cmd.Help()
				os.Exit(0)
			}
		},
		Run: func(cmd *cobra.Command, args []string) {
			if err := ReadCreatePolicyOptions(&o, args); err != nil {
				fmt.Printf("Read parameter error %s\n", err.Error())
				return
			}
			// Make body
			body := api.PolMod{}

			body.Ident = o.Ident
			if err := GetRatePair(&body, o.Rate); err != nil {
				fmt.Printf("Rate Error: %s\n", err.Error())
				return
			}
			if err := GetBlockPair(&body, o.Block); err != nil {
				fmt.Printf("Block Error: %s\n", err.Error())
				return
			}

			if err := GetTargetPair(&body, o.Target); err != nil {
				fmt.Printf("Target Error: %s\n", err.Error())
				return
			}
			body.Info.ColorAware = o.Color
			body.Info.PolType = o.PolType
			resp, err := PolicyAPICall(restOptions, body)
			if err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				return
			}
			defer resp.Body.Close()

			fmt.Printf("Debug: response.StatusCode: %d\n", resp.StatusCode)
			if resp.StatusCode != http.StatusOK {
				PrintCreatePolResult(resp, *restOptions)
				return
			}

		},
	}

	createPolCmd.Flags().StringVar(&o.Rate, "rate", o.Rate, "Rate pairs can be specified as '<Peak>:<Committed>'")
	createPolCmd.Flags().StringVar(&o.Block, "block-size", o.Block, "Block Size pairs can be specified as '<Excess>:<Committed>'")
	createPolCmd.Flags().StringVar(&o.Target, "target", o.Target, "Target '<ObjectName>:<rule|port|egress-port>'; numeric 0|1|2 remains accepted")
	createPolCmd.Flags().BoolVarP(&o.Color, "color", "", false, "Policy color enbale or not")
	createPolCmd.Flags().IntVar(&o.PolType, "pol-type", o.PolType, "Target Interface pairs can be specified as '<ObjectName>:<Attachment>'")

	return createPolCmd
}

func PrintCreatePolResult(resp *http.Response, o api.RESTOptions) {
	result := CreatePolicyResult{}
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

func PolicyAPICall(restOptions *api.RESTOptions, PolModel api.PolMod) (*http.Response, error) {
	client := api.NewLoxiClient(restOptions)
	ctx := context.TODO()
	var cancel context.CancelFunc
	if restOptions.Timeout > 0 {
		ctx, cancel = context.WithTimeout(context.TODO(), time.Duration(restOptions.Timeout)*time.Second)
		defer cancel()
	}

	return client.Policy().Create(ctx, PolModel)
}
