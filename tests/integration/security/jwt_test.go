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

package security

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"istio.io/istio/pkg/test/echo/common/response"
	"istio.io/istio/pkg/test/echo/common/scheme"
	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/istio"
	"istio.io/istio/pkg/test/framework/components/namespace"
	"istio.io/istio/pkg/test/kube"
	"istio.io/istio/pkg/test/util/file"
	"istio.io/istio/pkg/test/util/retry"
	"istio.io/istio/pkg/test/util/tmpl"
	"istio.io/istio/tests/common/jwt"
	"istio.io/istio/tests/integration/security/util"
	"istio.io/istio/tests/integration/security/util/authn"
	"istio.io/istio/tests/integration/security/util/connection"
)

const (
	authHeaderKey = "Authorization"
)

// TestRequestAuthentication tests beta authn policy for jwt.
func TestRequestAuthentication(t *testing.T) {
	payload1 := strings.Split(jwt.TokenIssuer1, ".")[1]
	payload2 := strings.Split(jwt.TokenIssuer2, ".")[1]
	framework.NewTest(t).
		Features("security.authentication.jwt").
		Run(func(ctx framework.TestContext) {
			ns := apps.Namespace1
			args := map[string]string{"Namespace": ns.Name()}
			applyYAML := func(filename string, ns namespace.Instance) []string {
				policy := tmpl.EvaluateAllOrFail(t, args, file.AsStringOrFail(t, filename))
				ctx.Config().ApplyYAMLOrFail(t, ns.Name(), policy...)
				return policy
			}

			jwtServer := applyYAML("../../../samples/jwt-server/jwt-server.yaml", ns)
			defer ctx.Config().DeleteYAMLOrFail(t, ns.Name(), jwtServer...)
			if _, _, err := kube.WaitUntilServiceEndpointsAreReady(ctx.Clusters().Default(), ns.Name(), "jwt-server"); err != nil {
				ctx.Fatalf("Wait for jwt-server server failed: %v", err)
			}

			// Apply the policy.
			namespaceTmpl := map[string]string{
				"Namespace": ns.Name(),
			}
			jwtPolicies := tmpl.EvaluateAllOrFail(t, namespaceTmpl,
				file.AsStringOrFail(t, "testdata/requestauthn/a-authn.yaml.tmpl"),
				file.AsStringOrFail(t, "testdata/requestauthn/b-authn-authz.yaml.tmpl"),
				file.AsStringOrFail(t, "testdata/requestauthn/c-authn.yaml.tmpl"),
				file.AsStringOrFail(t, "testdata/requestauthn/e-authn.yaml.tmpl"),
				file.AsStringOrFail(t, "testdata/requestauthn/f-authn.yaml.tmpl"),
				file.AsStringOrFail(t, "testdata/requestauthn/g-authn.yaml.tmpl"),
			)
			ctx.Config().ApplyYAMLOrFail(t, ns.Name(), jwtPolicies...)

			aSet := apps.A.Match(echo.Namespace(ns.Name()))
			bSet := apps.B.Match(echo.Namespace(ns.Name()))
			cSet := apps.C.Match(echo.Namespace(ns.Name()))
			dSet := apps.D.Match(echo.Namespace(ns.Name()))
			eSet := apps.E.Match(echo.Namespace(ns.Name()))
			fSet := apps.F.Match(echo.Namespace(ns.Name()))
			gSet := apps.G.Match(echo.Namespace(ns.Name()))

			callCount := 1
			if ctx.Clusters().IsMulticluster() {
				// so we can validate all clusters are hit
				callCount = util.CallsPerCluster * len(ctx.Clusters())
			}

			for _, cluster := range ctx.Clusters() {
				ctx.NewSubTest(fmt.Sprintf("From %s", cluster.StableName())).Run(func(ctx framework.TestContext) {
					a := apps.A.Match(echo.InCluster(cluster).And(echo.Namespace(apps.Namespace1.Name())))
					b := apps.B.Match(echo.InCluster(cluster).And(echo.Namespace(apps.Namespace1.Name())))
					testCases := []authn.TestCase{
						{
							Name: "valid-token-noauthz",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   cSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Headers: map[string][]string{
										authHeaderKey: {"Bearer " + jwt.TokenIssuer1},
									},
									Count: callCount,
								},
								DestClusters: cSet.Clusters(),
							},
							ExpectResponseCode: response.StatusCodeOK,
							ExpectHeaders: map[string]string{
								authHeaderKey:    "",
								"X-Test-Payload": payload1,
							},
						},
						{
							Name: "valid-token-2-noauthz",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   cSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Headers: map[string][]string{
										authHeaderKey: {"Bearer " + jwt.TokenIssuer2},
									},
									Count: callCount,
								},
								DestClusters: cSet.Clusters(),
							},
							ExpectResponseCode: response.StatusCodeOK,
							ExpectHeaders: map[string]string{
								authHeaderKey:    "",
								"X-Test-Payload": payload2,
							},
						},
						{
							Name: "expired-token-noauthz",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   cSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Headers: map[string][]string{
										authHeaderKey: {"Bearer " + jwt.TokenExpired},
									},
									Count: callCount,
								},
								DestClusters: cSet.Clusters(),
							},
							ExpectResponseCode: response.StatusUnauthorized,
						},
						{
							Name: "no-token-noauthz",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   cSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Count:    callCount,
								},
								DestClusters: cSet.Clusters(),
							},
							ExpectResponseCode: response.StatusCodeOK,
						},
						// Following app b is configured with authorization, only request with valid JWT succeed.
						{
							Name: "valid-token",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   bSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Headers: map[string][]string{
										authHeaderKey: {"Bearer " + jwt.TokenIssuer1},
									},
									Count: callCount,
								},
								DestClusters: bSet.Clusters(),
							},
							ExpectResponseCode: response.StatusCodeOK,
							ExpectHeaders: map[string]string{
								authHeaderKey: "",
							},
						},
						{
							Name: "expired-token",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   bSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Headers: map[string][]string{
										authHeaderKey: {"Bearer " + jwt.TokenExpired},
									},
									Count: callCount,
								},
								DestClusters: bSet.Clusters(),
							},
							ExpectResponseCode: response.StatusUnauthorized,
						},
						{
							Name: "no-token",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   bSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Count:    callCount,
								},
								DestClusters: bSet.Clusters(),
							},
							ExpectResponseCode: response.StatusCodeForbidden,
						},
						{
							Name: "no-authn-authz",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   dSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Count:    callCount,
								},
								DestClusters: dSet.Clusters(),
							},
							ExpectResponseCode: response.StatusCodeOK,
						},
						{
							Name: "valid-token-forward",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   eSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Headers: map[string][]string{
										authHeaderKey: {"Bearer " + jwt.TokenIssuer1},
									},
									Count: callCount,
								},
								DestClusters: eSet.Clusters(),
							},
							ExpectResponseCode: response.StatusCodeOK,
							ExpectHeaders: map[string]string{
								authHeaderKey:    "Bearer " + jwt.TokenIssuer1,
								"X-Test-Payload": payload1,
							},
						},
						{
							Name: "valid-token-forward-remote-jwks",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   gSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Headers: map[string][]string{
										authHeaderKey: {"Bearer " + jwt.TokenIssuer1},
									},
									Count: callCount,
								},
								DestClusters: gSet.Clusters(),
							},
							ExpectResponseCode: response.StatusCodeOK,
							ExpectHeaders: map[string]string{
								authHeaderKey:    "Bearer " + jwt.TokenIssuer1,
								"X-Test-Payload": payload1,
							},
						},
						{
							Name: "invalid aud",
							Request: connection.Checker{
								From: b[0],
								Options: echo.CallOptions{
									Target:   aSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Headers: map[string][]string{
										authHeaderKey: {"Bearer " + jwt.TokenIssuer1},
									},
									Count: callCount,
								},
								DestClusters: aSet.Clusters(),
							},
							ExpectResponseCode: response.StatusCodeForbidden,
						},
						{
							Name: "valid aud",
							Request: connection.Checker{
								From: b[0],
								Options: echo.CallOptions{
									Target:   aSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Headers: map[string][]string{
										authHeaderKey: {"Bearer " + jwt.TokenIssuer1WithAud},
									},
									Count: callCount,
								},
								DestClusters: aSet.Clusters(),
							},
							ExpectResponseCode: response.StatusCodeOK,
						},
						{
							Name: "verify policies are combined",
							Request: connection.Checker{
								From: b[0],
								Options: echo.CallOptions{
									Target:   aSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Headers: map[string][]string{
										authHeaderKey: {"Bearer " + jwt.TokenIssuer2},
									},
									Count: callCount,
								},
								DestClusters: aSet.Clusters(),
							},
							ExpectResponseCode: response.StatusCodeOK,
						},
						{
							Name: "invalid-jwks-valid-token-noauthz",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   fSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Headers: map[string][]string{
										authHeaderKey: {"Bearer " + jwt.TokenIssuer1},
									},
									Count: callCount,
								},
								DestClusters: fSet.Clusters(),
							},
							ExpectResponseCode: response.StatusUnauthorized,
						},
						{
							Name: "invalid-jwks-expired-token-noauthz",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   fSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Headers: map[string][]string{
										authHeaderKey: {"Bearer " + jwt.TokenExpired},
									},
									Count: callCount,
								},
								DestClusters: fSet.Clusters(),
							},
							ExpectResponseCode: response.StatusUnauthorized,
						},
						{
							Name: "invalid-jwks-no-token-noauthz",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   fSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Count:    callCount,
								},
								DestClusters: fSet.Clusters(),
							},
							ExpectResponseCode: response.StatusCodeOK,
						},
					}
					for _, c := range testCases {
						ctx.NewSubTest(c.Name).Run(func(ctx framework.TestContext) {
							retry.UntilSuccessOrFail(ctx, c.CheckAuthn,
								retry.Delay(250*time.Millisecond), retry.Timeout(30*time.Second))
						})
					}
				})
			}
		})
}

