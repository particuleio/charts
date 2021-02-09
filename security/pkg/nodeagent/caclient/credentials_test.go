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

package caclient

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"istio.io/istio/pilot/cmd/pilot-agent/config"
	secop "istio.io/istio/pilot/cmd/pilot-agent/security"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/jwt"
	"istio.io/istio/pkg/security"
	"istio.io/istio/security/pkg/stsservice"
	stsmock "istio.io/istio/security/pkg/stsservice/mock"
	"istio.io/istio/security/pkg/stsservice/tokenmanager/google"
	"istio.io/istio/security/pkg/stsservice/tokenmanager/google/mock"
)

// TestGetTokenForXDS tests getting token for XDS.
// Test case 1: xdsAuthProvider is google.GCPAuthProvider.
// Test case 2: xdsAuthProvider is empty.
func TestGetTokenForXDS(t *testing.T) {
	role := &model.Proxy{}
	role.Type = model.SidecarProxy
	meshConfigFile := ""
	serviceCluster := constants.ServiceClusterName
	proxyConfigEnv := ""
	concurrency := 0
	proxyConfig, err := config.ConstructProxyConfig(meshConfigFile, serviceCluster, proxyConfigEnv, concurrency, role)
	if err != nil {
		t.Fatalf("failed to construct proxy config: %v", err)
	}

	jwtPolicy := jwt.PolicyThirdParty
	credFetcherTypeEnv := ""
	credIdentityProvider := google.GCEProvider
	sop := security.Options{
		CAEndpoint:                     "",
		CAProviderName:                 "Citadel",
		PilotCertProvider:              "istiod",
		OutputKeyCertToDir:             "",
		ProvCert:                       "",
		WorkloadUDSPath:                security.DefaultLocalSDSPath,
		ClusterID:                      "",
		FileMountedCerts:               false,
		WorkloadNamespace:              "",
		ServiceAccount:                 "",
		TrustDomain:                    "cluster.local",
		Pkcs8Keys:                      false,
		ECCSigAlg:                      "",
		SecretTTL:                      24 * time.Hour,
		SecretRotationGracePeriodRatio: 0.5,
	}
	secOpts, err := secop.SetupSecurityOptions(proxyConfig, sop, jwtPolicy,
		credFetcherTypeEnv, credIdentityProvider)
	if err != nil {
		t.Fatalf("failed to setup security options: %v", err)
	}
	jwtPath, err := writeToTempFile(mock.FakeSubjectToken, "jwt-token-*")
	if err != nil {
		t.Fatalf("failed to write the JWT token file: %v", err)
	}
	secOpts.JWTPath = jwtPath
	defer os.Remove(jwtPath)
	// Use a mock token manager because real token exchange requires a working k8s token,
	// permissions for token exchange, and connection to the token exchange server.
	tokenManager := stsmock.CreateFakeTokenManager()
	tokenManager.SetRespStsParam(stsservice.StsResponseParameters{
		AccessToken:     mock.FakeAccessToken,
		IssuedTokenType: "urn:ietf:params:oauth:token-type:access_token",
		TokenType:       "Bearer",
		ExpiresIn:       60,
		Scope:           "example.com",
	})
	secOpts.TokenManager = tokenManager

	tests := []struct {
		name        string
		provider    string
		expectToken string
	}{
		{
			name:        "xdsAuthProvider is google.GCPAuthProvider",
			provider:    google.GCPAuthProvider,
			expectToken: mock.FakeAccessToken,
		},
		{
			name:        "xdsAuthProvider is empty",
			provider:    "",
			expectToken: mock.FakeSubjectToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secOpts.XdsAuthProvider = tt.provider
			provider := NewXDSTokenProvider(secOpts)
			token, err := provider.GetToken()
			if err != nil {
				t.Errorf("failed to get token: %v", err)
			}
			if token != tt.expectToken {
				t.Errorf("the token returned is unexpected, expect: %v, got: %v", tt.expectToken, token)
			}
		})
	}
}

func writeToTempFile(content, fileNamePrefix string) (string, error) {
	outFile, err := ioutil.TempFile("", fileNamePrefix)
	if err != nil {
		return "", fmt.Errorf("failed creating a temp file: %v", err)
	}
	defer func() { _ = outFile.Close() }()

	if _, err := outFile.Write([]byte(content)); err != nil {
		return "", fmt.Errorf("failed writing to the temp file: %v", err)
	}
	return outFile.Name(), nil
}
