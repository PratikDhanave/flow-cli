/*
 * Flow CLI
 *
 * Copyright 2019-2020 Dapper Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package main implements the entry point for the Flow CLI.
package main

import "C"
import (
	"encoding/json"
	"fmt"
	"github.com/onflow/flow-cli/cmd"
	"github.com/onflow/flow-cli/cmd/accounts"
	"github.com/onflow/flow-cli/flow/blocks"
	"github.com/onflow/flow-cli/flow/cadence"
	"github.com/onflow/flow-cli/flow/cli"
	"github.com/onflow/flow-cli/flow/collections"
	"github.com/onflow/flow-cli/flow/emulator"
	"github.com/onflow/flow-cli/flow/events"
	"github.com/onflow/flow-cli/flow/initialize"
	"github.com/onflow/flow-cli/flow/keys"
	"github.com/onflow/flow-cli/flow/project"
	"github.com/onflow/flow-cli/flow/scripts"
	"github.com/onflow/flow-cli/flow/transactions"
	"github.com/onflow/flow-cli/flow/version"
	"github.com/onflow/flow-cli/sharedlib/services"
	"github.com/onflow/flow-go-sdk/client"
	"github.com/psiemens/sconfig"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var c = &cobra.Command{
	Use:              "flow",
	TraverseChildren: true,
}

func init() {
	c.AddCommand(project.Cmd)
	c.AddCommand(initialize.Cmd)
	c.AddCommand(blocks.Cmd)
	c.AddCommand(collections.Cmd)
	c.AddCommand(keys.Cmd)
	c.AddCommand(emulator.Cmd)
	c.AddCommand(events.Cmd)
	c.AddCommand(cadence.Cmd)
	c.AddCommand(scripts.Cmd)
	c.AddCommand(transactions.Cmd)
	c.AddCommand(version.Cmd)

	c.PersistentFlags().StringSliceVarP(&cli.ConfigPath, "config-path", "f", cli.ConfigPath, "Path to flow configuration file")

	newInit()
}

var (
	filter      = ""
	format      = ""
	save        = ""
	runEmulator = false
)

func newInit() {
	addCommand(c, accounts.Init())

	c.PersistentFlags().StringVarP(&filter, "filter", "", filter, "Filter result values by property name")
	c.PersistentFlags().StringVarP(&format, "format", "", format, "Format to show result in")
	c.PersistentFlags().StringVarP(&save, "save", "", save, "Save result to a filename")
	c.PersistentFlags().BoolVarP(&runEmulator, "emulator", "", runEmulator, "Run in-memory emulator")

}

// addCommand add new command to main cmd
// and initializes all necessary things as well as take care of errors and output
// here we can do all boilerplate code that is else copied in each command and make sure
// we have one place to handle all errors and ensure commands have consistent results
func addCommand(c *cobra.Command, command cmd.Command) {
	command.GetCmd().RunE = func(cmd *cobra.Command, args []string) error {

		// initialize project but ignore error since config can be missing
		project, _ := cli.LoadProject(cli.ConfigPath)

		service, err := createService(cmd, project)
		handleError("Service Error", err)

		// run command
		result, err := command.Run(cmd, args, project, service)
		handleError("Command Error", err)

		// format output result
		formattedResult := formatResult(cmd, result)

		// output result
		err = outputResult(cmd, formattedResult)
		handleError("Output Error", err)

		return nil
	}

	bindFlags(command.SetFlags())
	c.AddCommand(command.GetCmd())
}

// createService creates a service to be used, defaults to grpc but can support others
func createService(cmd *cobra.Command, project *cli.Project) (services.Service, error) {
	// create in memory emulator client
	if runEmulator {
		return services.NewEmulatorService(), nil
	}

	// resolve host - todo: handle if host is nil (version command)
	host := cmd.Flag("host").Value.String()
	if host == "" && project != nil {
		host = project.Host("emulator")
	} else if host == "" {
		return nil, fmt.Errorf("Host must be provided using --host flag or in config by initializing project: flow project init")
	}

	// create default grpc client
	return services.NewRpcService(host)
}

// outputResult takes care of showing the result
func formatResult(cmd *cobra.Command, result cmd.Result) string {
	filter := cmd.Flag("filter").Value.String()
	format := cmd.Flag("format").Value.String()

	if filter != "" {
		var jsonResult map[string]interface{}
		val, _ := json.Marshal(result.JSON())
		json.Unmarshal(val, &jsonResult)

		return fmt.Sprintf("%v", jsonResult[filter])
	}

	if format == "json" {
		jsonRes, _ := json.Marshal(result.JSON())
		return string(jsonRes)
	}

	if format == "inline" {
		return result.Oneliner()
	}

	return result.String()
}

// outputResult to selected media
func outputResult(cmd *cobra.Command, result string) error {
	save := cmd.Flag("save").Value.String()

	if save != "" {
		af := afero.Afero{
			Fs: afero.NewOsFs(),
		}

		fmt.Printf("💾 Result saved to: %s \n", save)
		return af.WriteFile(save, []byte(result), 0644)
	}

	// todo: grep output

	// default normal output - todo: this can be changed to writer so we can test outputs easier
	fmt.Fprintf(os.Stdout, "%s\n", result)
	return nil
}

// handleError handle errors
func handleError(description string, err error) {
	if err == nil {
		return
	}

	// handle rpc error
	switch t := err.(type) {
	case *client.RPCError:
		fmt.Fprintf(os.Stderr, "🔴️ Grpc Error: %s \n", t.GRPCStatus().Err)
	default:
		if strings.Contains(err.Error(), "transport:") {
			fmt.Fprintf(os.Stderr, "🔴️ %s \n", strings.Split(err.Error(), "transport:")[1])
		} else {
			fmt.Fprintf(os.Stderr, "🔴️ %s: %s", description, err)
		}
	}

	os.Exit(1)
}

// bindFlags bind all the flags needed
func bindFlags(config *sconfig.Config) {
	err := config.
		FromEnvironment(cli.EnvPrefix).
		BindFlags(c.PersistentFlags()).
		Parse()
	if err != nil {
		fmt.Println(err)
	}
}

func main() {
	if err := c.Execute(); err != nil {
		cli.Exit(1, err.Error())
	}
}
