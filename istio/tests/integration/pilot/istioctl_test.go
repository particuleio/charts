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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	admin "github.com/envoyproxy/go-control-plane/envoy/admin/v3"
	"github.com/golang/protobuf/jsonpb"
	"github.com/onsi/gomega"

	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/echoboot"
	"istio.io/istio/pkg/test/framework/components/istioctl"
	"istio.io/istio/pkg/test/framework/components/namespace"
	kubetest "istio.io/istio/pkg/test/kube"
	"istio.io/istio/pkg/test/util/file"
	"istio.io/istio/pkg/test/util/retry"
	"istio.io/istio/pkg/url"
	"istio.io/istio/tests/integration/pilot/common"
)

var (
	// The full describe output is much larger, but testing for it requires a change anytime the test
	// app changes which is tedious. Instead, just check a minimum subset; unit test cover the
	// details.
	describeSvcAOutput = regexp.MustCompile(`(?s)Service: a\..*
   Port: http 80/HTTP targets pod port 18080
.*
80 DestinationRule: a\..* for "a"
   Matching subsets: v1
   No Traffic Policy
`)

	describePodAOutput = describeSvcAOutput

	addToMeshPodAOutput = `deployment .* updated successfully with Istio sidecar injected.
Next Step: Add related labels to the deployment to align with Istio's requirement: ` + url.DeploymentRequirements
	removeFromMeshPodAOutput = `deployment .* updated successfully with Istio sidecar un-injected.`
)

func TestWait(t *testing.T) {
	framework.NewTest(t).Features("usability.observability.wait").
		RequiresSingleCluster().
		Run(func(ctx framework.TestContext) {
			ns := namespace.NewOrFail(t, ctx, namespace.Config{
				Prefix: "default",
				Inject: true,
			})
			ctx.Config().ApplyYAMLOrFail(t, ns.Name(), `
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: reviews
spec:
  gateways: [missing-gw]
  hosts:
  - reviews
  http:
  - route:
    - destination: 
        host: reviews
`)
			istioCtl := istioctl.NewOrFail(ctx, ctx, istioctl.Config{Cluster: ctx.Environment().Clusters()[0]})
			istioCtl.InvokeOrFail(t, []string{"x", "wait", "-v", "VirtualService", "reviews." + ns.Name()})
		})
}

// This test requires `--istio.test.env=kube` because it tests istioctl doing PodExec
// TestVersion does "istioctl version --remote=true" to verify the CLI understands the data plane version data
func TestVersion(t *testing.T) {
	framework.
		NewTest(t).Features("usability.observability.version").
		RequiresSingleCluster().
		Run(func(ctx framework.TestContext) {
			cfg := i.Settings()

			istioCtl := istioctl.NewOrFail(ctx, ctx, istioctl.Config{Cluster: ctx.Environment().Clusters()[0]})
			args := []string{"version", "--remote=true", fmt.Sprintf("--istioNamespace=%s", cfg.SystemNamespace)}

			output, _ := istioCtl.InvokeOrFail(t, args)

			// istioctl will return a single "control plane version" if all control plane versions match
			controlPlaneRegex := regexp.MustCompile(`control plane version: [a-z0-9\-]*`)
			if controlPlaneRegex.MatchString(output) {
				return
			}

			ctx.Fatalf("Did not find control plane version: %v", output)
		})
}

