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

package util

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	http_conn "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/envoyproxy/go-control-plane/pkg/conversion"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/gogo/protobuf/types"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/duration"
	pstruct "github.com/golang/protobuf/ptypes/struct"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/hashicorp/go-multierror"

	meshconfig "istio.io/api/mesh/v1alpha1"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/features"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pilot/pkg/serviceregistry"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/host"
	"istio.io/istio/pkg/config/labels"
	"istio.io/istio/pkg/util/strcase"
	"istio.io/pkg/log"
)

const (
	// BlackHoleCluster to catch traffic from routes with unresolved clusters. Traffic arriving here goes nowhere.
	BlackHoleCluster = "BlackHoleCluster"
	// BlackHole is the name of the virtual host and route name used to block all traffic
	BlackHole = "block_all"
	// PassthroughCluster to forward traffic to the original destination requested. This cluster is used when
	// traffic does not match any listener in envoy.
	PassthroughCluster = "PassthroughCluster"
	// Passthrough is the name of the virtual host used to forward traffic to the
	// PassthroughCluster
	Passthrough = "allow_any"
	// PassthroughFilterChain to catch traffic that doesn't match other filter chains.
	PassthroughFilterChain = "PassthroughFilterChain"

	// Inbound pass through cluster need to the bind the loopback ip address for the security and loop avoidance.
	InboundPassthroughClusterIpv4 = "InboundPassthroughClusterIpv4"
	InboundPassthroughClusterIpv6 = "InboundPassthroughClusterIpv6"
	// 6 is the magical number for inbound: 15006, 127.0.0.6, ::6
	InboundPassthroughBindIpv4 = "127.0.0.6"
	InboundPassthroughBindIpv6 = "::6"

	// SniClusterFilter is the name of the sni_cluster envoy filter
	SniClusterFilter = "envoy.filters.network.sni_cluster"

	// IstioMetadataKey is the key under which metadata is added to a route or cluster
	// regarding the virtual service or destination rule used for each
	IstioMetadataKey = "istio"

	// EnvoyTransportSocketMetadataKey is the key under which metadata is added to an endpoint
	// which determines the endpoint level transport socket configuration.
	EnvoyTransportSocketMetadataKey = "envoy.transport_socket_match"

	// EnvoyRawBufferSocketName matched with hardcoded built-in Envoy transport name which determines
	// endpoint level plantext transport socket configuration
	EnvoyRawBufferSocketName = wellknown.TransportSocketRawBuffer

	// EnvoyTLSSocketName matched with hardcoded built-in Envoy transport name which determines endpoint
	// level tls transport socket configuration
	EnvoyTLSSocketName = wellknown.TransportSocketTls

	// StatName patterns
	serviceStatPattern         = "%SERVICE%"
	serviceFQDNStatPattern     = "%SERVICE_FQDN%"
	servicePortStatPattern     = "%SERVICE_PORT%"
	servicePortNameStatPattern = "%SERVICE_PORT_NAME%"
	subsetNameStatPattern      = "%SUBSET_NAME%"
)

// ALPNH2Only advertises that Proxy is going to use HTTP/2 when talking to the cluster.
var ALPNH2Only = []string{"h2"}

// ALPNInMeshH2 advertises that Proxy is going to use HTTP/2 when talking to the in-mesh cluster.
// The custom "istio" value indicates in-mesh traffic and it's going to be used for routing decisions.
// Once Envoy supports client-side ALPN negotiation, this should be {"istio", "h2", "http/1.1"}.
var ALPNInMeshH2 = []string{"istio", "h2"}

// ALPNInMeshH2WithMxc advertises that Proxy is going to use HTTP/2 when talking to the in-mesh cluster.
// The custom "istio" value indicates in-mesh traffic and it's going to be used for routing decisions.
// The custom "istio-peer-exchange" value indicates, metadata exchange is enabled for TCP.
var ALPNInMeshH2WithMxc = []string{"istio-peer-exchange", "istio", "h2"}

// ALPNInMesh advertises that Proxy is going to talk to the in-mesh cluster.
// The custom "istio" value indicates in-mesh traffic and it's going to be used for routing decisions.
var ALPNInMesh = []string{"istio"}

