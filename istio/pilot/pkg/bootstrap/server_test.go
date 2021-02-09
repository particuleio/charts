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
package bootstrap

import (
	"bytes"
	"crypto/tls"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"istio.io/istio/pilot/pkg/features"
	"istio.io/istio/pilot/pkg/serviceregistry"
	kubecontroller "istio.io/istio/pilot/pkg/serviceregistry/kube/controller"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/testcerts"
	"istio.io/pkg/filewatcher"
)

func TestNewServerWithExternalCertificates(t *testing.T) {
	configDir, err := ioutil.TempDir("", "test_istiod_config")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(configDir)
	}()

	certsDir, err := ioutil.TempDir("", "test_istiod_certs")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(certsDir)
	}()

	certFile := filepath.Join(certsDir, "cert-file.pem")
	keyFile := filepath.Join(certsDir, "key-file.pem")
	caCertFile := filepath.Join(certsDir, "ca-cert.pem")

	// load key and cert files.
	if err := ioutil.WriteFile(certFile, testcerts.ServerCert, 0644); err != nil { // nolint: vetshadow
		t.Fatalf("WriteFile(%v) failed: %v", certFile, err)
	}
	if err := ioutil.WriteFile(keyFile, testcerts.ServerKey, 0644); err != nil { // nolint: vetshadow
		t.Fatalf("WriteFile(%v) failed: %v", keyFile, err)
	}
	if err := ioutil.WriteFile(caCertFile, testcerts.CACert, 0644); err != nil { // nolint: vetshadow
		t.Fatalf("WriteFile(%v) failed: %v", caCertFile, err)
	}

	tlsOptions := TLSOptions{
		CertFile:   certFile,
		KeyFile:    keyFile,
		CaCertFile: caCertFile,
	}

	args := NewPilotArgs(func(p *PilotArgs) {
		p.Namespace = "istio-system"
		p.ServerOptions = DiscoveryServerOptions{
			// Dynamically assign all ports.
			HTTPAddr:       ":0",
			MonitoringAddr: ":0",
			GRPCAddr:       ":0",
			SecureGRPCAddr: ":0",
			TLSOptions:     tlsOptions,
		}
		p.RegistryOptions = RegistryOptions{
			FileDir: configDir,
		}

		// Include all of the default plugins
		p.Plugins = DefaultPlugins
		p.ShutdownDuration = 1 * time.Millisecond
	})

	g := NewWithT(t)
	s, err := NewServer(args)
	g.Expect(err).To(Succeed())

	stop := make(chan struct{})
	features.EnableCAServer = false
	g.Expect(s.Start(stop)).To(Succeed())
	defer func() {
		close(stop)
		s.WaitUntilCompletion()
	}()

	// Validate server started with the provided cert
	checkCert(t, s, testcerts.ServerCert, testcerts.ServerKey)
}

func TestReloadIstiodCert(t *testing.T) {
	dir, err := ioutil.TempDir("", "istiod_certs")
	stop := make(chan struct{})
	s := &Server{
		fileWatcher: filewatcher.NewWatcher(),
	}

	defer func() {
		close(stop)
		_ = s.fileWatcher.Close()
		_ = os.RemoveAll(dir)
	}()
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}

	certFile := filepath.Join(dir, "cert-file.yaml")
	keyFile := filepath.Join(dir, "key-file.yaml")

	// load key and cert files.
	if err := ioutil.WriteFile(certFile, testcerts.ServerCert, 0644); err != nil { // nolint: vetshadow
		t.Fatalf("WriteFile(%v) failed: %v", certFile, err)
	}
	if err := ioutil.WriteFile(keyFile, testcerts.ServerKey, 0644); err != nil { // nolint: vetshadow
		t.Fatalf("WriteFile(%v) failed: %v", keyFile, err)
	}

	tlsOptions := TLSOptions{
		CertFile: certFile,
		KeyFile:  keyFile,
	}

	// setup cert watches.
	err = s.initCertificateWatches(tlsOptions)
	for _, fn := range s.startFuncs {
		if err := fn(stop); err != nil {
			t.Fatalf("Could not invoke startFuncs: %v", err)
		}
	}

	if err != nil {
		t.Fatalf("initCertificateWatches failed: %v", err)
	}

	// Validate that the certs are loaded.
	checkCert(t, s, testcerts.ServerCert, testcerts.ServerKey)

	// Update cert/key files.
	if err := ioutil.WriteFile(tlsOptions.CertFile, testcerts.RotatedCert, 0644); err != nil { // nolint: vetshadow
		t.Fatalf("WriteFile(%v) failed: %v", tlsOptions.CertFile, err)
	}
	if err := ioutil.WriteFile(tlsOptions.KeyFile, testcerts.RotatedKey, 0644); err != nil { // nolint: vetshadow
		t.Fatalf("WriteFile(%v) failed: %v", tlsOptions.KeyFile, err)
	}

	g := NewWithT(t)

	// Validate that istiod cert is updated.
	g.Eventually(func() bool {
		return checkCert(t, s, testcerts.RotatedCert, testcerts.RotatedKey)
	}, "10s", "100ms").Should(BeTrue())
}