func TestDescribe(t *testing.T) {
	framework.NewTest(t).Features("usability.observability.describe").
		RequiresSingleCluster().
		Run(func(ctx framework.TestContext) {
			deployment := file.AsStringOrFail(t, "testdata/a.yaml")
			ctx.Config().ApplyYAMLOrFail(ctx, apps.Namespace.Name(), deployment)

			istioCtl := istioctl.NewOrFail(ctx, ctx, istioctl.Config{})

			// When this test passed the namespace through --namespace it was flakey
			// because istioctl uses a global variable for namespace, and this test may
			// run in parallel.
			retry.UntilSuccessOrFail(ctx, func() error {
				args := []string{
					"--namespace=dummy",
					"x", "describe", "svc", fmt.Sprintf("%s.%s", common.PodASvc, apps.Namespace.Name()),
				}
				output, _, err := istioCtl.Invoke(args)
				if err != nil {
					return err
				}
				if !describeSvcAOutput.MatchString(output) {
					return fmt.Errorf("output:\n%v\n does not match regex:\n%v", output, describeSvcAOutput)
				}
				return nil
			}, retry.Timeout(time.Second*5))

			retry.UntilSuccessOrFail(ctx, func() error {
				podID, err := getPodID(apps.PodA[0])
				if err != nil {
					return fmt.Errorf("could not get Pod ID: %v", err)
				}
				args := []string{
					"--namespace=dummy",
					"x", "describe", "pod", fmt.Sprintf("%s.%s", podID, apps.Namespace.Name()),
				}
				output, _, err := istioCtl.Invoke(args)
				if err != nil {
					return err
				}
				if !describePodAOutput.MatchString(output) {
					return fmt.Errorf("output:\n%v\n does not match regex:\n%v", output, describePodAOutput)
				}
				return nil
			}, retry.Timeout(time.Second*5))
		})
}

func getPodID(i echo.Instance) (string, error) {
	wls, err := i.Workloads()
	if err != nil {
		return "", nil
	}

	for _, wl := range wls {
		hostname := strings.Split(wl.Sidecar().NodeID(), "~")[2]
		podID := strings.Split(hostname, ".")[0]
		return podID, nil
	}

	return "", fmt.Errorf("no workloads")
}

func TestAddToAndRemoveFromMesh(t *testing.T) {
	framework.NewTest(t).Features("usability.helpers.add-to-mesh", "usability.helpers.remove-from-mesh").
		RequiresSingleCluster().
		RunParallel(func(ctx framework.TestContext) {
			ns := namespace.NewOrFail(t, ctx, namespace.Config{
				Prefix: "istioctl-add-to-mesh",
				Inject: true,
			})

			var a echo.Instance
			echoboot.NewBuilder(ctx).
				With(&a, echoConfig(ns, "a")).
				BuildOrFail(ctx)

			istioCtl := istioctl.NewOrFail(ctx, ctx, istioctl.Config{Cluster: ctx.Environment().Clusters()[0]})

			var output string
			var args []string
			g := gomega.NewWithT(t)

			// able to remove from mesh when the deployment is auto injected
			args = []string{
				fmt.Sprintf("--namespace=%s", ns.Name()),
				"x", "remove-from-mesh", "service", "a",
			}
			output, _ = istioCtl.InvokeOrFail(t, args)
			g.Expect(output).To(gomega.MatchRegexp(removeFromMeshPodAOutput))

			retry.UntilSuccessOrFail(t, func() error {
				// Wait until the new pod is ready
				fetch := kubetest.NewPodMustFetch(ctx.Clusters().Default(), ns.Name(), "app=a")
				pods, err := kubetest.WaitUntilPodsAreReady(fetch)
				if err != nil {
					return err
				}
				for _, p := range pods {
					for _, c := range p.Spec.Containers {
						if c.Name == "istio-proxy" {
							return fmt.Errorf("sidecar still present in %v", p.Name)
						}
					}
				}
				return nil
			}, retry.Delay(time.Second))

			args = []string{
				fmt.Sprintf("--namespace=%s", ns.Name()),
				"x", "add-to-mesh", "service", "a",
			}
			output, _ = istioCtl.InvokeOrFail(t, args)
			g.Expect(output).To(gomega.MatchRegexp(addToMeshPodAOutput))
		})
}

