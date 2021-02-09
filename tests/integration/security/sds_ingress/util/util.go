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

package util

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"istio.io/istio/pkg/config/protocol"
	"istio.io/istio/pkg/test"
	"istio.io/istio/pkg/test/echo/client"
	"istio.io/istio/pkg/test/framework"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/components/echo/echoboot"
	"istio.io/istio/pkg/test/framework/components/istio"
	"istio.io/istio/pkg/test/framework/components/istio/ingress"
	"istio.io/istio/pkg/test/framework/components/namespace"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/util/retry"
)

const (
	// The ID/name for the certificate chain in kubernetes tls secret.
	tlsScrtCert = "tls.crt"
	// The ID/name for the k8sKey in kubernetes tls secret.
	tlsScrtKey = "tls.key"
	// The ID/name for the CA certificate in kubernetes tls secret
	tlsScrtCaCert = "ca.crt"
	// The ID/name for the certificate chain in kubernetes generic secret.
	genericScrtCert = "cert"
	// The ID/name for the private key in kubernetes generic secret.
	genericScrtKey = "key"
	// The ID/name for the CA certificate in kubernetes generic secret.
	genericScrtCaCert = "cacert"
)

type IngressCredential struct {
	PrivateKey string
	ServerCert string
	CaCert     string
}

var IngressCredentialA = IngressCredential{
	PrivateKey: TLSServerKeyA,
	ServerCert: TLSServerCertA,
	CaCert:     CaCertA,
}

var IngressCredentialServerKeyCertA = IngressCredential{
	PrivateKey: TLSServerKeyA,
	ServerCert: TLSServerCertA,
}

var IngressCredentialCaCertA = IngressCredential{
	CaCert: CaCertA,
}

var IngressCredentialB = IngressCredential{
	PrivateKey: TLSServerKeyB,
	ServerCert: TLSServerCertB,
	CaCert:     CaCertB,
}

var IngressCredentialServerKeyCertB = IngressCredential{
	PrivateKey: TLSServerKeyB,
	ServerCert: TLSServerCertB,
}

// CreateIngressKubeSecret reads credential names from credNames and key/cert from ingressCred,
// and creates K8s secrets for ingress gateway.
// nolint: interfacer
func CreateIngressKubeSecret(ctx framework.TestContext, credNames []string,
	ingressType CallType, ingressCred IngressCredential, isCompoundAndNotGeneric bool) {
	ctx.Helper()
	// Get namespace for ingress gateway pod.
	istioCfg := istio.DefaultConfigOrFail(ctx, ctx)
	systemNS := namespace.ClaimOrFail(ctx, ctx, istioCfg.SystemNamespace)

	if len(credNames) == 0 {
		ctx.Log("no credential names are specified, skip creating ingress secret")
		return
	}
	// Create Kubernetes secret for ingress gateway
	cluster := ctx.Clusters().Default()
	for _, cn := range credNames {
		secret := createSecret(ingressType, cn, systemNS.Name(), ingressCred, isCompoundAndNotGeneric)
		_, err := cluster.CoreV1().Secrets(systemNS.Name()).Create(context.TODO(), secret, metav1.CreateOptions{})
		if err != nil {
			if errors.IsAlreadyExists(err) {
				if _, err := cluster.CoreV1().Secrets(systemNS.Name()).Update(context.TODO(), secret, metav1.UpdateOptions{}); err != nil {
					ctx.Fatalf("Failed to update secret (error: %s)", err)
				}
			} else {
				ctx.Fatalf("Failed to update secret (error: %s)", err)
			}
		}
	}
	// Check if Kubernetes secret is ready
	retry.UntilSuccessOrFail(ctx, func() error {
		for _, cn := range credNames {
			_, err := cluster.CoreV1().Secrets(systemNS.Name()).Get(context.TODO(), cn, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("secret %v not found: %v", cn, err)
			}
		}
		return nil
	}, retry.Timeout(time.Second*5))
}

