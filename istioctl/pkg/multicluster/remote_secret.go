// Copyright Istio Authors.
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

package multicluster

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/versioning"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	//  to avoid 'No Auth Provider found for name "gcp"'
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/clientcmd/api/latest"

	"istio.io/istio/operator/cmd/mesh"
	"istio.io/istio/operator/pkg/helm"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/labels"
	"istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/kube/secretcontroller"
)

var (
	codec  runtime.Codec
	scheme *runtime.Scheme
)

func init() {
	scheme = runtime.NewScheme()
	utilruntime.Must(v1.AddToScheme(scheme))
	opt := json.SerializerOptions{
		Yaml:   true,
		Pretty: false,
		Strict: false,
	}
	yamlSerializer := json.NewSerializerWithOptions(json.DefaultMetaFactory, scheme, scheme, opt)
	codec = versioning.NewDefaultingCodecForScheme(
		scheme,
		yamlSerializer,
		yamlSerializer,
		v1.SchemeGroupVersion,
		runtime.InternalGroupVersioner,
	)
}

const (
	remoteSecretPrefix = "istio-remote-secret-"
	configSecretName   = "istio-kubeconfig"
	configSecretKey    = "config"
)

func remoteSecretNameFromClusterName(clusterName string) string {
	return remoteSecretPrefix + clusterName
}

// NewCreateRemoteSecretCommand creates a new command for joining two contexts
// together in a multi-cluster mesh.
func NewCreateRemoteSecretCommand() *cobra.Command {
	opts := RemoteSecretOptions{
		AuthType:         RemoteSecretAuthTypeBearerToken,
		AuthPluginConfig: make(map[string]string),
		Type:             SecretTypeRemote,
	}
	c := &cobra.Command{
		Use:   "create-remote-secret",
		Short: "Create a secret with credentials to allow Istio to access remote Kubernetes apiservers",
		Example: `  # Create a secret to access cluster c0's apiserver and install it in cluster c1.
  istioctl --kubeconfig=c0.yaml x create-remote-secret --name c0 \
    | kubectl --kubeconfig=c1.yaml apply -f -

  # Delete a secret that was previously installed in c1
  istioctl --kubeconfig=c0.yaml x create-remote-secret --name c0 \
    | kubectl --kubeconfig=c1.yaml delete -f -

  # Create a secret access a remote cluster with an auth plugin
  istioctl --kubeconfig=c0.yaml x create-remote-secret --name c0 --auth-type=plugin --auth-plugin-name=gcp \
    | kubectl --kubeconfig=c1.yaml apply -f -`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if err := opts.prepare(c.Flags()); err != nil {
				return err
			}
			env, err := NewEnvironmentFromCobra(opts.Kubeconfig, opts.Context, c)
			if err != nil {
				return err
			}
			out, err := CreateRemoteSecret(opts, env)
			if err != nil {
				_, _ = fmt.Fprintf(c.OutOrStderr(), "error: %v\n", err)
				return err
			}
			_, _ = fmt.Fprint(c.OutOrStdout(), out)
			return nil
		},
	}
	opts.addFlags(c.PersistentFlags())
	return c
}

func createRemoteServiceAccountSecret(kubeconfig *api.Config, clusterName, secName string) (*v1.Secret, error) { // nolint:interfacer
	var data bytes.Buffer
	if err := latest.Codec.Encode(kubeconfig, &data); err != nil {
		return nil, err
	}
	key := clusterName
	if secName == configSecretName {
		key = configSecretKey
	}
	out := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secName,
			Annotations: map[string]string{
				clusterNameAnnotationKey: clusterName,
			},
			Labels: map[string]string{
				secretcontroller.MultiClusterSecretLabel: "true",
			},
		},
		Data: map[string][]byte{
			key: data.Bytes(),
		},
	}
	return out, nil
}

func createBaseKubeconfig(caData []byte, clusterName, server string) *api.Config {
	return &api.Config{
		Clusters: map[string]*api.Cluster{
			clusterName: {
				CertificateAuthorityData: caData,
				Server:                   server,
			},
		},
		AuthInfos: map[string]*api.AuthInfo{},
		Contexts: map[string]*api.Context{
			clusterName: {
				Cluster:  clusterName,
				AuthInfo: clusterName,
			},
		},
		CurrentContext: clusterName,
	}
}