func TestProxyConfig(t *testing.T) {
	framework.NewTest(t).Features("usability.observability.proxy-config").
		RequiresSingleCluster().
		Run(func(ctx framework.TestContext) {
			istioCtl := istioctl.NewOrFail(ctx, ctx, istioctl.Config{})

			podID, err := getPodID(apps.PodA[0])
			if err != nil {
				ctx.Fatalf("Could not get Pod ID: %v", err)
			}

			var output string
			var args []string
			g := gomega.NewWithT(t)

			args = []string{
				"--namespace=dummy",
				"pc", "bootstrap", fmt.Sprintf("%s.%s", podID, apps.Namespace.Name()),
			}
			output, _ = istioCtl.InvokeOrFail(t, args)
			jsonOutput := jsonUnmarshallOrFail(t, strings.Join(args, " "), output)
			g.Expect(jsonOutput).To(gomega.HaveKey("bootstrap"))

			args = []string{
				"--namespace=dummy",
				"pc", "cluster", fmt.Sprintf("%s.%s", podID, apps.Namespace.Name()), "-o", "json",
			}
			output, _ = istioCtl.InvokeOrFail(t, args)
			jsonOutput = jsonUnmarshallOrFail(t, strings.Join(args, " "), output)
			g.Expect(jsonOutput).To(gomega.Not(gomega.BeEmpty()))

			args = []string{
				"--namespace=dummy",
				"pc", "endpoint", fmt.Sprintf("%s.%s", podID, apps.Namespace.Name()), "-o", "json",
			}
			output, _ = istioCtl.InvokeOrFail(t, args)
			jsonOutput = jsonUnmarshallOrFail(t, strings.Join(args, " "), output)
			g.Expect(jsonOutput).To(gomega.Not(gomega.BeEmpty()))

			args = []string{
				"--namespace=dummy",
				"pc", "listener", fmt.Sprintf("%s.%s", podID, apps.Namespace.Name()), "-o", "json",
			}
			output, _ = istioCtl.InvokeOrFail(t, args)
			jsonOutput = jsonUnmarshallOrFail(t, strings.Join(args, " "), output)
			g.Expect(jsonOutput).To(gomega.Not(gomega.BeEmpty()))

			args = []string{
				"--namespace=dummy",
				"pc", "route", fmt.Sprintf("%s.%s", podID, apps.Namespace.Name()), "-o", "json",
			}
			output, _ = istioCtl.InvokeOrFail(t, args)
			jsonOutput = jsonUnmarshallOrFail(t, strings.Join(args, " "), output)
			g.Expect(jsonOutput).To(gomega.Not(gomega.BeEmpty()))

			args = []string{
				"--namespace=dummy",
				"pc", "secret", fmt.Sprintf("%s.%s", podID, apps.Namespace.Name()), "-o", "json",
			}
			output, _ = istioCtl.InvokeOrFail(t, args)
			jsonOutput = jsonUnmarshallOrFail(t, strings.Join(args, " "), output)
			g.Expect(jsonOutput).To(gomega.HaveKey("dynamicActiveSecrets"))
			dump := &admin.SecretsConfigDump{}
			if err := jsonpb.UnmarshalString(output, dump); err != nil {
				t.Fatal(err)
			}
			if len(dump.DynamicWarmingSecrets) > 0 {
				t.Fatalf("found warming secrets: %v", output)
			}
			if len(dump.DynamicActiveSecrets) != 2 {
				// If the config for the SDS does not align in all locations, we may get duplicates.
				// This check ensures we do not. If this is failing, check to ensure the bootstrap config matches
				// the XDS response.
				t.Fatalf("found unexpected secrets, should have only default and ROOTCA: %v", output)
			}
		})
}

func jsonUnmarshallOrFail(t *testing.T, context, s string) interface{} {
	t.Helper()
	var val interface{}

	// this is guarded by prettyPrint
	if err := json.Unmarshal([]byte(s), &val); err != nil {
		t.Fatalf("Could not unmarshal %s response %s", context, s)
	}
	return val
}

