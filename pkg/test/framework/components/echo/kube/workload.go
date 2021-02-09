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

package kube

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/go-multierror"
	kubeCore "k8s.io/api/core/v1"

	istioKube "istio.io/istio/pkg/kube"
	"istio.io/istio/pkg/test"
	"istio.io/istio/pkg/test/echo/client"
	"istio.io/istio/pkg/test/echo/common"
	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/components/echo"
	"istio.io/istio/pkg/test/framework/errors"
	"istio.io/istio/pkg/test/framework/resource"
	"istio.io/istio/pkg/test/util/retry"
)

const (
	appContainerName = "app"
)

var _ echo.Workload = &workload{}

type workload struct {
	*client.Instance

	pod       kubeCore.Pod
	forwarder istioKube.PortForwarder
	sidecar   *sidecar
	cluster   cluster.Cluster
	ctx       resource.Context
}

func newWorkload(pod kubeCore.Pod, sidecared bool, grpcPort uint16, cluster cluster.Cluster,
	tls *common.TLSSettings, ctx resource.Context) (*workload, error) {
	// Create a forwarder to the command port of the app.
	var forwarder istioKube.PortForwarder
	if err := retry.UntilSuccess(func() error {
		fw, err := cluster.NewPortForwarder(pod.Name, pod.Namespace, "", 0, int(grpcPort))
		if err != nil {
			return fmt.Errorf("new port forwarder: %v", err)
		}
		if err = fw.Start(); err != nil {
			fw.Close()
			return fmt.Errorf("forwarder start: %v", err)
		}
		forwarder = fw
		return nil
	}, retry.Delay(1*time.Second), retry.Timeout(10*time.Second)); err != nil {
		return nil, err
	}

	// Create a gRPC client to this workload.
	c, err := client.New(forwarder.Address(), tls)
	if err != nil {
		forwarder.Close()
		return nil, fmt.Errorf("grpc client: %v", err)
	}

	var s *sidecar
	if sidecared {
		if s, err = newSidecar(pod, cluster); err != nil {
			return nil, err
		}
	}

	return &workload{
		pod:       pod,
		forwarder: forwarder,
		Instance:  c,
		sidecar:   s,
		cluster:   cluster,
		ctx:       ctx,
	}, nil
}

func (w *workload) Close() (err error) {
	if w.Instance != nil {
		err = multierror.Append(err, w.Instance.Close()).ErrorOrNil()
	}
	if w.forwarder != nil {
		w.forwarder.Close()
	}
	if w.ctx.Settings().FailOnDeprecation && w.sidecar != nil {
		err = multierror.Append(err, w.checkDeprecation()).ErrorOrNil()
	}
	return
}

func (w *workload) checkDeprecation() error {
	logs, err := w.sidecar.Logs()
	if err != nil {
		return fmt.Errorf("could not get sidecar logs to inspect for deprecation messages: %v", err)
	}

	info := fmt.Sprintf("pod: %s/%s", w.pod.Namespace, w.pod.Name)
	return errors.FindDeprecatedMessagesInEnvoyLog(logs, info)
}

func (w *workload) PodName() string {
	return w.pod.Name
}

func (w *workload) Address() string {
	return w.pod.Status.PodIP
}

func (w *workload) Sidecar() echo.Sidecar {
	return w.sidecar
}

func (w *workload) Logs() (string, error) {
	return w.cluster.PodLogs(context.TODO(), w.pod.Name, w.pod.Namespace, appContainerName, false)
}

func (w *workload) LogsOrFail(t test.Failer) string {
	t.Helper()
	logs, err := w.Logs()
	if err != nil {
		t.Fatal(err)
	}
	return logs
}