func createBearerTokenKubeconfig(caData, token []byte, clusterName, server string) *api.Config {
	c := createBaseKubeconfig(caData, clusterName, server)
	c.AuthInfos[c.CurrentContext] = &api.AuthInfo{
		Token: string(token),
	}
	return c
}

func createPluginKubeconfig(caData []byte, clusterName, server string, authProviderConfig *api.AuthProviderConfig) *api.Config {
	c := createBaseKubeconfig(caData, clusterName, server)
	c.AuthInfos[c.CurrentContext] = &api.AuthInfo{
		AuthProvider: authProviderConfig,
	}
	return c
}

func createRemoteSecretFromPlugin(
	tokenSecret *v1.Secret,
	server, clusterName, secName string,
	authProviderConfig *api.AuthProviderConfig,
) (*v1.Secret, error) {
	caData, ok := tokenSecret.Data[v1.ServiceAccountRootCAKey]
	if !ok {
		return nil, errMissingRootCAKey
	}

	// Create a Kubeconfig to access the remote cluster using the auth provider plugin.
	kubeconfig := createPluginKubeconfig(caData, clusterName, server, authProviderConfig)
	if err := clientcmd.Validate(*kubeconfig); err != nil {
		return nil, fmt.Errorf("invalid kubeconfig: %v", err)
	}

	// Encode the Kubeconfig in a secret that can be loaded by Istio to dynamically discover and access the remote cluster.
	return createRemoteServiceAccountSecret(kubeconfig, clusterName, secName)
}

var (
	errMissingRootCAKey = fmt.Errorf("no %q data found", v1.ServiceAccountRootCAKey)
	errMissingTokenKey  = fmt.Errorf("no %q data found", v1.ServiceAccountTokenKey)
)

func createRemoteSecretFromTokenAndServer(tokenSecret *v1.Secret, clusterName, server, secName string) (*v1.Secret, error) {
	caData, ok := tokenSecret.Data[v1.ServiceAccountRootCAKey]
	if !ok {
		return nil, errMissingRootCAKey
	}
	token, ok := tokenSecret.Data[v1.ServiceAccountTokenKey]
	if !ok {
		return nil, errMissingTokenKey
	}

	// Create a Kubeconfig to access the remote cluster using the remote service account credentials.
	kubeconfig := createBearerTokenKubeconfig(caData, token, clusterName, server)
	if err := clientcmd.Validate(*kubeconfig); err != nil {
		return nil, fmt.Errorf("invalid kubeconfig: %v", err)
	}

	// Encode the Kubeconfig in a secret that can be loaded by Istio to dynamically discover and access the remote cluster.
	return createRemoteServiceAccountSecret(kubeconfig, clusterName, secName)
}

func getServiceAccountSecret(client kube.ExtendedClient, opt RemoteSecretOptions) (*v1.Secret, error) {
	// Create the service account if it doesn't exist.
	serviceAccount, err := getOrCreateServiceAccount(client, opt)
	if err != nil {
		return nil, err
	}

	if len(serviceAccount.Secrets) == 0 {
		return nil, fmt.Errorf("no secret found in the service account: %s", serviceAccount)
	}

	if len(serviceAccount.Secrets) != 1 && opt.SecretName == "" {
		return nil, fmt.Errorf("wrong number of secrets (%v) in serviceaccount %s/%s, please use --secret-name to specify one",
			len(serviceAccount.Secrets), opt.Namespace, opt.ServiceAccountName)
	}

	secretName := serviceAccount.Secrets[0].Name
	secretNamespace := serviceAccount.Secrets[0].Namespace
	for _, secret := range serviceAccount.Secrets {
		if secret.Name == opt.SecretName {
			secretName = secret.Name
			secretNamespace = secret.Namespace
			break
		}
	}

	if secretNamespace == "" {
		secretNamespace = opt.Namespace
	}
	return client.CoreV1().Secrets(secretNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})
}