// ALPNInMeshWithMxc advertises that Proxy is going to talk to the in-mesh cluster and has metadata exchange enabled for
// TCP. The custom "istio-peer-exchange" value indicates, metadata exchange is enabled for TCP. The custom "istio" value
// indicates in-mesh traffic and it's going to be used for routing decisions.
var ALPNInMeshWithMxc = []string{"istio-peer-exchange", "istio"}

// ALPNHttp advertises that Proxy is going to talking either http2 or http 1.1.
var ALPNHttp = []string{"h2", "http/1.1"}

// ALPNDownstream advertises that Proxy is going to talking either tcp(for metadata exchange), http2 or http 1.1.
var ALPNDownstream = []string{"istio-peer-exchange", "h2", "http/1.1"}

// FallThroughFilterChainBlackHoleService is the blackhole service used for fall though
// filter chain
var FallThroughFilterChainBlackHoleService = &model.Service{
	Hostname: host.Name(BlackHoleCluster),
	Attributes: model.ServiceAttributes{
		Name: BlackHoleCluster,
	},
}

// FallThroughFilterChainPassthroughService is the passthrough service used for fall though
var FallThroughFilterChainPassthroughService = &model.Service{
	Hostname: host.Name(PassthroughCluster),
	Attributes: model.ServiceAttributes{
		Name: PassthroughCluster,
	},
}

func getMaxCidrPrefix(addr string) uint32 {
	ip := net.ParseIP(addr)
	if ip.To4() == nil {
		// ipv6 address
		return 128
	}
	// ipv4 address
	return 32
}

func ListContains(haystack []string, needle string) bool {
	for _, n := range haystack {
		if needle == n {
			return true
		}
	}
	return false
}

// ConvertAddressToCidr converts from string to CIDR proto
func ConvertAddressToCidr(addr string) *core.CidrRange {
	if len(addr) == 0 {
		return nil
	}

	cidr := &core.CidrRange{
		AddressPrefix: addr,
		PrefixLen: &wrappers.UInt32Value{
			Value: getMaxCidrPrefix(addr),
		},
	}

	if strings.Contains(addr, "/") {
		parts := strings.Split(addr, "/")
		cidr.AddressPrefix = parts[0]
		prefix, _ := strconv.Atoi(parts[1])
		cidr.PrefixLen.Value = uint32(prefix)
	}
	return cidr
}

// BuildAddress returns a SocketAddress with the given ip and port or uds.
func BuildAddress(bind string, port uint32) *core.Address {
	if port != 0 {
		return &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Address: bind,
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: port,
					},
				},
			},
		}
	}

	return &core.Address{
		Address: &core.Address_Pipe{
			Pipe: &core.Pipe{
				Path: strings.TrimPrefix(bind, model.UnixAddressPrefix),
			},
		},
	}
}

// MessageToAnyWithError converts from proto message to proto Any
func MessageToAnyWithError(msg proto.Message) (*any.Any, error) {
	b := proto.NewBuffer(nil)
	b.SetDeterministic(true)
	err := b.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return &any.Any{
		// nolint: staticcheck
		TypeUrl: "type.googleapis.com/" + proto.MessageName(msg),
		Value:   b.Bytes(),
	}, nil
}

// MessageToAny converts from proto message to proto Any
func MessageToAny(msg proto.Message) *any.Any {
	out, err := MessageToAnyWithError(msg)
	if err != nil {
		log.Error(fmt.Sprintf("error marshaling Any %s: %v", msg.String(), err))
		return nil
	}
	return out
}

// MessageToStruct converts from proto message to proto Struct
func MessageToStruct(msg proto.Message) *pstruct.Struct {
	s, err := conversion.MessageToStruct(msg)
	if err != nil {
		log.Error(err.Error())
		return &pstruct.Struct{}
	}
	return s
}

// GogoDurationToDuration converts from gogo proto duration to time.duration
func GogoDurationToDuration(d *types.Duration) *duration.Duration {
	if d == nil {
		return nil
	}
	dur, err := types.DurationFromProto(d)
	if err != nil {
		// TODO(mostrowski): add error handling instead.
		log.Warnf("error converting duration %#v, using 0: %v", d, err)
		return nil
	}
	return ptypes.DurationProto(dur)
}

