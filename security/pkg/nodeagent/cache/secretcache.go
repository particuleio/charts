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

// Package cache is the in-memory secret store.
package cache

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/file"
	"istio.io/istio/pkg/queue"
	"istio.io/istio/pkg/security"
	"istio.io/istio/pkg/spiffe"
	"istio.io/istio/security/pkg/monitoring"
	nodeagentutil "istio.io/istio/security/pkg/nodeagent/util"
	pkiutil "istio.io/istio/security/pkg/pki/util"
	istiolog "istio.io/pkg/log"
)

var (
	cacheLog = istiolog.RegisterScope("cache", "cache debugging", 0)
	// The total timeout for any credential retrieval process, default value of 10s is used.
	totalTimeout = time.Second * 10
)

const (
	// The size of a private key for a leaf certificate.
	keySize = 2048

	// max retry number to wait CSR response come back to parse root cert from it.
	maxRetryNum = 5

	// initial retry wait time duration when waiting root cert is available.
	retryWaitDuration = 200 * time.Millisecond

	// RootCertReqResourceName is resource name of discovery request for root certificate.
	RootCertReqResourceName = "ROOTCA"

	// WorkloadKeyCertResourceName is the resource name of the discovery request for workload
	// identity.
	// TODO: change all the pilot one reference definition here instead.
	WorkloadKeyCertResourceName = "default"

	// firstRetryBackOffInMilliSec is the initial backoff time interval when hitting
	// non-retryable error in CSR request or while there is an error in reading file mounts.
	firstRetryBackOffInMilliSec = 50
)

// SecretManagerClient a SecretManager that signs CSRs using a provided security.Client. The primary
// usage is to fetch the two specially named resources: `default`, which refers to the workload's
// spiffe certificate, and ROOTCA, which contains just the root certificate for the workload
// certificates. These are separated only due to the fact that Envoy has them separated.
// Additionally, arbitrary certificates may be fetched from local files to support DestinationRule
// and Gateway. Note that certificates stored externally will be sent from Istiod directly; the
// in-agent SecretManagerClient has low privileges and cannot read Kubernetes Secrets or other
// storage backends. Istiod is in charge of determining whether the agent (ie SecretManagerClient) or
// Istiod will serve an SDS response, by selecting the appropriate cluster in the SDS configuration
// it serves.
//
// SecretManagerClient supports two modes of retrieving certificate (potentially at the same time):
// * File based certificates. If certs are mounted under well-known path /etc/certs/{key,cert,root-cert.pem},
//   requests for `default` and `ROOTCA` will automatically read from these files. Additionally,
//   certificates from Gateway/DestinationRule can also be served. This is done by parsing resource
//   names in accordance with model.SdsCertificateConfig (file-cert: and file-root:).
// * On demand CSRs. This is used only for the `default` certificate. When this resource is
//   requested, a CSR will be sent to the configured caClient.
//
// Callers are expected to only call GenerateSecret when a new certificate is required. Generally,
// this should be done a single time at startup, then repeatedly when the certificate is near
// expiration. To help users handle certificate expiration, any certificates created by the caClient
// will be monitored; when they are near expiration the notifyCallback function is triggered,
// prompting the client to call GenerateSecret again, if they still care about the certificate. For
// files, this callback is instead triggered on any change to the file (triggering on expiration
// would not be helpful, as all we can do is re-read the same file).
type SecretManagerClient struct {
	caClient security.Client

	// configOptions includes all configurable params for the cache.
	configOptions security.Options

	// callback function to invoke when detecting secret change.
	notifyCallback func(resourceName string)

	// Cache of workload certificate and root certificate. File based certs are never cached, as
	// lookup is cheap.
	cache secretCache

	// generateMutex ensures we do not send concurrent requests to generate a certificate
	generateMutex sync.Mutex

	// The paths for an existing certificate chain, key and root cert files. Istio agent will
	// use them as the source of secrets if they exist.
	existingCertificateFile model.SdsCertificateConfig

	// certWatcher watches the certificates for changes and triggers a notification to proxy.
	certWatcher *fsnotify.Watcher
	// certs being watched with file watcher.
	fileCerts map[FileCert]struct{}
	certMutex sync.RWMutex

	// outputMutex protects writes of certificates to disk
	outputMutex sync.Mutex

	// queue maintains all certificate rotation events that need to be triggered when they are about to expire
	queue queue.Delayed
	stop  chan struct{}
}