func getOrCreateServiceAccount(client kube.ExtendedClient, opt RemoteSecretOptions) (*v1.ServiceAccount, error) {
	if sa, err := client.CoreV1().ServiceAccounts(opt.Namespace).Get(
		context.TODO(), opt.ServiceAccountName, metav1.GetOptions{}); err == nil {
		return sa, nil
	} else if !opt.CreateServiceAccount {
		// User chose not to automatically create the service account.
		return nil, fmt.Errorf("failed retrieving service account %s.%s required for creating "+
			"the remote secret (hint: try installing a minimal Istio profile on the cluster first, "+
			"or run with '--create-service-account=true'): %v",
			opt.ServiceAccountName,
			opt.Namespace,
			err)
	}

	if err := createServiceAccount(client, opt); err != nil {
		return nil, err
	}

	// Return the newly created service account.
	sa, err := client.CoreV1().ServiceAccounts(opt.Namespace).Get(
		context.TODO(), opt.ServiceAccountName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed retrieving service account %s.%s after creating it: %v",
			opt.ServiceAccountName, opt.Namespace, err)
	}
	return sa, nil
}

func createServiceAccount(client kube.ExtendedClient, opt RemoteSecretOptions) error {
	// Create a renderer for the base installation.
	r, err := helm.NewHelmRenderer(opt.ManifestsPath, "base", "Base", opt.Namespace)
	if err != nil {
		return fmt.Errorf("failed creating Helm renderer: %v", err)
	}

	if err := r.Run(); err != nil {
		return fmt.Errorf("failed running Helm renderer: %v", err)
	}

	values := fmt.Sprintf(`
global:
  istioNamespace: %s
  istiod:
    enableAnalysis: false
  configValidation: false
base:
  enableCRDTemplates: false
`, opt.Namespace)

	yamlContent, err := r.RenderManifest(values)
	if err != nil {
		return fmt.Errorf("failed rendering base manifest: %v", err)
	}

	// Before we can apply the yaml, we have to ensure the system namespace exists.
	if err := createNamespaceIfNotExist(client, opt.Namespace); err != nil {
		return err
	}

	// Apply the YAML to the cluster.
	return applyYAML(client, yamlContent, opt.Namespace)
}

func applyYAML(client kube.ExtendedClient, yamlContent, ns string) error {
	yamlFile, err := writeToTempFile(yamlContent)
	if err != nil {
		return fmt.Errorf("failed creating manifest file: %v", err)
	}

	// Apply the YAML to the cluster.
	if err := client.ApplyYAMLFiles(ns, yamlFile); err != nil {
		return fmt.Errorf("failed applying manifest %s: %v", yamlFile, err)
	}
	return nil
}

func createNamespaceIfNotExist(client kube.Client, ns string) error {
	if _, err := client.CoreV1().Namespaces().Get(context.TODO(), ns, metav1.GetOptions{}); err != nil {
		if _, err := client.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: ns,
			},
		}, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed creating namespace %s: %v", ns, err)
		}
	}
	return nil
}

func writeToTempFile(content string) (string, error) {
	outFile, err := ioutil.TempFile("", "remote-secret-manifest-*")
	if err != nil {
		return "", fmt.Errorf("failed creating temp file for manifest: %v", err)
	}
	defer func() { _ = outFile.Close() }()

	if _, err := outFile.Write([]byte(content)); err != nil {
		return "", fmt.Errorf("failed writing manifest file: %v", err)
	}
	return outFile.Name(), nil
}

func getServerFromKubeconfig(context string, config *api.Config) (string, error) {
	if context == "" {
		context = config.CurrentContext
	}

	configContext, ok := config.Contexts[context]
	if !ok {
		return "", fmt.Errorf("could not find cluster for context %q", context)
	}
	cluster, ok := config.Clusters[configContext.Cluster]
	if !ok {
		return "", fmt.Errorf("could not find server for context %q", context)
	}
	return cluster.Server, nil
}

const (
	outputHeader  = "# This file is autogenerated, do not edit.\n"
	outputTrailer = "---\n"
)

func writeEncodedObject(out io.Writer, in runtime.Object) error {
	if _, err := fmt.Fprint(out, outputHeader); err != nil {
		return err
	}
	if err := codec.Encode(in, out); err != nil {
		return err
	}
	if _, err := fmt.Fprint(out, outputTrailer); err != nil {
		return err
	}
	return nil
}

