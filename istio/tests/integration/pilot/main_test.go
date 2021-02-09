// +build integ
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

package pilot

import (
	"io/ioutil"
	"testing"

	"istio.io/istio/pkg/config/protocol"
	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/istio"
	"istio.io/istio/pkg/test/framework/components/namespace"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/tests/integration/pilot/common"
)

var (
	i istio.Instance

	// Below are various preconfigured echo deployments. Whenever possible, tests should utilize these
	// to avoid excessive creation/tear down of deployments. In general, a test should only deploy echo if
	// its doing something unique to that specific test.
	apps = &common.EchoDeployments{}
)

func supportsCRDv1(ctx resource.Context) bool {
	for _, cluster := range ctx.Clusters() {
		if !cluster.MinKubeVersion(1, 16) {
			return false
		}
	}
	return true
}

// TestMain defines the entrypoint for pilot tests using a standard Istio installation.
// If a test requires a custom install it should go into its own package, otherwise it should go
// here to reuse a single install across tests.
func TestMain(m *testing.M) {
	framework.
		NewSuite(m).
		Setup(func(ctx resource.Context) (err error) {
			if supportsCRDv1(ctx) {
				crd, err := ioutil.ReadFile("testdata/service-apis-crd.yaml")
				if err != nil {
					return err
				}
				return ctx.Config().ApplyYAML("", string(crd))
			}
			return nil
		}).
		Setup(istio.Setup(&i, nil)).
		Setup(func(ctx resource.Context) error {
			return common.SetupApps(ctx, i, apps)
		}).
		Run()
}

func echoConfig(ns namespace.Instance, name string) echo.Config {
	return echo.Config{
		Service:   name,
		Namespace: ns,
		Ports: []echo.Port{
			{
				Name:     "http",
				Protocol: protocol.HTTP,
				// We use a port > 1024 to not require root
				InstancePort: 8090,
			},
		},
		Subsets: []echo.SubsetConfig{{}},
	}
}