// DeleteKubeSecret deletes a secret
// nolint: interfacer
func DeleteKubeSecret(ctx framework.TestContext, credNames []string) {
	// Get namespace for ingress gateway pod.
	istioCfg := istio.DefaultConfigOrFail(ctx, ctx)
	systemNS := namespace.ClaimOrFail(ctx, ctx, istioCfg.SystemNamespace)

	if len(credNames) == 0 {
		ctx.Log("no credential names are specified, skip creating ingress secret")
		return
	}
	// Create Kubernetes secret for ingress gateway
	cluster := ctx.Clusters().Default()
	for _, cn := range credNames {
		var immediate int64
		err := cluster.CoreV1().Secrets(systemNS.Name()).Delete(context.TODO(), cn,
			metav1.DeleteOptions{GracePeriodSeconds: &immediate})
		if err != nil {
			ctx.Fatalf("Failed to delete secret (error: %s)", err)
		}
	}
}

// createSecret creates a kubernetes secret which stores private key, server certificate for TLS ingress gateway.
// For mTLS ingress gateway, createSecret adds ca certificate into the secret object.

func createSecret(ingressType CallType, cn, ns string, ic IngressCredential, isCompoundAndNotGeneric bool) *v1.Secret {
	if ingressType == Mtls {
		if isCompoundAndNotGeneric {
			return &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cn,
					Namespace: ns,
				},
				Data: map[string][]byte{
					tlsScrtCert:   []byte(ic.ServerCert),
					tlsScrtKey:    []byte(ic.PrivateKey),
					tlsScrtCaCert: []byte(ic.CaCert),
				},
			}
		}
		return &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cn,
				Namespace: ns,
			},
			Data: map[string][]byte{
				genericScrtCert:   []byte(ic.ServerCert),
				genericScrtKey:    []byte(ic.PrivateKey),
				genericScrtCaCert: []byte(ic.CaCert),
			},
		}
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cn,
			Namespace: ns,
		},
		Data: map[string][]byte{
			tlsScrtCert: []byte(ic.ServerCert),
			tlsScrtKey:  []byte(ic.PrivateKey),
		},
	}
}

// CallType defines ingress gateway type
type CallType int

const (
	TLS CallType = iota
	Mtls
)

type ExpectedResponse struct {
	ResponseCode int
	ErrorMessage string
}

type TLSContext struct {
	// CaCert is inline base64 encoded root certificate that authenticates server certificate provided
	// by ingress gateway.
	CaCert string
	// PrivateKey is inline base64 encoded private key for test client.
	PrivateKey string
	// Cert is inline base64 encoded certificate for test client.
	Cert string
}

// SendRequestOrFail makes HTTPS request to ingress gateway to visit product page
func SendRequestOrFail(ctx framework.TestContext, ing ingress.Instance, host string, path string,
	callType CallType, tlsCtx TLSContext, exRsp ExpectedResponse) {
	ctx.Helper()
	opts := echo.CallOptions{
		Timeout: time.Second,
		Port: &echo.Port{
			Protocol: protocol.HTTPS,
		},
		Path: fmt.Sprintf("/%s", path),
		Headers: map[string][]string{
			"Host": {host},
		},
		CaCert: tlsCtx.CaCert,
		Validator: echo.And(
			echo.ValidatorFunc(
				func(resp client.ParsedResponses, err error) error {
					// Check that the error message is expected.
					if err != nil {
						if !strings.Contains(err.Error(), exRsp.ErrorMessage) {
							return fmt.Errorf("expected response error message %s but got %v",
								exRsp.ErrorMessage, err)
						}
						return nil
					}

					return resp.CheckCode(strconv.Itoa(exRsp.ResponseCode))
				})),
	}

	if callType == Mtls {
		opts.Key = tlsCtx.PrivateKey
		opts.Cert = tlsCtx.Cert
	}

	// Certs occasionally take quite a while to become active in Envoy, so retry for a long time (2min)
	ing.CallEchoWithRetryOrFail(ctx, opts, retry.Timeout(time.Minute*2))
}