type secretCache struct {
	mu             sync.RWMutex
	workload       *security.SecretItem
	root           []byte
	rootExpiration time.Time
}

// GetRoot returns cached root cert and cert expiration time. This method is thread safe.
func (s *secretCache) GetRoot() (rootCert []byte, rootCertExpr time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.root, s.rootExpiration
}

// SetRoot sets root cert into cache. This method is thread safe.
func (s *secretCache) SetRoot(rootCert []byte, rootCertExpr time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.root = rootCert
	s.rootExpiration = rootCertExpr
}

func (s *secretCache) GetWorkload() *security.SecretItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.workload == nil {
		return nil
	}
	return s.workload
}

func (s *secretCache) SetWorkload(value *security.SecretItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workload = value
}

var _ security.SecretManager = &SecretManagerClient{}

// FileCert stores a reference to a certificate on disk
type FileCert struct {
	ResourceName string
	Filename     string
}

// NewSecretManagerClient creates a new SecretManagerClient.
func NewSecretManagerClient(caClient security.Client, options security.Options) (*SecretManagerClient, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	ret := &SecretManagerClient{
		queue:         queue.NewDelayed(queue.DelayQueueBuffer(0)),
		caClient:      caClient,
		configOptions: options,
		existingCertificateFile: model.SdsCertificateConfig{
			CertificatePath:   security.DefaultCertChainFilePath,
			PrivateKeyPath:    security.DefaultKeyFilePath,
			CaCertificatePath: security.DefaultRootCertFilePath,
		},
		certWatcher: watcher,
		fileCerts:   make(map[FileCert]struct{}),
		stop:        make(chan struct{}),
	}

	go ret.queue.Run(ret.stop)
	go ret.handleFileWatch()
	return ret, nil
}

func (sc *SecretManagerClient) Close() {
	_ = sc.certWatcher.Close()
	if sc.caClient != nil {
		sc.caClient.Close()
	}
	close(sc.stop)
}

func (sc *SecretManagerClient) SetUpdateCallback(f func(resourceName string)) {
	sc.certMutex.Lock()
	defer sc.certMutex.Unlock()
	sc.notifyCallback = f
}

func (sc *SecretManagerClient) CallUpdateCallback(resourceName string) {
	sc.certMutex.RLock()
	defer sc.certMutex.RUnlock()
	if sc.notifyCallback != nil {
		sc.notifyCallback(resourceName)
	}
}

