// +build integ
//  Copyright Istio Authors
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package security

import (
	"fmt"
	"testing"
	"time"

	"istio.io/istio/pkg/config/protocol"
	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/echoboot"
	"istio.io/istio/pkg/test/framework/components/namespace"
)

// TestMtlsHealthCheck verifies Kubernetes HTTP health check can work when mTLS
// is enabled, https://github.com/istio/istio/issues/9150.
// Currently this test can only pass on Prow with a real GKE cluster, and fail
// on Minikube. For more details, see https://github.com/istio/istio/issues/12754.
func TestMtlsHealthCheck(t *testing.T) {
	framework.NewTest(t).
		Features("security.healthcheck").
		Run(func(ctx framework.TestContext) {
			ns := namespace.NewOrFail(t, ctx, namespace.Config{Prefix: "healthcheck", Inject: true})
			for _, testCase := range []struct {
				name    string
				rewrite bool
			}{
				{name: "norewrite-fail", rewrite: false},
				{name: "rewrite-success", rewrite: true},
			} {
				t.Run(testCase.name, func(t *testing.T) {
					runHealthCheckDeployment(t, ctx, ns, testCase.name, testCase.rewrite)
				})
			}
		})
}

func runHealthCheckDeployment(t *testing.T, ctx framework.TestContext, ns namespace.Instance, //nolint:interfacer
	name string, rewrite bool) {
	t.Helper()
	wantSuccess := rewrite
	policyYAML := fmt.Sprintf(`apiVersion: "security.istio.io/v1beta1"
kind: "PeerAuthentication"
metadata:
  name: "mtls-strict-for-%v"
spec:
  selector:
    matchLabels:
      app: "%v"
  mtls:
    mode: STRICT
`, name, name)
	ctx.Config().ApplyYAMLOrFail(t, ns.Name(), policyYAML)

	var healthcheck echo.Instance
	cfg := echo.Config{
		Namespace: ns,
		Service:   name,
		Ports: []echo.Port{{
			Name:         "http-8080",
			Protocol:     protocol.HTTP,
			ServicePort:  8080,
			InstancePort: 8080,
		}},
		Subsets: []echo.SubsetConfig{
			{
				Annotations: echo.NewAnnotations().SetBool(echo.SidecarRewriteAppHTTPProbers, rewrite),
			},
		},
	}
	// Negative test, we expect the health check fails, so set a timeout duration.
	if !rewrite {
		cfg.ReadinessTimeout = time.Second * 15
	}
	_, err := echoboot.NewBuilder(ctx).
		With(&healthcheck, cfg).
		Build()
	gotSuccess := err == nil
	if gotSuccess != wantSuccess {
		t.Errorf("health check app %v, got error %v, want success = %v", name, err, wantSuccess)
	}
}