type writer interface {
	io.Writer
	String() string
}

func makeOutputWriter() writer {
	return &bytes.Buffer{}
}

var makeOutputWriterTestHook = makeOutputWriter

// RemoteSecretAuthType is a strongly typed authentication type suitable for use with pflags.Var().
type (
	RemoteSecretAuthType string
	SecretType           string
)

var _ pflag.Value = (*RemoteSecretAuthType)(nil)

func (at *RemoteSecretAuthType) String() string { return string(*at) }
func (at *RemoteSecretAuthType) Type() string   { return "RemoteSecretAuthType" }
func (at *RemoteSecretAuthType) Set(in string) error {
	*at = RemoteSecretAuthType(in)
	return nil
}

func (at *SecretType) String() string { return string(*at) }
func (at *SecretType) Type() string   { return "SecretType" }
func (at *SecretType) Set(in string) error {
	*at = SecretType(in)
	return nil
}

const (
	// Use a bearer token for authentication to the remote kubernetes cluster.
	RemoteSecretAuthTypeBearerToken RemoteSecretAuthType = "bearer-token"

	// User a custom custom authentication plugin for the remote kubernetes cluster.
	RemoteSecretAuthTypePlugin RemoteSecretAuthType = "plugin"

	// Secret generated from remote cluster
	SecretTypeRemote SecretType = "remote"

	// Secret generated from config cluster
	SecretTypeConfig SecretType = "config"
)

// RemoteSecretOptions contains the options for creating a remote secret.
type RemoteSecretOptions struct {
	KubeOptions

	// Name of the local cluster whose credentials are stored in the secret. Must be
	// DNS1123 label as it will be used for the k8s secret name.
	ClusterName string

	// Create a secret with this service account's credentials.
	ServiceAccountName string

	// CreateServiceAccount if true, the service account specified by ServiceAccountName
	// will be created if it doesn't exist.
	CreateServiceAccount bool

	// Authentication method for the remote Kubernetes cluster.
	AuthType RemoteSecretAuthType
	// Authenticator plugin configuration
	AuthPluginName   string
	AuthPluginConfig map[string]string

	// Type of the generated secret
	Type SecretType

	// ManifestsPath is a path to a manifestsPath and profiles directory in the local filesystem,
	// or URL with a release tgz. This is only used when no reader service account exists and has
	// to be created.
	ManifestsPath string

	// ServerOverride overrides the server IP/hostname field from the Kubeconfig
	ServerOverride string

	// SecretName selects a specific secret from the remote service account, if there are multiple
	SecretName string
}

func (o *RemoteSecretOptions) addFlags(flagset *pflag.FlagSet) {
	flagset.StringVar(&o.ServiceAccountName, "service-account", "",
		"Create a secret with this service account's credentials. Use \""+
			constants.DefaultServiceAccountName+"\" as default value if --type is \"remote\", use \""+
			constants.DefaultConfigServiceAccountName+"\" as default value if --type is \"config\".")
	flagset.BoolVar(&o.CreateServiceAccount, "create-service-account", true,
		"If true, the service account needed for creating the remote secret will be created "+
			"if it doesn't exist.")
	flagset.StringVar(&o.ClusterName, "name", "",
		"Name of the local cluster whose credentials are stored "+
			"in the secret. If a name is not specified the kube-system namespace's UUID of "+
			"the local cluster will be used.")
	flagset.StringVar(&o.ServerOverride, "server", "",
		"The address and port of the Kubernetes API server.")
	flagset.StringVar(&o.SecretName, "secret-name", "",
		"The name of the specific secret to use from the service-account. Needed when there are multiple secrets in the service account.")
	var supportedAuthType []string
	for _, at := range []RemoteSecretAuthType{RemoteSecretAuthTypeBearerToken, RemoteSecretAuthTypePlugin} {
		supportedAuthType = append(supportedAuthType, string(at))
	}
	var supportedSecretType []string
	for _, at := range []SecretType{SecretTypeRemote, SecretTypeConfig} {
		supportedSecretType = append(supportedSecretType, string(at))
	}

	flagset.Var(&o.AuthType, "auth-type",
		fmt.Sprintf("Type of authentication to use. supported values = %v", supportedAuthType))
	flagset.StringVar(&o.AuthPluginName, "auth-plugin-name", o.AuthPluginName,
		fmt.Sprintf("Authenticator plug-in name. --auth-type=%v must be set with this option",
			RemoteSecretAuthTypePlugin))
	flagset.StringToString("auth-plugin-config", o.AuthPluginConfig,
		fmt.Sprintf("Authenticator plug-in configuration. --auth-type=%v must be set with this option",
			RemoteSecretAuthTypePlugin))
	flagset.Var(&o.Type, "type",
		fmt.Sprintf("Type of the generated secret. supported values = %v", supportedSecretType))
	flagset.StringVarP(&o.ManifestsPath, "manifests", "d", "", mesh.ManifestsFlagHelpStr)
}