// GenerateSecret generates new secret and cache the secret, this function is called by SDS.StreamSecrets
// and SDS.FetchSecret. Since credential passing from client may change, regenerate secret every time
// instead of reading from cache.
func (sc *SecretManagerClient) GenerateSecret(resourceName string) (secret *security.SecretItem, err error) {
	// Setup the call to store generated secret to disk
	defer func() {
		if secret == nil || err != nil {
			return
		}
		// We need to hold a mutex here, otherwise if two threads are writing the same certificate,
		// we may permanently end up with a mismatch key/cert pair. We still make end up temporarily
		// with mismatched key/cert pair since we cannot atomically write multiple files. It may be
		// possible by keeping the output in a directory with clever use of symlinks in the future,
		// if needed.
		sc.outputMutex.Lock()
		if resourceName == RootCertReqResourceName || resourceName == WorkloadKeyCertResourceName {
			if err := nodeagentutil.OutputKeyCertToDir(sc.configOptions.OutputKeyCertToDir, secret.PrivateKey,
				secret.CertificateChain, secret.RootCert); err != nil {
				cacheLog.Errorf("error when output the resource: %v", err)
			} else {
				resourceLog(resourceName).Debugf("output the resource to %v", sc.configOptions.OutputKeyCertToDir)
			}
		}
		sc.outputMutex.Unlock()
	}()

	// First try to generate secret from file.
	sdsFromFile, ns, err := sc.generateFileSecret(resourceName)

	if sdsFromFile {
		if err != nil {
			return nil, err
		}
		return ns, nil
	}

	if resourceName != RootCertReqResourceName {
		if c := sc.cache.GetWorkload(); c != nil {
			// cache hit
			cacheLog.WithLabels("ttl", time.Until(c.ExpireTime)).Info("returned workload certificate from cache")
			return c, nil
		}
		t0 := time.Now()
		sc.generateMutex.Lock()
		defer sc.generateMutex.Unlock()
		if c := sc.cache.GetWorkload(); c != nil {
			// Now that we got the lock, check again if there is a cached secret (from the caller holding the lock previously)
			cacheLog.WithLabels("ttl", time.Until(c.ExpireTime)).Info("returned delayed workload certificate from cache")
			return c, nil
		}
		if ts := time.Since(t0); ts > time.Second {
			cacheLog.Warnf("slow generate secret lock: %v", ts)
		}

		ns, err := sc.generateSecret(resourceName)
		if err != nil {
			return nil, fmt.Errorf("failed to generate workload certificate: %v", err)
		}

		cacheLog.WithLabels("latency", time.Since(t0), "ttl", time.Until(ns.ExpireTime)).Info("generated new workload certificate")
		sc.registerSecret(*ns)
		return ns, nil
	}
	// If request is for root certificate,
	// retry since rootCert may be empty until there is CSR response returned from CA.
	rootCert, rootCertExpr := sc.cache.GetRoot()
	if rootCert == nil {
		wait := retryWaitDuration
		retryNum := 0
		for ; retryNum < maxRetryNum; retryNum++ {
			time.Sleep(wait)
			rootCert, rootCertExpr = sc.cache.GetRoot()
			if rootCert != nil {
				break
			}

			wait *= 2
		}
	}

	if rootCert == nil {
		return nil, errors.New("failed to get root cert")
	}

	t := time.Now()
	ns = &security.SecretItem{
		ResourceName: resourceName,
		RootCert:     rootCert,
		ExpireTime:   rootCertExpr,
		CreatedTime:  t,
	}

	return ns, nil
}

func (sc *SecretManagerClient) addFileWatcher(file string, resourceName string) {
	// TODO(ramaraochavali): add integration test for file watcher functionality.
	// Check if this file is being already watched, if so ignore it. This check is needed here to
	// avoid processing duplicate events for the same file.
	sc.certMutex.Lock()
	defer sc.certMutex.Unlock()
	file = filepath.Clean(file)
	key := FileCert{
		ResourceName: resourceName,
		Filename:     file,
	}
	if _, alreadyWatching := sc.fileCerts[key]; alreadyWatching {
		cacheLog.Debugf("already watching file for %s", file)
		// Already watching, no need to do anything
		return
	}
	sc.fileCerts[key] = struct{}{}
	// File is not being watched, start watching now and trigger key push.
	cacheLog.Infof("adding watcher for file certificate %s", file)
	if err := sc.certWatcher.Add(file); err != nil {
		cacheLog.Errorf("%v: error adding watcher for file, skipping watches [%s] %v", resourceName, file, err)
		numFileWatcherFailures.Increment()
		return
	}
}

// If there is existing root certificates under a well known path, return true.
// Otherwise, return false.
func (sc *SecretManagerClient) rootCertificateExist(filePath string) bool {
	b, err := ioutil.ReadFile(filePath)
	if err != nil || len(b) == 0 {
		return false
	}
	return true
}

// If there is an existing private key and certificate under a well known path, return true.
// Otherwise, return false.
func (sc *SecretManagerClient) keyCertificateExist(certPath, keyPath string) bool {
	b, err := ioutil.ReadFile(certPath)
	if err != nil || len(b) == 0 {
		return false
	}
	b, err = ioutil.ReadFile(keyPath)
	if err != nil || len(b) == 0 {
		return false
	}

	return true
}