// RotateSecrets deletes kubernetes secrets by name in credNames and creates same secrets using key/cert
// from ingressCred.
func RotateSecrets(ctx framework.TestContext, credNames []string, // nolint:interfacer
	ingressType CallType, ingressCred IngressCredential, isCompoundAndNotGeneric bool) {
	ctx.Helper()
	cluster := ctx.Clusters().Default()
	ist := istio.GetOrFail(ctx, ctx)
	systemNS := namespace.ClaimOrFail(ctx, ctx, ist.Settings().SystemNamespace)
	for _, cn := range credNames {
		scrt, err := cluster.CoreV1().Secrets(systemNS.Name()).Get(context.TODO(), cn, metav1.GetOptions{})
		if err != nil {
			ctx.Errorf("Failed to get secret %s:%s (error: %s)", systemNS.Name(), cn, err)
			continue
		}
		scrt = updateSecret(ingressType, scrt, ingressCred, isCompoundAndNotGeneric)
		if _, err = cluster.CoreV1().Secrets(systemNS.Name()).Update(context.TODO(), scrt, metav1.UpdateOptions{}); err != nil {
			ctx.Errorf("Failed to update secret %s:%s (error: %s)", scrt.Namespace, scrt.Name, err)
		}
	}
	// Check if Kubernetes secret is ready
	retry.UntilSuccessOrFail(ctx, func() error {
		for _, cn := range credNames {
			_, err := cluster.CoreV1().Secrets(systemNS.Name()).Get(context.TODO(), cn, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("secret %v not found: %v", cn, err)
			}
		}
		return nil
	}, retry.Timeout(time.Second*5))
}

// createSecret creates a kubernetes secret which stores private key, server certificate for TLS ingress gateway.
// For mTLS ingress gateway, createSecret adds ca certificate into the secret object.
func updateSecret(ingressType CallType, scrt *v1.Secret, ic IngressCredential, isCompoundAndNotGeneric bool) *v1.Secret {
	if ingressType == Mtls {
		if isCompoundAndNotGeneric {
			scrt.Data[tlsScrtCert] = []byte(ic.ServerCert)
			scrt.Data[tlsScrtKey] = []byte(ic.PrivateKey)
			scrt.Data[tlsScrtCaCert] = []byte(ic.CaCert)
		} else {
			scrt.Data[genericScrtCert] = []byte(ic.ServerCert)
			scrt.Data[genericScrtKey] = []byte(ic.PrivateKey)
			scrt.Data[genericScrtCaCert] = []byte(ic.CaCert)
		}
	} else {
		scrt.Data[tlsScrtCert] = []byte(ic.ServerCert)
		scrt.Data[tlsScrtKey] = []byte(ic.PrivateKey)
	}
	return scrt
}

func SetupTest(ctx resource.Context) (namespace.Instance, error) {
	serverNs, err := namespace.New(ctx, namespace.Config{
		Prefix: "ingress",
		Inject: true,
	})
	if err != nil {
		return nil, err
	}
	var a echo.Instance
	if _, err := echoboot.NewBuilder(ctx).
		With(&a, echo.Config{
			Service:   "server",
			Namespace: serverNs,
			Ports: []echo.Port{
				{
					Name:     "http",
					Protocol: protocol.HTTP,
					// We use a port > 1024 to not require root
					InstancePort: 8090,
				},
			},
		}).Build(); err != nil {
		return nil, err
	}
	return serverNs, nil
}

type TestConfig struct {
	Mode           string
	CredentialName string
	Host           string
}

const vsTemplate = `
apiVersion: networking.istio.io/v1alpha3
kind: VirtualService
metadata:
  name: {{.CredentialName}}
spec:
  hosts:
  - "{{.Host}}"
  gateways:
  - {{.CredentialName}}
  http:
  - match:
    - uri:
        exact: /{{.CredentialName}}
    route:
    - destination:
        host: server
        port:
          number: 80
`

const gwTemplate = `
apiVersion: networking.istio.io/v1alpha3
kind: Gateway
metadata:
  name: {{.CredentialName}}
spec:
  selector:
    istio: ingressgateway # use istio default ingress gateway
  servers:
  - port:
      number: 443
      name: https
      protocol: HTTPS
    tls:
      mode: {{.Mode}}
      credentialName: "{{.CredentialName}}"
    hosts:
    - "{{.Host}}"
`