// SortVirtualHosts sorts a slice of virtual hosts by name.
//
// Envoy computes a hash of RDS to see if things have changed - hash is affected by order of elements in the filter. Therefore
// we sort virtual hosts by name before handing them back so the ordering is stable across HTTP Route Configs.
func SortVirtualHosts(hosts []*route.VirtualHost) {
	if len(hosts) < 2 {
		return
	}
	sort.SliceStable(hosts, func(i, j int) bool {
		return hosts[i].Name < hosts[j].Name
	})
}

// IsIstioVersionGE18 checks whether the given Istio version is greater than or equals 1.8.
func IsIstioVersionGE18(node *model.Proxy) bool {
	return node == nil || node.IstioVersion == nil ||
		node.IstioVersion.Compare(&model.IstioVersion{Major: 1, Minor: 8, Patch: -1}) >= 0
}

// IsIstioVersionGE19 checks whether the given Istio version is greater than or equals 1.9.
func IsIstioVersionGE19(node *model.Proxy) bool {
	return node == nil || node.IstioVersion == nil ||
		node.IstioVersion.Compare(&model.IstioVersion{Major: 1, Minor: 9, Patch: -1}) >= 0
}

// IsIstioVersionGE181 checks whether the given Istio version is greater than or equals 1.8.1
func IsIstioVersionGE181(node *model.Proxy) bool {
	return node == nil || node.IstioVersion == nil ||
		node.IstioVersion.Compare(&model.IstioVersion{Major: 1, Minor: 8, Patch: 1}) >= 0
}

func BuildInboundSubsetKey(node *model.Proxy, subsetName string, hostname host.Name, servicePort int, endpointPort int) string {
	if IsIstioVersionGE181(node) {
		// On 1.8.1+ Proxies, we use format inbound|endpointPort||. Telemetry no longer requires the hostname
		return model.BuildSubsetKey(model.TrafficDirectionInbound, "", "", endpointPort)
	} else if IsIstioVersionGE18(node) {
		// On 1.8.0 Proxies, we used format inbound|servicePort||. Since changing a cluster name leads to a 503
		// blip on upgrade, we will support this legacy format.
		return model.BuildSubsetKey(model.TrafficDirectionInbound, "", "", servicePort)
	}
	return model.BuildSubsetKey(model.TrafficDirectionInbound, subsetName, hostname, servicePort)
}

func IsProtocolSniffingEnabledForPort(port *model.Port) bool {
	return features.EnableProtocolSniffingForOutbound && port.Protocol.IsUnsupported()
}

func IsProtocolSniffingEnabledForInboundPort(port *model.Port) bool {
	return features.EnableProtocolSniffingForInbound && port.Protocol.IsUnsupported()
}

func IsProtocolSniffingEnabledForOutboundPort(port *model.Port) bool {
	return features.EnableProtocolSniffingForOutbound && port.Protocol.IsUnsupported()
}

// ConvertLocality converts '/' separated locality string to Locality struct.
func ConvertLocality(locality string) *core.Locality {
	if locality == "" {
		return &core.Locality{}
	}

	region, zone, subzone := model.SplitLocalityLabel(locality)
	return &core.Locality{
		Region:  region,
		Zone:    zone,
		SubZone: subzone,
	}
}

// ConvertLocality converts '/' separated locality string to Locality struct.
func LocalityToString(l *core.Locality) string {
	if l == nil {
		return ""
	}
	resp := l.Region
	if l.Zone == "" {
		return resp
	}
	resp += "/" + l.Zone
	if l.SubZone == "" {
		return resp
	}
	resp += "/" + l.SubZone
	return resp
}

// IsLocalityEmpty checks if a locality is empty (checking region is good enough, based on how its initialized)
func IsLocalityEmpty(locality *core.Locality) bool {
	if locality == nil || (len(locality.GetRegion()) == 0) {
		return true
	}
	return false
}

func LocalityMatch(proxyLocality *core.Locality, ruleLocality string) bool {
	ruleRegion, ruleZone, ruleSubzone := model.SplitLocalityLabel(ruleLocality)
	regionMatch := ruleRegion == "*" || proxyLocality.GetRegion() == ruleRegion
	zoneMatch := ruleZone == "*" || ruleZone == "" || proxyLocality.GetZone() == ruleZone
	subzoneMatch := ruleSubzone == "*" || ruleSubzone == "" || proxyLocality.GetSubZone() == ruleSubzone

	if regionMatch && zoneMatch && subzoneMatch {
		return true
	}
	return false
}

