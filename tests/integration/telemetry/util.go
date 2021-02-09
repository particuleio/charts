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

package telemetry

import (
	"fmt"
	"strings"

	"istio.io/istio/pkg/test/framework/components/cluster"
	"istio.io/istio/pkg/test/framework/components/prometheus"
)

// promDump gets all of the recorded values for a metric by name and generates a report of the values.
// used for debugging of failures to provide a comprehensive view of traffic experienced.
func PromDump(cluster cluster.Cluster, prometheus prometheus.Instance, metric string) string {
	return PromDumpWithAttributes(cluster, prometheus, metric, nil)
}

// promDumpWithAttributes is used to get all of the recorded values of a metric for particular attributes.
// Attributes have to be of format %s=\"%s\"
// nolint: unparam
func PromDumpWithAttributes(cluster cluster.Cluster, prometheus prometheus.Instance, metric string, attributes []string) string {
	if value, err := prometheus.WaitForQuiesceForCluster(cluster, fmt.Sprintf("%s{%s}", metric, strings.Join(attributes, ", "))); err == nil {
		return value.String()
	}

	return ""
}