func TestNewServer(t *testing.T) {
	// All of the settings to apply and verify. Currently just testing domain suffix,
	// but we should expand this list.
	cases := []struct {
		name           string
		domain         string
		expectedDomain string
		secureGRPCport string
		jwtRule        string
	}{
		{
			name:           "default domain",
			domain:         "",
			expectedDomain: constants.DefaultKubernetesDomain,
		},
		{
			name:           "default domain with JwtRule",
			domain:         "",
			expectedDomain: constants.DefaultKubernetesDomain,
			jwtRule:        `{"issuer": "foo", "jwks_uri": "baz", "audiences": ["aud1", "aud2"]}`,
		},
		{
			name:           "override domain",
			domain:         "mydomain.com",
			expectedDomain: "mydomain.com",
		},
		{
			name:           "override default secured grpc port",
			domain:         "",
			expectedDomain: constants.DefaultKubernetesDomain,
			secureGRPCport: ":31128",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			configDir, err := ioutil.TempDir("", "TestNewServer")
			if err != nil {
				t.Fatal(err)
			}

			defer func() {
				_ = os.RemoveAll(configDir)
			}()

			args := NewPilotArgs(func(p *PilotArgs) {
				p.Namespace = "istio-system"
				p.ServerOptions = DiscoveryServerOptions{
					// Dynamically assign all ports.
					HTTPAddr:       ":0",
					MonitoringAddr: ":0",
					GRPCAddr:       ":0",
					SecureGRPCAddr: c.secureGRPCport,
				}
				p.RegistryOptions = RegistryOptions{
					KubeOptions: kubecontroller.Options{
						DomainSuffix: c.domain,
					},
					FileDir: configDir,
				}

				// Include all of the default plugins
				p.Plugins = DefaultPlugins
				p.ShutdownDuration = 1 * time.Millisecond

				p.JwtRule = c.jwtRule
			})

			g := NewWithT(t)
			s, err := NewServer(args)
			g.Expect(err).To(Succeed())

			stop := make(chan struct{})
			g.Expect(s.Start(stop)).To(Succeed())
			defer func() {
				close(stop)
				s.WaitUntilCompletion()
			}()

			g.Expect(s.environment.GetDomainSuffix()).To(Equal(c.expectedDomain))
		})
	}
}

func TestNewServerWithMockRegistry(t *testing.T) {
	cases := []struct {
		name             string
		registry         string
		expectedRegistry serviceregistry.ProviderID
		secureGRPCport   string
	}{
		{
			name:             "Mock Registry",
			registry:         "Mock",
			expectedRegistry: serviceregistry.Mock,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			configDir, err := ioutil.TempDir("", "TestNewServer")
			if err != nil {
				t.Fatal(err)
			}

			defer func() {
				_ = os.RemoveAll(configDir)
			}()

			args := NewPilotArgs(func(p *PilotArgs) {
				p.Namespace = "istio-system"

				// As the same with args in main go of pilot-discovery
				p.InjectionOptions = InjectionOptions{
					InjectionDirectory: "./var/lib/istio/inject",
				}

				p.ServerOptions = DiscoveryServerOptions{
					// Dynamically assign all ports.
					HTTPAddr:       ":0",
					MonitoringAddr: ":0",
					GRPCAddr:       ":0",
					SecureGRPCAddr: c.secureGRPCport,
				}

				p.RegistryOptions = RegistryOptions{
					Registries: []string{c.registry},
					FileDir:    configDir,
				}

				// Include all of the default plugins
				p.Plugins = DefaultPlugins
				p.ShutdownDuration = 1 * time.Millisecond
			})

			g := NewWithT(t)
			s, err := NewServer(args)
			g.Expect(err).To(Succeed())

			stop := make(chan struct{})
			g.Expect(s.Start(stop)).To(Succeed())
			defer func() {
				close(stop)
				s.WaitUntilCompletion()
			}()

			g.Expect(s.ServiceController().GetRegistries()[1].Provider()).To(Equal(c.expectedRegistry))
		})
	}
}

func TestInitOIDC(t *testing.T) {
	tests := []struct {
		name      string
		expectErr bool
		jwtRule   string
	}{
		{
			name:      "valid jwt rule",
			expectErr: false,
			jwtRule:   `{"issuer": "foo", "jwks_uri": "baz", "audiences": ["aud1", "aud2"]}`,
		},
		{
			name:      "invalid jwt rule",
			expectErr: true,
			jwtRule:   "invalid",
		},
		{
			name:      "jwt rule with invalid audiences",
			expectErr: true,
			// audiences must be a string array
			jwtRule: `{"issuer": "foo", "jwks_uri": "baz", "audiences": "aud1"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := &PilotArgs{JwtRule: tt.jwtRule}

			_, err := initOIDC(args, "domain-foo")
			gotErr := err != nil
			if gotErr != tt.expectErr {
				t.Errorf("expect error is %v while actual error is %v", tt.expectErr, gotErr)
			}
		})
	}
}

func checkCert(t *testing.T, s *Server, cert, key []byte) bool {
	t.Helper()
	actual, _ := s.getIstiodCertificate(nil)
	expected, err := tls.X509KeyPair(cert, key)
	if err != nil {
		t.Fatalf("fail to load test certs.")
	}
	return bytes.Equal(actual.Certificate[0], expected.Certificate[0])
}