// TestIngressRequestAuthentication tests beta authn policy for jwt on ingress.
// The policy is also set at global namespace, with authorization on ingressgateway.
func TestIngressRequestAuthentication(t *testing.T) {
	framework.NewTest(t).
		Features("security.authentication.ingressjwt").
		Run(func(ctx framework.TestContext) {
			ns := apps.Namespace1

			// Apply the policy.
			namespaceTmpl := map[string]string{
				"Namespace":     ns.Name(),
				"RootNamespace": istio.GetOrFail(ctx, ctx).Settings().SystemNamespace,
			}

			applyPolicy := func(filename string, ns namespace.Instance) {
				policy := tmpl.EvaluateAllOrFail(t, namespaceTmpl, file.AsStringOrFail(t, filename))
				ctx.Config().ApplyYAMLOrFail(t, ns.Name(), policy...)
			}

			applyPolicy("testdata/requestauthn/global-jwt.yaml.tmpl", rootNS{})
			applyPolicy("testdata/requestauthn/ingress.yaml.tmpl", ns)

			bSet := apps.B.Match(echo.Namespace(ns.Name()))

			callCount := 1
			if ctx.Clusters().IsMulticluster() {
				// so we can validate all clusters are hit
				callCount = util.CallsPerCluster * len(ctx.Clusters())
			}

			for _, cluster := range ctx.Clusters() {
				ctx.NewSubTest(fmt.Sprintf("In %s", cluster.StableName())).Run(func(ctx framework.TestContext) {
					a := apps.A.Match(echo.InCluster(cluster).And(echo.Namespace(apps.Namespace1.Name())))
					// These test cases verify in-mesh traffic doesn't need tokens.
					testCases := []authn.TestCase{
						{
							Name: "in-mesh-with-expired-token",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   bSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Headers: map[string][]string{
										authHeaderKey: {"Bearer " + jwt.TokenExpired},
									},
									Count: callCount,
								},
								DestClusters: bSet.Clusters(),
							},
							ExpectResponseCode: response.StatusUnauthorized,
						},
						{
							Name: "in-mesh-without-token",
							Request: connection.Checker{
								From: a[0],
								Options: echo.CallOptions{
									Target:   bSet[0],
									PortName: "http",
									Scheme:   scheme.HTTP,
									Count:    callCount,
								},
								DestClusters: bSet.Clusters(),
							},
							ExpectResponseCode: response.StatusCodeOK,
						},
					}
					for _, c := range testCases {
						ctx.NewSubTest(c.Name).Run(func(ctx framework.TestContext) {
							retry.UntilSuccessOrFail(ctx, c.CheckAuthn,
								retry.Delay(250*time.Millisecond), retry.Timeout(30*time.Second))
						})
					}
					ingr := ist.IngressFor(cluster)
					// These test cases verify requests go through ingress will be checked for validate token.
					ingTestCases := []struct {
						Name               string
						Host               string
						Path               string
						Token              string
						ExpectResponseCode int
					}{
						{
							Name:               "deny without token",
							Host:               "example.com",
							Path:               "/",
							ExpectResponseCode: 403,
						},
						{
							Name:               "allow with sub-1 token",
							Host:               "example.com",
							Path:               "/",
							Token:              jwt.TokenIssuer1,
							ExpectResponseCode: 200,
						},
						{
							Name:               "deny with sub-2 token",
							Host:               "example.com",
							Path:               "/",
							Token:              jwt.TokenIssuer2,
							ExpectResponseCode: 403,
						},
						{
							Name:               "deny with expired token",
							Host:               "example.com",
							Path:               "/",
							Token:              jwt.TokenExpired,
							ExpectResponseCode: 401,
						},
						{
							Name:               "allow with sub-1 token on any.com",
							Host:               "any-request-principlal-ok.com",
							Path:               "/",
							Token:              jwt.TokenIssuer1,
							ExpectResponseCode: 200,
						},
						{
							Name:               "allow with sub-2 token on any.com",
							Host:               "any-request-principlal-ok.com",
							Path:               "/",
							Token:              jwt.TokenIssuer2,
							ExpectResponseCode: 200,
						},
						{
							Name:               "deny without token on any.com",
							Host:               "any-request-principlal-ok.com",
							Path:               "/",
							ExpectResponseCode: 403,
						},
						{
							Name:               "deny with token on other host",
							Host:               "other-host.com",
							Path:               "/",
							Token:              jwt.TokenIssuer1,
							ExpectResponseCode: 403,
						},
						{
							Name:               "allow healthz",
							Host:               "example.com",
							Path:               "/healthz",
							ExpectResponseCode: 200,
						},
					}

					for _, c := range ingTestCases {
						ctx.NewSubTest(c.Name).Run(func(ctx framework.TestContext) {
							authn.CheckIngressOrFail(ctx, ingr, c.Host, c.Path, nil, c.Token, c.ExpectResponseCode)
						})
					}
				})
			}
		})
}