func (o *RemoteSecretOptions) prepare(flags *pflag.FlagSet) error {
	o.KubeOptions.prepare(flags)

	if o.ClusterName != "" {
		if !labels.IsDNS1123Label(o.ClusterName) {
			return fmt.Errorf("%v is not a valid DNS 1123 label", o.ClusterName)
		}
	}
	return nil
}

func createRemoteSecret(opt RemoteSecretOptions, client kube.ExtendedClient, env Environment) (*v1.Secret, error) {
	// generate the clusterName if not specified
	if opt.ClusterName == "" {
		uid, err := clusterUID(client)
		if err != nil {
			return nil, err
		}
		opt.ClusterName = string(uid)
	}

	var secretName string
	switch opt.Type {
	case SecretTypeRemote:
		secretName = remoteSecretNameFromClusterName(opt.ClusterName)
		if opt.ServiceAccountName == "" {
			opt.ServiceAccountName = constants.DefaultServiceAccountName
		}
	case SecretTypeConfig:
		secretName = configSecretName
		if opt.ServiceAccountName == "" {
			opt.ServiceAccountName = constants.DefaultConfigServiceAccountName
		}
	default:
		return nil, fmt.Errorf("unsupported type: %v", opt.Type)
	}
	tokenSecret, err := getServiceAccountSecret(client, opt)
	if err != nil {
		return nil, fmt.Errorf("could not get access token to read resources from local kube-apiserver: %v", err)
	}

	var server string
	if opt.ServerOverride != "" {
		server = opt.ServerOverride
	} else {
		server, err = getServerFromKubeconfig(opt.Context, env.GetConfig())
		if err != nil {
			return nil, err
		}
	}

	var remoteSecret *v1.Secret
	switch opt.AuthType {
	case RemoteSecretAuthTypeBearerToken:
		remoteSecret, err = createRemoteSecretFromTokenAndServer(tokenSecret, opt.ClusterName, server, secretName)
	case RemoteSecretAuthTypePlugin:
		authProviderConfig := &api.AuthProviderConfig{
			Name:   opt.AuthPluginName,
			Config: opt.AuthPluginConfig,
		}
		remoteSecret, err = createRemoteSecretFromPlugin(tokenSecret, server, opt.ClusterName, secretName,
			authProviderConfig)
	default:
		err = fmt.Errorf("unsupported authentication type: %v", opt.AuthType)
	}
	if err != nil {
		return nil, err
	}

	remoteSecret.Namespace = opt.Namespace
	return remoteSecret, nil
}

// CreateRemoteSecret creates a remote secret with credentials of the specified service account.
// This is useful for providing a cluster access to a remote apiserver.
func CreateRemoteSecret(opt RemoteSecretOptions, env Environment) (string, error) {
	client, err := env.CreateClient(opt.Context)
	if err != nil {
		return "", err
	}

	remoteSecret, err := createRemoteSecret(opt, client, env)
	if err != nil {
		return "", err
	}

	// convert any binary data to the string equivalent for easier review. The
	// kube-apiserver will convert this to binary before it persists it to storage.
	remoteSecret.StringData = make(map[string]string, len(remoteSecret.Data))
	for k, v := range remoteSecret.Data {
		remoteSecret.StringData[k] = string(v)
	}
	remoteSecret.Data = nil

	w := makeOutputWriterTestHook()
	if err := writeEncodedObject(w, remoteSecret); err != nil {
		return "", err
	}
	return w.String(), nil
}