// Generate a root certificate item from the passed in rootCertPath
func (sc *SecretManagerClient) generateRootCertFromExistingFile(rootCertPath, resourceName string, workload bool) (*security.SecretItem, error) {
	rootCert, err := sc.readFileWithTimeout(rootCertPath)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var certExpireTime time.Time
	if certExpireTime, err = nodeagentutil.ParseCertAndGetExpiryTimestamp(rootCert); err != nil {
		cacheLog.Errorf("failed to extract expiration time in the root certificate loaded from file: %v", err)
		return nil, fmt.Errorf("failed to extract expiration time in the root certificate loaded from file: %v", err)
	}

	// Set the rootCert only if it is workload root cert.
	if workload {
		sc.cache.SetRoot(rootCert, certExpireTime)
	}
	return &security.SecretItem{
		ResourceName: resourceName,
		RootCert:     rootCert,
		ExpireTime:   certExpireTime,
		CreatedTime:  now,
	}, nil
}

// Generate a key and certificate item from the existing key certificate files from the passed in file paths.
func (sc *SecretManagerClient) generateKeyCertFromExistingFiles(certChainPath, keyPath, resourceName string) (*security.SecretItem, error) {
	certChain, err := sc.readFileWithTimeout(certChainPath)
	if err != nil {
		return nil, err
	}
	keyPEM, err := sc.readFileWithTimeout(keyPath)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var certExpireTime time.Time
	if certExpireTime, err = nodeagentutil.ParseCertAndGetExpiryTimestamp(certChain); err != nil {
		cacheLog.Errorf("failed to extract expiration time in the certificate loaded from file: %v", err)
		return nil, fmt.Errorf("failed to extract expiration time in the certificate loaded from file: %v", err)
	}

	return &security.SecretItem{
		CertificateChain: certChain,
		PrivateKey:       keyPEM,
		ResourceName:     resourceName,
		CreatedTime:      now,
		ExpireTime:       certExpireTime,
	}, nil
}

// readFileWithTimeout reads the given file with timeout. It returns error
// if it is not able to read file after timeout.
func (sc *SecretManagerClient) readFileWithTimeout(path string) ([]byte, error) {
	retryBackoffInMS := int64(firstRetryBackOffInMilliSec)
	for {
		cert, err := ioutil.ReadFile(path)
		if err == nil {
			return cert, nil
		}
		select {
		case <-time.After(time.Duration(retryBackoffInMS)):
			retryBackoffInMS *= 2
		case <-time.After(totalTimeout):
			return nil, err
		case <-sc.stop:
			return nil, err
		}
	}
}

func (sc *SecretManagerClient) generateFileSecret(resourceName string) (bool, *security.SecretItem, error) {
	logPrefix := cacheLogPrefix(resourceName)

	cf := sc.existingCertificateFile
	// outputToCertificatePath handles a special case where we have configured to output certificates
	// to the special /etc/certs directory. In this case, we need to ensure we do *not* read from
	// these files, otherwise we would never rotate.
	outputToCertificatePath, ferr := file.DirEquals(filepath.Dir(cf.CertificatePath), sc.configOptions.OutputKeyCertToDir)
	if ferr != nil {
		return false, nil, ferr
	}
	// When there are existing root certificates, or private key and certificate under
	// a well known path, they are used in the SDS response.
	sdsFromFile := false
	var err error
	var sitem *security.SecretItem

	switch {
	// Default root certificate.
	case resourceName == RootCertReqResourceName && sc.rootCertificateExist(cf.CaCertificatePath) && !outputToCertificatePath:
		sdsFromFile = true
		if sitem, err = sc.generateRootCertFromExistingFile(cf.CaCertificatePath, resourceName, true); err == nil {
			sc.addFileWatcher(cf.CaCertificatePath, resourceName)
		}
	// Default workload certificate.
	case resourceName == WorkloadKeyCertResourceName && sc.keyCertificateExist(cf.CertificatePath, cf.PrivateKeyPath) && !outputToCertificatePath:
		sdsFromFile = true
		if sitem, err = sc.generateKeyCertFromExistingFiles(cf.CertificatePath, cf.PrivateKeyPath, resourceName); err == nil {
			// Adding cert is sufficient here as key can't change without changing the cert.
			sc.addFileWatcher(cf.CertificatePath, resourceName)
		}
	default:
		// Check if the resource name refers to a file mounted certificate.
		// Currently used in destination rules and server certs (via metadata).
		// Based on the resource name, we need to read the secret from a file encoded in the resource name.
		cfg, ok := model.SdsCertificateConfigFromResourceName(resourceName)
		sdsFromFile = ok
		switch {
		case ok && cfg.IsRootCertificate():
			if sitem, err = sc.generateRootCertFromExistingFile(cfg.CaCertificatePath, resourceName, false); err == nil {
				sc.addFileWatcher(cfg.CaCertificatePath, resourceName)
			}
		case ok && cfg.IsKeyCertificate():
			if sitem, err = sc.generateKeyCertFromExistingFiles(cfg.CertificatePath, cfg.PrivateKeyPath, resourceName); err == nil {
				// Adding cert is sufficient here as key can't change without changing the cert.
				sc.addFileWatcher(cfg.CertificatePath, resourceName)
			}
		}
	}

	if sdsFromFile {
		if err != nil {
			cacheLog.Errorf("%s failed to generate secret for proxy from file: %v",
				logPrefix, err)
			return sdsFromFile, nil, err
		}
		cacheLog.WithLabels("resource", resourceName).Info("read certificate from file")
		// We do not register the secret. Unlike on-demand CSRs, there is nothing we can do if a file
		// cert expires; there is no point sending an update when its near expiry. Instead, a
		// separate file watcher will ensure if the file changes we trigger an update.
		return sdsFromFile, sitem, nil
	}
	return sdsFromFile, nil, nil
}