func runTemplate(t test.Failer, tmpl string, params interface{}) string {
	tm, err := template.New("").Parse(tmpl)
	if err != nil {
		t.Fatalf("failed to render template: %v", err)
	}

	var buf bytes.Buffer
	if err := tm.Execute(&buf, params); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func SetupConfig(ctx framework.TestContext, ns namespace.Instance, config ...TestConfig) func() {
	var apply []string
	for _, c := range config {
		apply = append(apply, runTemplate(ctx, vsTemplate, c), runTemplate(ctx, gwTemplate, c))
	}
	ctx.Config().ApplyYAMLOrFail(ctx, ns.Name(), apply...)
	return func() {
		ctx.Config().DeleteYAMLOrFail(ctx, ns.Name(), apply...)
	}
}

// RunTestMultiMtlsGateways deploys multiple mTLS gateways with SDS enabled, and creates kubernetes that store
// private key, server certificate and CA certificate for each mTLS gateway. Verifies that all gateways are able to terminate
// mTLS connections successfully.
func RunTestMultiMtlsGateways(ctx framework.TestContext, inst istio.Instance, ns namespace.Instance) { // nolint:interfacer
	var credNames []string
	var tests []TestConfig
	for i := 1; i < 6; i++ {
		cred := fmt.Sprintf("runtestmultimtlsgateways-%d", i)
		tests = append(tests, TestConfig{
			Mode:           "MUTUAL",
			CredentialName: cred,
			Host:           fmt.Sprintf("runtestmultimtlsgateways%d.example.com", i),
		})
		credNames = append(credNames, cred)
	}
	CreateIngressKubeSecret(ctx, credNames, Mtls, IngressCredentialA, false)
	defer DeleteKubeSecret(ctx, credNames)
	ctx.Cleanup(SetupConfig(ctx, ns, tests...))
	ing := inst.IngressFor(ctx.Clusters().Default())
	tlsContext := TLSContext{
		CaCert:     CaCertA,
		PrivateKey: TLSClientKeyA,
		Cert:       TLSClientCertA,
	}
	callType := Mtls

	for _, h := range tests {
		ctx.NewSubTest(h.Host).Run(func(ctx framework.TestContext) {
			SendRequestOrFail(ctx, ing, h.Host, h.CredentialName, callType, tlsContext,
				ExpectedResponse{ResponseCode: 200, ErrorMessage: ""})
		})
	}
}

// RunTestMultiTLSGateways deploys multiple TLS gateways with SDS enabled, and creates kubernetes that store
// private key and server certificate for each TLS gateway. Verifies that all gateways are able to terminate
// SSL connections successfully.
func RunTestMultiTLSGateways(ctx framework.TestContext, inst istio.Instance, ns namespace.Instance) { // nolint:interfacer
	var credNames []string
	var tests []TestConfig
	for i := 1; i < 6; i++ {
		cred := fmt.Sprintf("runtestmultitlsgateways-%d", i)
		tests = append(tests, TestConfig{
			Mode:           "SIMPLE",
			CredentialName: cred,
			Host:           fmt.Sprintf("runtestmultitlsgateways%d.example.com", i),
		})
		credNames = append(credNames, cred)
	}
	CreateIngressKubeSecret(ctx, credNames, Mtls, IngressCredentialA, false)
	defer DeleteKubeSecret(ctx, credNames)
	SetupConfig(ctx, ns, tests...)
	ing := inst.IngressFor(ctx.Clusters().Default())
	tlsContext := TLSContext{
		CaCert: CaCertA,
	}
	callType := TLS

	for _, h := range tests {
		ctx.NewSubTest(h.Host).Run(func(ctx framework.TestContext) {
			SendRequestOrFail(ctx, ing, h.Host, h.CredentialName, callType, tlsContext,
				ExpectedResponse{ResponseCode: 200, ErrorMessage: ""})
		})
	}
}
