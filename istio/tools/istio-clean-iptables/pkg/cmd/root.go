// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"istio.io/istio/tools/istio-iptables/pkg/constants"
	"istio.io/pkg/log"
)

var rootCmd = &cobra.Command{
	Use:   "istio-clean-iptables",
	Short: "Clean up iptables rules for Istio Sidecar",
	Long:  "Script responsible for cleaning up iptables rules",
	PreRun: func(cmd *cobra.Command, args []string) {
		if err := viper.BindPFlag(constants.DryRun, cmd.Flags().Lookup(constants.DryRun)); err != nil {
			handleError(err)
		}
		viper.SetDefault(constants.DryRun, false)
	},
	Run: func(cmd *cobra.Command, args []string) {
		cleanup(viper.GetBool(constants.DryRun))
	},
}

func init() {
	// Read in all environment variables
	viper.AutomaticEnv()
	// Replace - with _; so that environment variables are looked up correctly.
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// https://github.com/spf13/viper/issues/233.
	// The `dry-run` flag is bound in init() across both `istio-iptables` and `istio-clean-iptables` subcommands and
	// will be overwritten by the last. Thus, only adding it here while moving its binding to Viper and value
	// defaulting as part of the command execution.
	rootCmd.Flags().BoolP(constants.DryRun, "n", false, "Do not call any external dependencies like iptables")
}

func GetCommand() *cobra.Command {
	return rootCmd
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		handleError(err)
	}
}

func handleError(err error) {
	log.Error(err)
	os.Exit(1)
}