func LbPriority(proxyLocality, endpointsLocality *core.Locality) int {
	if proxyLocality.GetRegion() == endpointsLocality.GetRegion() {
		if proxyLocality.GetZone() == endpointsLocality.GetZone() {
			if proxyLocality.GetSubZone() == endpointsLocality.GetSubZone() {
				return 0
			}
			return 1
		}
		return 2
	}
	return 3
}

// return a shallow copy ClusterLoadAssignment
func CloneClusterLoadAssignment(original *endpoint.ClusterLoadAssignment) *endpoint.ClusterLoadAssignment {
	if original == nil {
		return nil
	}
	out := &endpoint.ClusterLoadAssignment{}

	out.ClusterName = original.ClusterName
	out.Endpoints = cloneLocalityLbEndpoints(original.Endpoints)
	out.Policy = original.Policy

	return out
}

// return a shallow copy LocalityLbEndpoints
func cloneLocalityLbEndpoints(endpoints []*endpoint.LocalityLbEndpoints) []*endpoint.LocalityLbEndpoints {
	out := make([]*endpoint.LocalityLbEndpoints, 0, len(endpoints))
	for _, ep := range endpoints {
		clone := &endpoint.LocalityLbEndpoints{}
		clone.Locality = ep.Locality
		clone.LbEndpoints = ep.LbEndpoints
		clone.Proximity = ep.Proximity
		clone.Priority = ep.Priority
		if ep.LoadBalancingWeight != nil {
			clone.LoadBalancingWeight = &wrappers.UInt32Value{
				Value: ep.GetLoadBalancingWeight().GetValue(),
			}
		}
		out = append(out, clone)
	}
	return out
}

// BuildConfigInfoMetadata builds core.Metadata struct containing the
// name.namespace of the config, the type, etc.
func BuildConfigInfoMetadata(config config.Meta) *core.Metadata {
	return AddConfigInfoMetadata(nil, config)
}

// AddConfigInfoMetadata adds name.namespace of the config, the type, etc
// to the given core.Metadata struct, if metadata is not initialized, build a new metadata.
func AddConfigInfoMetadata(metadata *core.Metadata, config config.Meta) *core.Metadata {
	if metadata == nil {
		metadata = &core.Metadata{
			FilterMetadata: map[string]*pstruct.Struct{},
		}
	}
	s := "/apis/" + config.GroupVersionKind.Group + "/" + config.GroupVersionKind.Version + "/namespaces/" + config.Namespace + "/" +
		strcase.CamelCaseToKebabCase(config.GroupVersionKind.Kind) + "/" + config.Name
	if _, ok := metadata.FilterMetadata[IstioMetadataKey]; !ok {
		metadata.FilterMetadata[IstioMetadataKey] = &pstruct.Struct{
			Fields: map[string]*pstruct.Value{},
		}
	}
	metadata.FilterMetadata[IstioMetadataKey].Fields["config"] = &pstruct.Value{
		Kind: &pstruct.Value_StringValue{
			StringValue: s,
		},
	}
	return metadata
}

// AddSubsetToMetadata will build a new core.Metadata struct containing the
// subset name supplied. This is used for telemetry reporting. A new core.Metadata
// is created to prevent modification to shared base Metadata across subsets, etc.
// This should be called after the initial "istio" metadata has been created for the
// cluster. If the "istio" metadata field is not already defined, the subset information will
// not be added (to prevent adding this information where not needed).
func AddSubsetToMetadata(md *core.Metadata, subset string) *core.Metadata {
	updatedMeta := &core.Metadata{}
	proto.Merge(updatedMeta, md)
	if istioMeta, ok := updatedMeta.FilterMetadata[IstioMetadataKey]; ok {
		istioMeta.Fields["subset"] = &pstruct.Value{
			Kind: &pstruct.Value_StringValue{
				StringValue: subset,
			},
		}
	}
	return updatedMeta
}