func (sc *SecretManagerClient) generateSecret(resourceName string) (*security.SecretItem, error) {
	if sc.caClient == nil {
		return nil, fmt.Errorf("attempted to fetch secret, but ca client is nil")
	}
	logPrefix := cacheLogPrefix(resourceName)

	csrHostName := &spiffe.Identity{
		TrustDomain:    sc.configOptions.TrustDomain,
		Namespace:      sc.configOptions.WorkloadNamespace,
		ServiceAccount: sc.configOptions.ServiceAccount,
	}

	cacheLog.Debugf("constructed host name for CSR: %s", csrHostName.String())
	options := pkiutil.CertOptions{
		Host:       csrHostName.String(),
		RSAKeySize: keySize,
		PKCS8Key:   sc.configOptions.Pkcs8Keys,
		ECSigAlg:   pkiutil.SupportedECSignatureAlgorithms(sc.configOptions.ECCSigAlg),
	}

	// Generate the cert/key, send CSR to CA.
	csrPEM, keyPEM, err := pkiutil.GenCSR(options)
	if err != nil {
		cacheLog.Errorf("%s failed to generate key and certificate for CSR: %v", logPrefix, err)
		return nil, err
	}

	numOutgoingRequests.With(RequestType.Value(monitoring.CSR)).Increment()
	timeBeforeCSR := time.Now()
	certChainPEM, err := sc.caClient.CSRSign(csrPEM, int64(sc.configOptions.SecretTTL.Seconds()))
	csrLatency := float64(time.Since(timeBeforeCSR).Nanoseconds()) / float64(time.Millisecond)
	outgoingLatency.With(RequestType.Value(monitoring.CSR)).Record(csrLatency)
	if err != nil {
		numFailedOutgoingRequests.With(RequestType.Value(monitoring.CSR)).Increment()
		return nil, err
	}

	certChain := concatCerts(certChainPEM)

	var expireTime time.Time
	// Cert expire time by default is createTime + sc.configOptions.SecretTTL.
	// Istiod respects SecretTTL that passed to it and use it decide TTL of cert it issued.
	// Some customer CA may override TTL param that's passed to it.
	if expireTime, err = nodeagentutil.ParseCertAndGetExpiryTimestamp(certChain); err != nil {
		cacheLog.Errorf("%s failed to extract expire time from server certificate in CSR response %+v: %v",
			logPrefix, certChainPEM, err)
		return nil, fmt.Errorf("failed to extract expire time from server certificate in CSR response: %v", err)
	}

	length := len(certChainPEM)
	rootCert, _ := sc.cache.GetRoot()
	// Leaf cert is element '0'. Root cert is element 'n'.
	rootCertChanged := !bytes.Equal(rootCert, []byte(certChainPEM[length-1]))
	if rootCert == nil || rootCertChanged {
		rootCertExpireTime, err := nodeagentutil.ParseCertAndGetExpiryTimestamp([]byte(certChainPEM[length-1]))
		if err == nil {
			sc.cache.SetRoot([]byte(certChainPEM[length-1]), rootCertExpireTime)
		} else {
			cacheLog.Errorf("%s failed to parse root certificate in CSR response: %v", logPrefix, err)
			rootCertChanged = false
		}
	}

	if rootCertChanged {
		cacheLog.Info("Root cert has changed, start rotating root cert")
		sc.CallUpdateCallback(RootCertReqResourceName)
	}

	return &security.SecretItem{
		CertificateChain: certChain,
		PrivateKey:       keyPEM,
		ResourceName:     resourceName,
		CreatedTime:      time.Now(),
		ExpireTime:       expireTime,
	}, nil
}

