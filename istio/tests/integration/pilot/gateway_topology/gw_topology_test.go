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

package gwtopology

import (
	"testing"

	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/istio"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/tests/integration/pilot/common"
)

var (
	i    istio.Instance
	apps = &common.EchoDeployments{}
)

func TestMain(m *testing.M) {
	framework.
		NewSuite(m).
		Setup(istio.Setup(&i, func(ctx resource.Context, cfg *istio.Config) {
			cfg.ControlPlaneValues = `
meshConfig:
  defaultConfig:
    gatewayTopology:
      numTrustedProxies: 2`
		})).
		Setup(func(ctx resource.Context) error {
			return common.SetupApps(ctx, i, apps)
		}).
		Run()
}

func TestTraffic(t *testing.T) {
	framework.
		NewTest(t).
		Features("traffic.ingress.topology").
		Run(func(ctx framework.TestContext) {
			runXFFTrafficTests(ctx, apps)
		})
}

func runXFFTrafficTests(ctx framework.TestContext, apps *common.EchoDeployments) {
	cases := map[string][]common.TrafficTestCase{
		"xff": common.XFFGatewayCase(apps),
	}

	for name, tts := range cases {
		ctx.NewSubTest(name).Run(func(ctx framework.TestContext) {
			for _, tt := range tts {
				tt.Run(ctx, apps.Namespace.Name())
			}
		})
	}
}