func TestProxyStatus(t *testing.T) {
	framework.NewTest(t).Features("usability.observability.proxy-status").
		RequiresSingleCluster().
		Run(func(ctx framework.TestContext) {
			istioCtl := istioctl.NewOrFail(ctx, ctx, istioctl.Config{})

			podID, err := getPodID(apps.PodA[0])
			if err != nil {
				ctx.Fatalf("Could not get Pod ID: %v", err)
			}

			var output string
			var args []string
			g := gomega.NewWithT(t)

			args = []string{"proxy-status"}
			output, _ = istioCtl.InvokeOrFail(t, args)
			// Just verify pod A is known to Pilot; implicitly this verifies that
			// the printing code printed it.
			g.Expect(output).To(gomega.ContainSubstring(fmt.Sprintf("%s.%s", podID, apps.Namespace.Name())))

			expectSubstrings := func(have string, wants ...string) error {
				for _, want := range wants {
					if !strings.Contains(have, want) {
						return fmt.Errorf("substring %q not found; have %q", want, have)
					}
				}
				return nil
			}

			retry.UntilSuccessOrFail(t, func() error {
				args = []string{
					"proxy-status", fmt.Sprintf("%s.%s", podID, apps.Namespace.Name()),
				}
				output, _ = istioCtl.InvokeOrFail(t, args)
				return expectSubstrings(output, "Clusters Match", "Listeners Match", "Routes Match")
			})

			// test the --file param
			retry.UntilSuccessOrFail(t, func() error {
				filename := "ps-configdump.json"
				cs := ctx.Clusters().Default()
				dump, err := cs.EnvoyDo(context.TODO(), podID, apps.Namespace.Name(), "GET", "config_dump", nil)
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
				err = ioutil.WriteFile(filename, dump, os.ModePerm)
				g.Expect(err).ShouldNot(gomega.HaveOccurred())
				args = []string{
					"proxy-status", fmt.Sprintf("%s.%s", podID, apps.Namespace.Name()), "--file", filename,
				}
				output, _ = istioCtl.InvokeOrFail(t, args)
				return expectSubstrings(output, "Clusters Match", "Listeners Match", "Routes Match")
			})
		})
}

func TestAuthZCheck(t *testing.T) {
	framework.NewTest(t).Features("usability.observability.authz-check").
		RequiresSingleCluster().
		Run(func(ctx framework.TestContext) {
			appPolicy := file.AsStringOrFail(t, "testdata/authz-a.yaml")
			gwPolicy := file.AsStringOrFail(t, "testdata/authz-b.yaml")
			ctx.Config().ApplyYAMLOrFail(ctx, apps.Namespace.Name(), appPolicy)
			ctx.Config().ApplyYAMLOrFail(ctx, i.Settings().SystemNamespace, gwPolicy)

			gwPod, err := i.IngressFor(ctx.Clusters().Default()).PodID(0)
			if err != nil {
				ctx.Fatalf("Could not get Pod ID: %v", err)
			}
			appPod, err := getPodID(apps.PodA[0])
			if err != nil {
				ctx.Fatalf("Could not get Pod ID: %v", err)
			}

			cases := []struct {
				name  string
				pod   string
				wants []*regexp.Regexp
			}{
				{
					name: "ingressgateway",
					pod:  fmt.Sprintf("%s.%s", gwPod, i.Settings().SystemNamespace),
					wants: []*regexp.Regexp{
						regexp.MustCompile(fmt.Sprintf(`DENY\s+deny-policy\.%s\s+2`, i.Settings().SystemNamespace)),
						regexp.MustCompile(fmt.Sprintf(`ALLOW\s+allow-policy\.%s\s+1`, i.Settings().SystemNamespace)),
					},
				},
				{
					name: "workload",
					pod:  fmt.Sprintf("%s.%s", appPod, apps.Namespace.Name()),
					wants: []*regexp.Regexp{
						regexp.MustCompile(fmt.Sprintf(`DENY\s+deny-policy\.%s\s+2`, apps.Namespace.Name())),
						regexp.MustCompile(`ALLOW\s+_anonymous_match_nothing_\s+1`),
						regexp.MustCompile(fmt.Sprintf(`ALLOW\s+allow-policy\.%s\s+1`, apps.Namespace.Name())),
					},
				},
			}

			istioCtl := istioctl.NewOrFail(ctx, ctx, istioctl.Config{Cluster: ctx.Environment().Clusters()[0]})
			for _, c := range cases {
				args := []string{"experimental", "authz", "check", c.pod}
				ctx.NewSubTest(c.name).Run(func(ctx framework.TestContext) {
					// Verify the output matches the expected text, which is the policies loaded above.
					retry.UntilSuccessOrFail(ctx, func() error {
						output, _ := istioCtl.InvokeOrFail(t, args)
						for _, want := range c.wants {
							if !want.MatchString(output) {
								return fmt.Errorf("%v did not match %v", output, want)
							}
						}
						return nil
					}, retry.Timeout(time.Second*5))
				})
			}
		})
}