// IsHTTPFilterChain returns true if the filter chain contains a HTTP connection manager filter
func IsHTTPFilterChain(filterChain *listener.FilterChain) bool {
	for _, f := range filterChain.Filters {
		if f.Name == wellknown.HTTPConnectionManager {
			return true
		}
	}
	return false
}

// MergeAnyWithStruct merges a given struct into the given Any typed message by dynamically inferring the
// type of Any, converting the struct into the inferred type, merging the two messages, and then
// marshaling the merged message back into Any.
func MergeAnyWithStruct(a *any.Any, pbStruct *pstruct.Struct) (*any.Any, error) {
	// Assuming that Pilot is compiled with this type [which should always be the case]
	var err error
	var x ptypes.DynamicAny

	// First get an object of type used by this message
	if err = ptypes.UnmarshalAny(a, &x); err != nil {
		return nil, err
	}

	// Create a typed copy. We will convert the user's struct to this type
	temp := proto.Clone(x.Message)
	temp.Reset()
	if err = conversion.StructToMessage(pbStruct, temp); err != nil {
		return nil, err
	}

	// Merge the two typed protos
	proto.Merge(x.Message, temp)
	var retVal *any.Any
	// Convert the merged proto back to any
	if retVal, err = ptypes.MarshalAny(x.Message); err != nil {
		return nil, err
	}

	return retVal, nil
}

// MergeAnyWithAny merges a given any typed message into the given Any typed message by dynamically inferring the
// type of Any
func MergeAnyWithAny(dst *any.Any, src *any.Any) (*any.Any, error) {
	// Assuming that Pilot is compiled with this type [which should always be the case]
	var err error
	var dstX, srcX ptypes.DynamicAny

	// get an object of type used by this message
	if err = ptypes.UnmarshalAny(dst, &dstX); err != nil {
		return nil, err
	}

	// get an object of type used by this message
	if err = ptypes.UnmarshalAny(src, &srcX); err != nil {
		return nil, err
	}

	// Merge the two typed protos
	proto.Merge(dstX.Message, srcX.Message)
	var retVal *any.Any
	// Convert the merged proto back to dst
	if retVal, err = ptypes.MarshalAny(dstX.Message); err != nil {
		return nil, err
	}

	return retVal, nil
}

// BuildLbEndpointMetadata adds metadata values to a lb endpoint
func BuildLbEndpointMetadata(network, tlsMode, workloadname, namespace string, labels labels.Instance) *core.Metadata {
	if network == "" && tlsMode == model.DisabledTLSModeLabel && !shouldAddTelemetryLabel(workloadname) {
		return nil
	}

	metadata := &core.Metadata{
		FilterMetadata: map[string]*pstruct.Struct{},
	}

	if network != "" {
		addIstioEndpointLabel(metadata, "network", &pstruct.Value{Kind: &pstruct.Value_StringValue{StringValue: network}})
	}

	if tlsMode != "" {
		metadata.FilterMetadata[EnvoyTransportSocketMetadataKey] = &pstruct.Struct{
			Fields: map[string]*pstruct.Value{
				model.TLSModeLabelShortname: {Kind: &pstruct.Value_StringValue{StringValue: tlsMode}},
			},
		}
	}

	// Add compressed telemetry metadata. Note this is a short term solution to make server workload metadata
	// available at client sidecar, so that telemetry filter could use for metric labels. This is useful for two cases:
	// server does not have sidecar injected, and request fails to reach server and thus metadata exchange does not happen.
	// Due to performance concern, telemetry metadata is compressed into a semicolon separted string:
	// workload-name;namespace;canonical-service-name;canonical-service-revision.
	if shouldAddTelemetryLabel(workloadname) {
		var sb strings.Builder
		sb.WriteString(workloadname)
		sb.WriteString(";")
		sb.WriteString(namespace)
		sb.WriteString(";")
		if csn, ok := labels[model.IstioCanonicalServiceLabelName]; ok {
			sb.WriteString(csn)
		}
		sb.WriteString(";")
		if csr, ok := labels[model.IstioCanonicalServiceRevisionLabelName]; ok {
			sb.WriteString(csr)
		}
		addIstioEndpointLabel(metadata, "workload", &pstruct.Value{Kind: &pstruct.Value_StringValue{StringValue: sb.String()}})
	}

	return metadata
}