func (sc *SecretManagerClient) rotateTime(secret security.SecretItem) time.Duration {
	secretLifeTime := secret.ExpireTime.Sub(secret.CreatedTime)
	gracePeriod := time.Duration((sc.configOptions.SecretRotationGracePeriodRatio) * float64(secretLifeTime))
	delay := time.Until(secret.ExpireTime.Add(-gracePeriod))
	if delay < 0 {
		delay = 0
	}
	return delay
}

func (sc *SecretManagerClient) registerSecret(item security.SecretItem) {
	delay := sc.rotateTime(item)
	// In case there are two calls to GenerateSecret at once, we don't want both to be concurrently registered
	if sc.cache.GetWorkload() != nil {
		resourceLog(item.ResourceName).Infof("skip scheduling certificate rotation, already scheduled")
		return
	}
	sc.cache.SetWorkload(&item)
	resourceLog(item.ResourceName).Debugf("scheduled certificate for rotation in %v", delay)
	sc.queue.PushDelayed(func() error {
		resourceLog(item.ResourceName).Debugf("rotating certificate")
		// Clear the cache so the next call generates a fresh certificate
		sc.cache.SetWorkload(nil)

		sc.CallUpdateCallback(item.ResourceName)
		return nil
	}, delay)
}

func (sc *SecretManagerClient) handleFileWatch() {
	for {
		select {
		case event, ok := <-sc.certWatcher.Events:
			// Channel is closed.
			if !ok {
				return
			}
			// We only care about updates that change the file content
			if !(isWrite(event) || isRemove(event) || isCreate(event)) {
				continue
			}
			cacheLog.Debugf("file cert update: %v", event)

			sc.certMutex.RLock()
			resources := sc.fileCerts
			sc.certMutex.RUnlock()
			// Trigger callbacks for all resources referencing this file. This is practically always
			// a single resource.
			for k := range resources {
				if k.Filename == event.Name {
					sc.CallUpdateCallback(k.ResourceName)
				}
			}

		case err, ok := <-sc.certWatcher.Errors:
			// Channel is closed.
			if !ok {
				return
			}

			cacheLog.Errorf("certificate watch error: %v", err)
		}
	}
}

func isWrite(event fsnotify.Event) bool {
	return event.Op&fsnotify.Write == fsnotify.Write
}

func isCreate(event fsnotify.Event) bool {
	return event.Op&fsnotify.Create == fsnotify.Create
}

func isRemove(event fsnotify.Event) bool {
	return event.Op&fsnotify.Remove == fsnotify.Remove
}

// concatCerts concatenates PEM certificates, making sure each one starts on a new line
func concatCerts(certsPEM []string) []byte {
	if len(certsPEM) == 0 {
		return []byte{}
	}
	var certChain bytes.Buffer
	for i, c := range certsPEM {
		certChain.WriteString(c)
		if i < len(certsPEM)-1 && !strings.HasSuffix(c, "\n") {
			certChain.WriteString("\n")
		}
	}
	return certChain.Bytes()
}