func addIstioEndpointLabel(metadata *core.Metadata, key string, val *pstruct.Value) {
	if _, ok := metadata.FilterMetadata[IstioMetadataKey]; !ok {
		metadata.FilterMetadata[IstioMetadataKey] = &pstruct.Struct{
			Fields: map[string]*pstruct.Value{},
		}
	}

	metadata.FilterMetadata[IstioMetadataKey].Fields[key] = val
}

func shouldAddTelemetryLabel(workloadName string) bool {
	return features.EndpointTelemetryLabel && (workloadName != "")
}

// IsAllowAnyOutbound checks if allow_any is enabled for outbound traffic
func IsAllowAnyOutbound(node *model.Proxy) bool {
	return node.SidecarScope != nil &&
		node.SidecarScope.OutboundTrafficPolicy != nil &&
		node.SidecarScope.OutboundTrafficPolicy.Mode == networking.OutboundTrafficPolicy_ALLOW_ANY
}

// BuildStatPrefix builds a stat prefix based on the stat pattern.
func BuildStatPrefix(statPattern string, host string, subset string, port *model.Port, attributes model.ServiceAttributes) string {
	prefix := strings.ReplaceAll(statPattern, serviceStatPattern, shortHostName(host, attributes))
	prefix = strings.ReplaceAll(prefix, serviceFQDNStatPattern, host)
	prefix = strings.ReplaceAll(prefix, subsetNameStatPattern, subset)
	prefix = strings.ReplaceAll(prefix, servicePortStatPattern, strconv.Itoa(port.Port))
	prefix = strings.ReplaceAll(prefix, servicePortNameStatPattern, port.Name)
	return prefix
}

// shortHostName constructs the name from kubernetes hosts based on attributes (name and namespace).
// For other hosts like VMs, this method does not do any thing - just returns the passed in host as is.
func shortHostName(host string, attributes model.ServiceAttributes) string {
	if attributes.ServiceRegistry == string(serviceregistry.Kubernetes) {
		return attributes.Name + "." + attributes.Namespace
	}
	return host
}

func StringToExactMatch(in []string) []*matcher.StringMatcher {
	if len(in) == 0 {
		return nil
	}
	res := make([]*matcher.StringMatcher, 0, len(in))
	for _, s := range in {
		res = append(res, &matcher.StringMatcher{
			MatchPattern: &matcher.StringMatcher_Exact{Exact: s},
		})
	}
	return res
}

func StringToPrefixMatch(in []string) []*matcher.StringMatcher {
	if len(in) == 0 {
		return nil
	}
	res := make([]*matcher.StringMatcher, 0, len(in))
	for _, s := range in {
		res = append(res, &matcher.StringMatcher{
			MatchPattern: &matcher.StringMatcher_Prefix{Prefix: s},
		})
	}
	return res
}

func StringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func UInt32SliceEqual(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func CidrRangeSliceEqual(a, b []*core.CidrRange) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i].GetAddressPrefix() != b[i].GetAddressPrefix() || a[i].GetPrefixLen().GetValue() != b[i].GetPrefixLen().GetValue() {
			return false
		}
	}

	return true
}

// meshconfig ForwardClientCertDetails and the Envoy config enum are off by 1
// due to the UNDEFINED in the meshconfig ForwardClientCertDetails
func MeshConfigToEnvoyForwardClientCertDetails(c meshconfig.Topology_ForwardClientCertDetails) http_conn.HttpConnectionManager_ForwardClientCertDetails {
	return http_conn.HttpConnectionManager_ForwardClientCertDetails(c - 1)
}

// MultiErrorFormat provides a format for multierrors. This matches the default format, but if there
// is only one error we will not expand to multiple lines.
func MultiErrorFormat() multierror.ErrorFormatFunc {
	return func(es []error) string {
		if len(es) == 1 {
			return es[0].Error()
		}

		points := make([]string, len(es))
		for i, err := range es {
			points[i] = fmt.Sprintf("* %s", err)
		}

		return fmt.Sprintf(
			"%d errors occurred:\n\t%s\n\n",
			len(es), strings.Join(points, "\n\t"))
	}
}

// ByteCount returns a human readable byte format
// Inspired by https://yourbasic.org/golang/formatting-byte-size-to-human-readable-format/
func ByteCount(b int) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}
