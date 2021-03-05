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

package v1alpha3

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	accesslog "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	ratelimit "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	thrift_ratelimit "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/filters/ratelimit/v3"
	thrift "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"
	auth "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	tracing "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
	xdstype "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/wrappers"

	meshconfig "istio.io/api/mesh/v1alpha1"
	networking "istio.io/api/networking/v1alpha3"
	"istio.io/istio/pilot/pkg/features"
	"istio.io/istio/pilot/pkg/model"
	istionetworking "istio.io/istio/pilot/pkg/networking"
	"istio.io/istio/pilot/pkg/networking/plugin"
	"istio.io/istio/pilot/pkg/networking/util"
	"istio.io/istio/pilot/pkg/serviceregistry"
	xdsfilters "istio.io/istio/pilot/pkg/xds/filters"
	"istio.io/istio/pkg/config"
	"istio.io/istio/pkg/config/constants"
	"istio.io/istio/pkg/config/host"
	"istio.io/istio/pkg/config/protocol"
	"istio.io/istio/pkg/proto"
	"istio.io/istio/pkg/util/gogo"
	"istio.io/istio/pkg/util/protomarshal"
	"istio.io/pkg/log"
	"istio.io/pkg/monitoring"
)

const (
	NoConflict = iota
	// Incoming HTTP existing HTTP
	HTTPOverHTTP
	// Incoming HTTP existing TCP
	HTTPOverTCP
	// Incoming HTTP existing AUTO
	HTTPOverAuto
	// Incoming TCP existing HTTP
	TCPOverHTTP
	// Incoming TCP existing TCP
	TCPOverTCP
	// Incoming TCP existing AUTO
	TCPOverAuto
	// Incoming AUTO existing HTTP
	AutoOverHTTP
	// Incoming AUTO existing TCP
	AutoOverTCP
	// Incoming AUTO existing AUTO
	AutoOverAuto
)

const (
	// RDSHttpProxy is the special name for HTTP PROXY route
	RDSHttpProxy = "http_proxy"

	// VirtualOutboundListenerName is the name for traffic capture listener
	VirtualOutboundListenerName = "virtualOutbound"

	// VirtualOutboundCatchAllTCPFilterChainName is the name of the catch all tcp filter chain
	VirtualOutboundCatchAllTCPFilterChainName = "virtualOutbound-catchall-tcp"

	// VirtualOutboundCatchAllTCPFilterChainName is the name of the filter chain to blackhole undesired traffic
	VirtualOutboundBlackholeFilterChainName = "virtualOutbound-blackhole"
	// VirtualInboundCatchAllTCPFilterChainName is the name of the filter chain to blackhole undesired traffic
	VirtualInboundBlackholeFilterChainName = "virtualInbound-blackhole"

	// VirtualInboundListenerName is the name for traffic capture listener
	VirtualInboundListenerName = "virtualInbound"

	// virtualInboundCatchAllHTTPFilterChainName is the name of the catch all http filter chain
	virtualInboundCatchAllHTTPFilterChainName = "virtualInbound-catchall-http"

	// WildcardAddress binds to all IP addresses
	WildcardAddress = "0.0.0.0"

	// WildcardIPv6Address binds to all IPv6 addresses
	WildcardIPv6Address = "::"

	// LocalhostAddress for local binding
	LocalhostAddress = "127.0.0.1"

	// LocalhostIPv6Address for local binding
	LocalhostIPv6Address = "::1"

	// ProxyInboundListenPort is the port on which all inbound traffic to the pod/vm will be captured to
	// TODO: allow configuration through mesh config
	ProxyInboundListenPort = 15006

	ThriftRLSDefaultTimeoutMS = 50
)

type FilterChainMatchOptions struct {
	// Application protocols of the filter chain match
	ApplicationProtocols []string
	// Transport protocol of the filter chain match. "tls" or empty
	TransportProtocol string
	// Filter chain protocol. HTTP for HTTP proxy and TCP for TCP proxy
	Protocol istionetworking.ListenerProtocol
}

// A set of pre-allocated variables related to protocol sniffing logic for
// propagating the ALPN to upstreams
var (
	// These are sniffed by the HTTP Inspector in the outbound listener
	// We need to forward these ALPNs to upstream so that the upstream can
	// properly use a HTTP or TCP listener
	plaintextHTTPALPNs = []string{"http/1.0", "http/1.1", "h2c"}
	mtlsHTTPALPNs      = []string{"istio-http/1.0", "istio-http/1.1", "istio-h2"}

	mtlsTCPWithMxcALPNs = []string{"istio-peer-exchange", "istio"}

	// ALPN used for TCP Metadata Exchange.
	tcpMxcALPN = "istio-peer-exchange"

	// Same as inboundPermissiveFilterChainMatchOptions except for following case:
	// FCM 3: ALPN [istio-peer-exchange, istio] Transport protocol: tls            --> TCP traffic from sidecar over TLS
	inboundPermissiveFilterChainMatchWithMxcOptions = []FilterChainMatchOptions{
		{
			// client side traffic was detected as HTTP by the outbound listener, sent over mTLS
			ApplicationProtocols: mtlsHTTPALPNs,
			// If client sends mTLS traffic, transport protocol will be set by the TLS inspector
			TransportProtocol: xdsfilters.TLSTransportProtocol,
			Protocol:          istionetworking.ListenerProtocolHTTP,
		},
		{
			// client side traffic was detected as HTTP by the outbound listener, sent out as plain text
			ApplicationProtocols: plaintextHTTPALPNs,
			// No transport protocol match as this filter chain (+match) will be used for plain text connections
			Protocol:          istionetworking.ListenerProtocolHTTP,
			TransportProtocol: xdsfilters.RawBufferTransportProtocol,
		},
		{
			// client side traffic could not be identified by the outbound listener, but sent over mTLS
			ApplicationProtocols: mtlsTCPWithMxcALPNs,
			// If client sends mTLS traffic, transport protocol will be set by the TLS inspector
			TransportProtocol: xdsfilters.TLSTransportProtocol,
			Protocol:          istionetworking.ListenerProtocolTCP,
		},
		{
			// client side traffic could not be identified by the outbound listener, sent over plaintext
			// or it could be that the client has no sidecar. In this case, this filter chain is simply
			// receiving plaintext TCP traffic.
			Protocol:          istionetworking.ListenerProtocolTCP,
			TransportProtocol: xdsfilters.RawBufferTransportProtocol,
		},
		{
			// client side traffic could not be identified by the outbound listener, sent over one-way
			// TLS (HTTPS for example) by the downstream application.
			// or it could be that the client has no sidecar, and it is directly making a HTTPS connection to
			// this sidecar. In this case, this filter chain is receiving plaintext one-way TLS traffic. The TLS
			// inspector would detect this as TLS traffic [not necessarily mTLS]. But since there is no ALPN to match,
			// this filter chain match will treat the traffic as just another TCP proxy.
			TransportProtocol: xdsfilters.TLSTransportProtocol,
			Protocol:          istionetworking.ListenerProtocolTCP,
		},
	}

	inboundStrictFilterChainMatchOptions = []FilterChainMatchOptions{
		{
			// client side traffic was detected as HTTP by the outbound listener.
			// If we are in strict mode, we will get mTLS HTTP ALPNS only.
			ApplicationProtocols: mtlsHTTPALPNs,
			Protocol:             istionetworking.ListenerProtocolHTTP,
			TransportProtocol:    xdsfilters.TLSTransportProtocol,
		},
		{
			// Could not detect traffic on the client side. Server side has no mTLS.
			Protocol:          istionetworking.ListenerProtocolTCP,
			TransportProtocol: xdsfilters.TLSTransportProtocol,
		},
	}

	inboundPlainTextFilterChainMatchOptions = []FilterChainMatchOptions{
		{
			ApplicationProtocols: plaintextHTTPALPNs,
			Protocol:             istionetworking.ListenerProtocolHTTP,
			TransportProtocol:    xdsfilters.RawBufferTransportProtocol,
		},
		{
			// Could not detect traffic on the client side. Server side has no mTLS.
			Protocol:          istionetworking.ListenerProtocolTCP,
			TransportProtocol: xdsfilters.RawBufferTransportProtocol,
		},
	}

	// pilotTraceSamplingEnv is value of PILOT_TRACE_SAMPLING env bounded
	// by [0.0, 100.0]; if outside the range it is set to 1.0
	pilotTraceSamplingEnv = getPilotRandomSamplingEnv()

	emptyFilterChainMatch = &listener.FilterChainMatch{}
)

// TODO: gauge should be reset on refresh, not the best way to represent errors but better
// than nothing.
// TODO: add dimensions - namespace of rule, service, rule name
var invalidOutboundListeners = monitoring.NewGauge(
	"pilot_invalid_out_listeners",
	"Number of invalid outbound listeners.",
)

func init() {
	monitoring.MustRegister(invalidOutboundListeners)
}

// BuildListeners produces a list of listeners and referenced clusters for all proxies
func (configgen *ConfigGeneratorImpl) BuildListeners(node *model.Proxy,
	push *model.PushContext) []*listener.Listener {
	builder := NewListenerBuilder(node, push)

	switch node.Type {
	case model.SidecarProxy:
		builder = configgen.buildSidecarListeners(builder)
	case model.Router:
		builder = configgen.buildGatewayListeners(builder)
	}

	builder.patchListeners()
	return builder.getListeners()
}

// buildSidecarListeners produces a list of listeners for sidecar proxies
func (configgen *ConfigGeneratorImpl) buildSidecarListeners(builder *ListenerBuilder) *ListenerBuilder {
	if builder.push.Mesh.ProxyListenPort > 0 {
		// Any build order change need a careful code review
		builder.buildSidecarInboundListeners(configgen).
			buildSidecarOutboundListeners(configgen).
			buildHTTPProxyListener(configgen).
			buildVirtualOutboundListener(configgen).
			buildVirtualInboundListener(configgen)
	}

	return builder
}

// buildSidecarInboundListeners creates listeners for the server-side (inbound)
// configuration for co-located service proxyInstances.
func (configgen *ConfigGeneratorImpl) buildSidecarInboundListeners(
	node *model.Proxy,
	push *model.PushContext) []*listener.Listener {
	var listeners []*listener.Listener
	listenerMap := make(map[int]*inboundListenerEntry)

	sidecarScope := node.SidecarScope
	noneMode := node.GetInterceptionMode() == model.InterceptionNone

	if !sidecarScope.HasCustomIngressListeners {
		// There is no user supplied sidecarScope for this namespace
		// Construct inbound listeners in the usual way by looking at the ports of the service instances
		// attached to the proxy
		// We should not create inbound listeners in NONE mode based on the service instances
		// Doing so will prevent the workloads from starting as they would be listening on the same port
		// Users are required to provide the sidecar config to define the inbound listeners
		if noneMode {
			return nil
		}

		// inbound connections/requests are redirected to the endpoint address but appear to be sent
		// to the service address.
		//
		// Protocol sniffing for inbound listener.
		// If there is no ingress listener, for each service instance, the listener port protocol is determined
		// by the service port protocol. If user doesn't specify the service port protocol, the listener will
		// be generated using protocol sniffing.
		// For example, the set of service instances
		//      --> Endpoint
		//              Address:Port 172.16.0.1:1111
		//              ServicePort  80|HTTP
		//      --> Endpoint
		//              Address:Port 172.16.0.1:2222
		//              ServicePort  8888|TCP
		//      --> Endpoint
		//              Address:Port 172.16.0.1:3333
		//              ServicePort 9999|Unknown
		//
		//	The pilot will generate three listeners, the last one will use protocol sniffing.
		//
		for _, instance := range node.ServiceInstances {
			endpoint := instance.Endpoint
			// Inbound listeners will be aggregated into a single virtual listener (port 15006)
			// As a result, we don't need to worry about binding to the endpoint IP; we already know
			// all traffic for these listeners is inbound.
			// TODO: directly build filter chains rather than translating listeneners to filter chains
			wildcard, _ := getActualWildcardAndLocalHost(node)
			bind := wildcard

			// Local service instances can be accessed through one of three
			// addresses: localhost, endpoint IP, and service
			// VIP. Localhost bypasses the proxy and doesn't need any TCP
			// route config. Endpoint IP is handled below and Service IP is handled
			// by outbound routes.
			// Traffic sent to our service VIP is redirected by remote
			// services' kubeproxy to our specific endpoint IP.
			port := *instance.ServicePort
			port.Port = int(endpoint.EndpointPort)
			listenerOpts := buildListenerOpts{
				push:       push,
				proxy:      node,
				bind:       bind,
				port:       &port,
				bindToPort: false,
			}

			pluginParams := &plugin.InputParams{
				ListenerProtocol: istionetworking.ModelProtocolToListenerProtocol(instance.ServicePort.Protocol, core.TrafficDirection_INBOUND),
				Node:             node,
				ServiceInstance:  instance,
				Push:             push,
			}

			if l := configgen.buildSidecarInboundListenerForPortOrUDS(node, listenerOpts, pluginParams, listenerMap); l != nil {
				listeners = append(listeners, l)
			}
		}
		return listeners

	}

	for _, ingressListener := range sidecarScope.Sidecar.Ingress {
		// determine the bindToPort setting for listeners. Validation guarantees that these are all IP listeners.
		bindToPort := false
		if noneMode {
			// do not care what the listener's capture mode setting is. The proxy does not use iptables
			bindToPort = true
		} else if ingressListener.CaptureMode == networking.CaptureMode_NONE {
			// proxy uses iptables redirect or tproxy. IF mode is not set
			// for older proxies, it defaults to iptables redirect.  If the
			// listener's capture mode specifies NONE, then the proxy wants
			// this listener alone to be on a physical port. If the
			// listener's capture mode is default, then its same as
			// iptables i.e. bindToPort is false.
			bindToPort = true
		}

		listenPort := &model.Port{
			Port:     int(ingressListener.Port.Number),
			Protocol: protocol.Parse(ingressListener.Port.Protocol),
			Name:     ingressListener.Port.Name,
		}

		bind := ingressListener.Bind
		if len(bind) == 0 {
			// User did not provide one. Pick the proxy's IP or wildcard inbound listener.
			bind = getSidecarInboundBindIP(node)
		}

		instance := configgen.findOrCreateServiceInstance(node.ServiceInstances, ingressListener,
			sidecarScope.Name, sidecarScope.Namespace)

		listenerOpts := buildListenerOpts{
			push:       push,
			proxy:      node,
			bind:       bind,
			port:       listenPort,
			bindToPort: bindToPort,
		}

		// we don't need to set other fields of the endpoint here as
		// the consumers of this service instance (listener/filter chain constructors)
		// are simply looking for the service port and the service associated with the instance.
		instance.ServicePort = listenPort

		// Validation ensures that the protocol specified in Sidecar.ingress
		// is always a valid known protocol
		pluginParams := &plugin.InputParams{
			ListenerProtocol: istionetworking.ModelProtocolToListenerProtocol(listenPort.Protocol,
				core.TrafficDirection_INBOUND),
			Node:            node,
			ServiceInstance: instance,
			Push:            push,
		}

		if l := configgen.buildSidecarInboundListenerForPortOrUDS(node, listenerOpts, pluginParams, listenerMap); l != nil {
			listeners = append(listeners, l)
		}
	}

	return listeners
}

func (configgen *ConfigGeneratorImpl) buildSidecarInboundHTTPListenerOptsForPortOrUDS(node *model.Proxy,
	pluginParams *plugin.InputParams, clusterName string) *httpListenerOpts {
	if clusterName == "" {
		// In case of unix domain sockets, the service port will be 0. So use the port name to distinguish the
		// inbound listeners that a user specifies in Sidecar. Otherwise, all inbound clusters will be the same.
		// We use the port name as the subset in the inbound cluster for differentiation. Its fine to use port
		// names here because the inbound clusters are not referred to anywhere in the API, unlike the outbound
		// clusters and these are static endpoint clusters used only for sidecar (proxy -> app)
		clusterName = model.BuildInboundSubsetKey(int(pluginParams.ServiceInstance.Endpoint.EndpointPort))
	}

	httpOpts := &httpListenerOpts{
		routeConfig: configgen.buildSidecarInboundHTTPRouteConfig(pluginParams.Node,
			pluginParams.Push, pluginParams.ServiceInstance, clusterName),
		rds:              "", // no RDS for inbound traffic
		useRemoteAddress: false,
		connectionManager: &hcm.HttpConnectionManager{
			// Append and forward client cert to backend.
			ForwardClientCertDetails: hcm.HttpConnectionManager_APPEND_FORWARD,
			SetCurrentClientCertDetails: &hcm.HttpConnectionManager_SetCurrentClientCertDetails{
				Subject: proto.BoolTrue,
				Uri:     true,
				Dns:     true,
			},
			ServerName: EnvoyServerName,
		},
	}
	// See https://github.com/grpc/grpc-web/tree/master/net/grpc/gateway/examples/helloworld#configure-the-proxy
	if pluginParams.ServiceInstance.ServicePort.Protocol.IsHTTP2() {
		httpOpts.connectionManager.Http2ProtocolOptions = &core.Http2ProtocolOptions{}
		if pluginParams.ServiceInstance.ServicePort.Protocol == protocol.GRPCWeb {
			httpOpts.addGRPCWebFilter = true
		}
	}

	if features.HTTP10 || node.Metadata.HTTP10 == "1" {
		httpOpts.connectionManager.HttpProtocolOptions = &core.Http1ProtocolOptions{
			AcceptHttp_10: true,
		}
	}

	return httpOpts
}

func (configgen *ConfigGeneratorImpl) buildSidecarThriftListenerOptsForPortOrUDS(pluginParams *plugin.InputParams) *thriftListenerOpts {
	// In case of unix domain sockets, the service port will be 0. So use the port name to distinguish the
	// inbound listeners that a user specifies in Sidecar. Otherwise, all inbound clusters will be the same.
	// We use the port name as the subset in the inbound cluster for differentiation. Its fine to use port
	// names here because the inbound clusters are not referred to anywhere in the API, unlike the outbound
	// clusters and these are static endpoint clusters used only for sidecar (proxy -> app)
	clusterName := model.BuildInboundSubsetKey(int(pluginParams.ServiceInstance.Endpoint.EndpointPort))

	thriftOpts := &thriftListenerOpts{
		transport:   thrift.TransportType_AUTO_TRANSPORT,
		protocol:    thrift.ProtocolType_AUTO_PROTOCOL,
		routeConfig: configgen.buildSidecarThriftRouteConfig(clusterName, pluginParams.Push.Mesh.ThriftConfig.RateLimitUrl),
	}

	return thriftOpts
}

// buildSidecarInboundListenerForPortOrUDS creates a single listener on the server-side (inbound)
// for a given port or unix domain socket
func (configgen *ConfigGeneratorImpl) buildSidecarInboundListenerForPortOrUDS(node *model.Proxy, listenerOpts buildListenerOpts,
	pluginParams *plugin.InputParams, listenerMap map[int]*inboundListenerEntry) *listener.Listener {
	// Local service instances can be accessed through one of four addresses:
	// unix domain socket, localhost, endpoint IP, and service
	// VIP. Localhost bypasses the proxy and doesn't need any TCP
	// route config. Endpoint IP is handled below and Service IP is handled
	// by outbound routes. Traffic sent to our service VIP is redirected by
	// remote services' kubeproxy to our specific endpoint IP.

	listenerOpts.class = ListenerClassSidecarInbound

	if old, exists := listenerMap[listenerOpts.port.Port]; exists {
		// If we already setup this hostname, its not a conflict. This may just mean there are multiple
		// IPs for this hostname
		if old.instanceHostname != pluginParams.ServiceInstance.Service.Hostname {
			// For sidecar specified listeners, the caller is expected to supply a dummy service instance
			// with the right port and a hostname constructed from the sidecar config's name+namespace
			// TODO everything in inbound listener is now workload oriented. We should no longer have listener conflicts.
			pluginParams.Push.AddMetric(model.ProxyStatusConflictInboundListener, pluginParams.Node.ID, pluginParams.Node.ID,
				fmt.Sprintf("Conflicting inbound listener:%d. existing: %s, incoming: %s", listenerOpts.port.Port,
					old.instanceHostname, pluginParams.ServiceInstance.Service.Hostname))
		}
		// Skip building listener for the same port
		return nil
	}

	var allChains []istionetworking.FilterChain
	for _, p := range configgen.Plugins {
		chains := p.OnInboundFilterChains(pluginParams)
		allChains = append(allChains, chains...)
	}

	tlsInspectorEnabled := false
	hasTLSContext := false
allChainsLabel:
	for _, c := range allChains {
		for _, lf := range c.ListenerFilters {
			if lf.Name == wellknown.TlsInspector {
				tlsInspectorEnabled = true
				break allChainsLabel
			}
		}

		hasTLSContext = hasTLSContext || c.TLSContext != nil
	}

	var filterChainMatchOption []FilterChainMatchOptions
	// Detect protocol by sniffing and double the filter chain
	if pluginParams.ListenerProtocol == istionetworking.ListenerProtocolAuto {
		allChains = append(allChains, allChains...)
		if tlsInspectorEnabled {
			allChains = append(allChains, istionetworking.FilterChain{})
			filterChainMatchOption = inboundPermissiveFilterChainMatchWithMxcOptions
		} else {
			if hasTLSContext {
				filterChainMatchOption = inboundStrictFilterChainMatchOptions
			} else {
				filterChainMatchOption = inboundPlainTextFilterChainMatchOptions
			}
		}
		listenerOpts.needHTTPInspector = true
	} else if pluginParams.ListenerProtocol == istionetworking.ListenerProtocolTCP {
		// When we are in permissive mode, we need a third filter chain to handle how envoy treats filter
		// chain matching
		// There will be one mtls filter chain, then two identical filter chains with transport=raw and transport=tls.
		// TODO(https://github.com/istio/istio/issues/29588) clean this up
		if tlsInspectorEnabled {
			allChains = append(allChains, istionetworking.FilterChain{
				FilterChainMatch: &listener.FilterChainMatch{TransportProtocol: xdsfilters.TLSTransportProtocol},
			})
		}
	}

	// name all the filter chains

	for id, chain := range allChains {
		var httpOpts *httpListenerOpts
		var thriftOpts *thriftListenerOpts
		var tcpNetworkFilters []*listener.Filter
		var filterChainMatch *listener.FilterChainMatch

		if chain.FilterChainMatch == nil {
			chain.FilterChainMatch = &listener.FilterChainMatch{}
		}
		if chain.FilterChainMatch.TransportProtocol == "" {
			if chain.TLSContext == nil {
				chain.FilterChainMatch.TransportProtocol = xdsfilters.RawBufferTransportProtocol
			} else {
				chain.FilterChainMatch.TransportProtocol = xdsfilters.TLSTransportProtocol
			}
		}
		switch pluginParams.ListenerProtocol {
		case istionetworking.ListenerProtocolHTTP:
			filterChainMatch = chain.FilterChainMatch
			if len(filterChainMatch.ApplicationProtocols) > 0 {
				// This is the filter chain used by permissive mTLS. Append mtlsHTTPALPNs as the client side will
				// override the ALPN with mtlsHTTPALPNs.
				// TODO: This should move to authN code instead of us appending additional ALPNs here.
				filterChainMatch.ApplicationProtocols = append(filterChainMatch.ApplicationProtocols, mtlsHTTPALPNs...)
			}

			httpOpts = configgen.buildSidecarInboundHTTPListenerOptsForPortOrUDS(node, pluginParams, "")

		case istionetworking.ListenerProtocolThrift:
			filterChainMatch = chain.FilterChainMatch
			thriftOpts = configgen.buildSidecarThriftListenerOptsForPortOrUDS(pluginParams)

		case istionetworking.ListenerProtocolTCP:
			filterChainMatch = chain.FilterChainMatch
			tcpNetworkFilters = buildInboundNetworkFilters(pluginParams.Push, pluginParams.ServiceInstance, node)

		case istionetworking.ListenerProtocolAuto:
			// Make sure id is not out of boundary of filterChainMatchOption
			if filterChainMatchOption == nil || len(filterChainMatchOption) <= id {
				continue
			}

			// TODO(yxue) avoid bypassing authN using TCP
			// Build filter chain options for listener configured with protocol sniffing
			fcm := &listener.FilterChainMatch{}
			if chain.FilterChainMatch != nil {
				fcm = protomarshal.ShallowCopy(chain.FilterChainMatch).(*listener.FilterChainMatch)
			}
			fcm.ApplicationProtocols = filterChainMatchOption[id].ApplicationProtocols
			fcm.TransportProtocol = filterChainMatchOption[id].TransportProtocol
			filterChainMatch = fcm
			if filterChainMatchOption[id].Protocol == istionetworking.ListenerProtocolHTTP {
				httpOpts = configgen.buildSidecarInboundHTTPListenerOptsForPortOrUDS(node, pluginParams, "")
				if chain.TLSContext != nil && chain.TLSContext.CommonTlsContext != nil {
					chain.TLSContext.CommonTlsContext.AlpnProtocols = dropAlpnFromList(
						chain.TLSContext.CommonTlsContext.AlpnProtocols, tcpMxcALPN)
				}
			} else {
				tcpNetworkFilters = buildInboundNetworkFilters(pluginParams.Push, pluginParams.ServiceInstance, node)
			}
		default:
			log.Warnf("Unsupported inbound protocol %v for port %#v", pluginParams.ListenerProtocol,
				pluginParams.ServiceInstance.ServicePort)
			return nil
		}

		listenerOpts.filterChainOpts = append(listenerOpts.filterChainOpts, &filterChainOpts{
			httpOpts:        httpOpts,
			thriftOpts:      thriftOpts,
			networkFilters:  tcpNetworkFilters,
			tlsContext:      chain.TLSContext,
			match:           filterChainMatch,
			listenerFilters: chain.ListenerFilters,
		})
	}

	// call plugins
	l := buildListener(listenerOpts, core.TrafficDirection_INBOUND)

	mutable := &istionetworking.MutableObjects{
		Listener:     l,
		FilterChains: getPluginFilterChain(listenerOpts),
	}
	for _, p := range configgen.Plugins {
		if err := p.OnInboundListener(pluginParams, mutable); err != nil {
			log.Warn(err.Error())
		}
	}
	// Filters are serialized one time into an opaque struct once we have the complete list.
	if err := buildCompleteFilterChain(mutable, listenerOpts); err != nil {
		log.Warn("buildSidecarInboundListeners ", err.Error())
		return nil
	}

	listenerMap[listenerOpts.port.Port] = &inboundListenerEntry{
		instanceHostname: pluginParams.ServiceInstance.Service.Hostname,
	}
	return mutable.Listener
}

type inboundListenerEntry struct {
	instanceHostname host.Name // could be empty if generated via Sidecar CRD
}

type outboundListenerEntry struct {
	services    []*model.Service
	servicePort *model.Port
	bind        string
	listener    *listener.Listener
	locked      bool
	protocol    protocol.Instance
}

func protocolName(p protocol.Instance) string {
	switch istionetworking.ModelProtocolToListenerProtocol(p, core.TrafficDirection_OUTBOUND) {
	case istionetworking.ListenerProtocolHTTP:
		return "HTTP"
	case istionetworking.ListenerProtocolTCP:
		return "TCP"
	case istionetworking.ListenerProtocolThrift:
		return "THRIFT"
	default:
		return "UNKNOWN"
	}
}

type outboundListenerConflict struct {
	metric          monitoring.Metric
	node            *model.Proxy
	listenerName    string
	currentProtocol protocol.Instance
	currentServices []*model.Service
	newHostname     host.Name
	newProtocol     protocol.Instance
}

func (c outboundListenerConflict) addMetric(metrics model.Metrics) {
	currentHostnames := make([]string, len(c.currentServices))
	for i, s := range c.currentServices {
		currentHostnames[i] = string(s.Hostname)
	}
	concatHostnames := strings.Join(currentHostnames, ",")
	metrics.AddMetric(c.metric,
		c.listenerName,
		c.node.ID,
		fmt.Sprintf("Listener=%s Accepted%s=%s Rejected%s=%s %sServices=%d",
			c.listenerName,
			protocolName(c.currentProtocol),
			concatHostnames,
			protocolName(c.newProtocol),
			c.newHostname,
			protocolName(c.currentProtocol),
			len(c.currentServices)))
}

// buildSidecarOutboundListeners generates http and tcp listeners for
// outbound connections from the proxy based on the sidecar scope associated with the proxy.
func (configgen *ConfigGeneratorImpl) buildSidecarOutboundListeners(node *model.Proxy,
	push *model.PushContext) []*listener.Listener {
	noneMode := node.GetInterceptionMode() == model.InterceptionNone

	actualWildcard, actualLocalHostAddress := getActualWildcardAndLocalHost(node)

	var tcpListeners, httpListeners []*listener.Listener
	// For conflict resolution
	listenerMap := make(map[string]*outboundListenerEntry)

	// The sidecarConfig if provided could filter the list of
	// services/virtual services that we need to process. It could also
	// define one or more listeners with specific ports. Once we generate
	// listeners for these user specified ports, we will auto generate
	// configs for other ports if and only if the sidecarConfig has an
	// egressListener on wildcard port.
	//
	// Validation will ensure that we have utmost one wildcard egress listener
	// occurring in the end

	// Add listeners based on the config in the sidecar.EgressListeners if
	// no Sidecar CRD is provided for this config namespace,
	// push.SidecarScope will generate a default catch all egress listener.
	for _, egressListener := range node.SidecarScope.EgressListeners {

		services := egressListener.Services()
		virtualServices := egressListener.VirtualServices()

		// determine the bindToPort setting for listeners
		bindToPort := false
		if noneMode {
			// do not care what the listener's capture mode setting is. The proxy does not use iptables
			bindToPort = true
		} else if egressListener.IstioListener != nil {
			if egressListener.IstioListener.CaptureMode == networking.CaptureMode_NONE {
				// proxy uses iptables redirect or tproxy. IF mode is not set
				// for older proxies, it defaults to iptables redirect.  If the
				// listener's capture mode specifies NONE, then the proxy wants
				// this listener alone to be on a physical port. If the
				// listener's capture mode is default, then its same as
				// iptables i.e. bindToPort is false.
				bindToPort = true
			} else if strings.HasPrefix(egressListener.IstioListener.Bind, model.UnixAddressPrefix) {
				// If the bind is a Unix domain socket, set bindtoPort to true as it makes no
				// sense to have ORIG_DST listener for unix domain socket listeners.
				bindToPort = true
			}
		}

		if egressListener.IstioListener != nil &&
			egressListener.IstioListener.Port != nil {
			// We have a non catch all listener on some user specified port
			// The user specified port may or may not match a service port.
			// If it does not match any service port and the service has only
			// one port, then we pick a default service port. If service has
			// multiple ports, we expect the user to provide a virtualService
			// that will route to a proper Service.

			listenPort := &model.Port{
				Port:     int(egressListener.IstioListener.Port.Number),
				Protocol: protocol.Parse(egressListener.IstioListener.Port.Protocol),
				Name:     egressListener.IstioListener.Port.Name,
			}

			// If capture mode is NONE i.e., bindToPort is true, and
			// Bind IP + Port is specified, we will bind to the specified IP and Port.
			// This specified IP is ideally expected to be a loopback IP.
			//
			// If capture mode is NONE i.e., bindToPort is true, and
			// only Port is specified, we will bind to the default loopback IP
			// 127.0.0.1 and the specified Port.
			//
			// If capture mode is NONE, i.e., bindToPort is true, and
			// only Bind IP is specified, we will bind to the specified IP
			// for each port as defined in the service registry.
			//
			// If captureMode is not NONE, i.e., bindToPort is false, then
			// we will bind to user specified IP (if any) or to the VIPs of services in
			// this egress listener.
			bind := egressListener.IstioListener.Bind
			if bind == "" {
				if bindToPort {
					bind = actualLocalHostAddress
				} else {
					bind = actualWildcard
				}
			}

			// Build ListenerOpts and PluginParams once and reuse across all Services to avoid unnecessary allocations.
			listenerOpts := buildListenerOpts{
				push:       push,
				proxy:      node,
				bind:       bind,
				port:       listenPort,
				bindToPort: bindToPort,
			}

			for _, service := range services {
				listenerOpts.service = service
				// Set service specific attributes here.
				configgen.buildSidecarOutboundListenerForPortOrUDS(node, listenerOpts, listenerMap, virtualServices, actualWildcard)
			}
		} else {
			// This is a catch all egress listener with no port. This
			// should be the last egress listener in the sidecar
			// Scope. Construct a listener for each service and service
			// port, if and only if this port was not specified in any of
			// the preceding listeners from the sidecarScope. This allows
			// users to specify a trimmed set of services for one or more
			// listeners and then add a catch all egress listener for all
			// other ports. Doing so allows people to restrict the set of
			// services exposed on one or more listeners, and avoid hard
			// port conflicts like tcp taking over http or http taking over
			// tcp, or simply specify that of all the listeners that Istio
			// generates, the user would like to have only specific sets of
			// services exposed on a particular listener.
			//
			// To ensure that we do not add anything to listeners we have
			// already generated, run through the outboundListenerEntry map and set
			// the locked bit to true.
			// buildSidecarOutboundListenerForPortOrUDS will not add/merge
			// any HTTP/TCP listener if there is already a outboundListenerEntry
			// with locked bit set to true
			for _, e := range listenerMap {
				e.locked = true
			}

			bind := ""
			if egressListener.IstioListener != nil && egressListener.IstioListener.Bind != "" {
				bind = egressListener.IstioListener.Bind
			}
			if bindToPort && bind == "" {
				bind = actualLocalHostAddress
			}

			// Build ListenerOpts and PluginParams once and reuse across all Services to avoid unnecessary allocations.
			listenerOpts := buildListenerOpts{
				push:       push,
				proxy:      node,
				bindToPort: bindToPort,
			}

			pluginParams := &plugin.InputParams{
				Node: node,
				Push: push,
			}

			for _, service := range services {
				saddress := service.GetServiceAddressForProxy(node)
				for _, servicePort := range service.Ports {
					// bind might have been modified by below code, so reset it for every Service.
					listenerOpts.bind = bind
					// port depends on servicePort.
					listenerOpts.port = servicePort
					listenerOpts.service = service

					// The listener protocol is determined by the protocol of service port.
					pluginParams.ListenerProtocol = istionetworking.ModelProtocolToListenerProtocol(servicePort.Protocol,
						core.TrafficDirection_OUTBOUND)

					// Support statefulsets/headless services with TCP ports, and empty service address field.
					// Instead of generating a single 0.0.0.0:Port listener, generate a listener
					// for each instance. HTTP services can happily reside on 0.0.0.0:PORT and use the
					// wildcard route match to get to the appropriate IP through original dst clusters.
					if features.EnableHeadlessService && bind == "" && service.Resolution == model.Passthrough &&
						saddress == constants.UnspecifiedIP && (servicePort.Protocol.IsTCP() || servicePort.Protocol.IsUnsupported()) {
						instances := push.ServiceInstancesByPort(service, servicePort.Port, nil)
						if service.Attributes.ServiceRegistry != string(serviceregistry.Kubernetes) && len(instances) == 0 && service.Attributes.LabelSelectors == nil {
							// A Kubernetes service with no endpoints means there are no endpoints at
							// all, so don't bother sending, as traffic will never work. If we did
							// send a wildcard listener, we may get into a situation where a scale
							// down leads to a listener conflict. Similarly, if we have a
							// labelSelector on the Service, then this may have endpoints not yet
							// selected or scaled down, so we skip these as well. This leaves us with
							// only a plain ServiceEntry with resolution NONE. In this case, we will
							// fallback to a wildcard listener.
							configgen.buildSidecarOutboundListenerForPortOrUDS(node, listenerOpts, listenerMap, virtualServices, actualWildcard)
							continue
						}
						for _, instance := range instances {
							// Make sure each endpoint address is a valid address
							// as service entries could have NONE resolution with label selectors for workload
							// entries (which could technically have hostnames).
							if net.ParseIP(instance.Endpoint.Address) == nil {
								continue
							}
							// Skip build outbound listener to the node itself,
							// as when app access itself by pod ip will not flow through this listener.
							// Simultaneously, it will be duplicate with inbound listener.
							if instance.Endpoint.Address == node.IPAddresses[0] {
								continue
							}
							listenerOpts.bind = instance.Endpoint.Address
							configgen.buildSidecarOutboundListenerForPortOrUDS(node, listenerOpts, listenerMap, virtualServices, actualWildcard)
						}
					} else {
						// Standard logic for headless and non headless services
						if features.EnableThriftFilter &&
							servicePort.Protocol.IsThrift() {
							listenerOpts.bind = saddress
						}
						configgen.buildSidecarOutboundListenerForPortOrUDS(node, listenerOpts, listenerMap, virtualServices, actualWildcard)
					}
				}
			}
		}
	}

	// Now validate all the listeners. Collate the tcp listeners first and then the HTTP listeners
	// TODO: This is going to be bad for caching as the order of listeners in tcpListeners or httpListeners is not
	// guaranteed.
	for _, l := range listenerMap {
		if l.servicePort.Protocol.IsTCP() {
			tcpListeners = append(tcpListeners, l.listener)
		} else {
			httpListeners = append(httpListeners, l.listener)
		}
	}
	tcpListeners = append(tcpListeners, httpListeners...)
	// Build pass through filter chains now that all the non-passthrough filter chains are ready.
	for _, listener := range tcpListeners {
		configgen.appendListenerFallthroughRouteForCompleteListener(listener, node, push)
	}
	removeListenerFilterTimeout(tcpListeners)
	return tcpListeners
}

func (configgen *ConfigGeneratorImpl) buildHTTPProxy(node *model.Proxy,
	push *model.PushContext) *listener.Listener {
	httpProxyPort := push.Mesh.ProxyHttpPort
	if httpProxyPort == 0 {
		return nil
	}

	// enable HTTP PROXY port if necessary; this will add an RDS route for this port
	_, listenAddress := getActualWildcardAndLocalHost(node)

	httpOpts := &core.Http1ProtocolOptions{
		AllowAbsoluteUrl: proto.BoolTrue,
	}
	if features.HTTP10 || node.Metadata.HTTP10 == "1" {
		httpOpts.AcceptHttp_10 = true
	}

	opts := buildListenerOpts{
		push:  push,
		proxy: node,
		bind:  listenAddress,
		port:  &model.Port{Port: int(httpProxyPort)},
		filterChainOpts: []*filterChainOpts{{
			httpOpts: &httpListenerOpts{
				rds:              RDSHttpProxy,
				useRemoteAddress: false,
				connectionManager: &hcm.HttpConnectionManager{
					HttpProtocolOptions: httpOpts,
				},
			},
		}},
		bindToPort:      true,
		skipUserFilters: true,
	}
	l := buildListener(opts, core.TrafficDirection_OUTBOUND)

	// TODO: plugins for HTTP_PROXY mode, envoyfilter needs another listener match for SIDECAR_HTTP_PROXY
	mutable := &istionetworking.MutableObjects{
		Listener:     l,
		FilterChains: []istionetworking.FilterChain{{}},
	}
	if err := buildCompleteFilterChain(mutable, opts); err != nil {
		log.Warn("buildHTTPProxy filter chain error  ", err.Error())
		return nil
	}
	return l
}

func (configgen *ConfigGeneratorImpl) buildSidecarOutboundHTTPListenerOptsForPortOrUDS(listenerMapKey *string,
	currentListenerEntry **outboundListenerEntry, listenerOpts *buildListenerOpts,
	listenerMap map[string]*outboundListenerEntry, actualWildcard string) (bool, []*filterChainOpts) {
	// first identify the bind if its not set. Then construct the key
	// used to lookup the listener in the conflict map.
	if len(listenerOpts.bind) == 0 { // no user specified bind. Use 0.0.0.0:Port
		listenerOpts.bind = actualWildcard
	}
	*listenerMapKey = listenerOpts.bind + ":" + strconv.Itoa(listenerOpts.port.Port)

	var exists bool
	sniffingEnabled := features.EnableProtocolSniffingForOutbound

	// Have we already generated a listener for this Port based on user
	// specified listener ports? if so, we should not add any more HTTP
	// services to the port. The user could have specified a sidecar
	// resource with one or more explicit ports and then added a catch
	// all listener, implying add all other ports as usual. When we are
	// iterating through the services for a catchAll egress listener,
	// the caller would have set the locked bit for each listener Entry
	// in the map.
	//
	// Check if this HTTP listener conflicts with an existing TCP
	// listener. We could have listener conflicts occur on unix domain
	// sockets, or on IP binds. Specifically, its common to see
	// conflicts on binds for wildcard address when a service has NONE
	// resolution type, since we collapse all HTTP listeners into a
	// single 0.0.0.0:port listener and use vhosts to distinguish
	// individual http services in that port
	if *currentListenerEntry, exists = listenerMap[*listenerMapKey]; exists {
		// NOTE: This is not a conflict. This is simply filtering the
		// services for a given listener explicitly.
		// When the user declares their own ports in Sidecar.egress
		// with some specific services on those ports, we should not
		// generate any more listeners on that port as the user does
		// not want those listeners. Protocol sniffing is not needed.
		if (*currentListenerEntry).locked {
			return false, nil
		}

		if !sniffingEnabled {
			if listenerOpts.service != nil {
				if !(*currentListenerEntry).servicePort.Protocol.IsHTTP() {
					outboundListenerConflict{
						metric:          model.ProxyStatusConflictOutboundListenerTCPOverHTTP,
						node:            listenerOpts.proxy,
						listenerName:    *listenerMapKey,
						currentServices: (*currentListenerEntry).services,
						currentProtocol: (*currentListenerEntry).servicePort.Protocol,
						newHostname:     listenerOpts.service.Hostname,
						newProtocol:     listenerOpts.port.Protocol,
					}.addMetric(listenerOpts.push)
				}

				// Skip building listener for the same http port
				(*currentListenerEntry).services = append((*currentListenerEntry).services, listenerOpts.service)
			}
			return false, nil
		}
	}

	listenerProtocol := istionetworking.ModelProtocolToListenerProtocol(listenerOpts.port.Protocol, core.TrafficDirection_OUTBOUND)

	// No conflicts. Add a http filter chain option to the listenerOpts
	var rdsName string
	if listenerOpts.port.Port == 0 {
		rdsName = listenerOpts.bind // use the UDS as a rds name
	} else {
		if listenerProtocol == istionetworking.ListenerProtocolAuto &&
			sniffingEnabled && listenerOpts.bind != actualWildcard && listenerOpts.service != nil {
			rdsName = string(listenerOpts.service.Hostname) + ":" + strconv.Itoa(listenerOpts.port.Port)
		} else {
			rdsName = strconv.Itoa(listenerOpts.port.Port)
		}
	}
	httpOpts := &httpListenerOpts{
		// Set useRemoteAddress to true for side car outbound listeners so that it picks up the localhost address of the sender,
		// which is an internal address, so that trusted headers are not sanitized. This helps to retain the timeout headers
		// such as "x-envoy-upstream-rq-timeout-ms" set by the calling application.
		useRemoteAddress: features.UseRemoteAddress,
		rds:              rdsName,
	}

	if features.HTTP10 || listenerOpts.proxy.Metadata.HTTP10 == "1" {
		httpOpts.connectionManager = &hcm.HttpConnectionManager{
			HttpProtocolOptions: &core.Http1ProtocolOptions{
				AcceptHttp_10: true,
			},
		}
	}

	return true, []*filterChainOpts{{
		httpOpts: httpOpts,
	}}
}

func (configgen *ConfigGeneratorImpl) buildSidecarOutboundThriftListenerOptsForPortOrUDS(listenerMapKey *string,
	currentListenerEntry **outboundListenerEntry, listenerOpts *buildListenerOpts,
	listenerMap map[string]*outboundListenerEntry, actualWildcard string) (bool, []*filterChainOpts) {
	// first identify the bind if its not set. Then construct the key
	// used to lookup the listener in the conflict map.
	if len(listenerOpts.bind) == 0 { // no user specified bind. Use 0.0.0.0:Port
		listenerOpts.bind = actualWildcard
	}
	*listenerMapKey = listenerKey(listenerOpts.bind, listenerOpts.port.Port)

	var exists bool

	// Have we already generated a listener for this Port based on user
	// specified listener ports? if so, we should not add any more Thrift
	// services to the port. The user could have specified a sidecar
	// resource with one or more explicit ports and then added a catch
	// all listener, implying add all other ports as usual. When we are
	// iterating through the services for a catchAll egress listener,
	// the caller would have set the locked bit for each listener Entry
	// in the map.
	//
	// Check if this Thrift listener conflicts with an existing TCP or
	// HTTP listener. We could have listener conflicts occur on unix
	// domain sockets, or on IP binds.
	if *currentListenerEntry, exists = listenerMap[*listenerMapKey]; exists {
		// NOTE: This is not a conflict. This is simply filtering the
		// services for a given listener explicitly.
		// When the user declares their own ports in Sidecar.egress
		// with some specific services on those ports, we should not
		// generate any more listeners on that port as the user does
		// not want those listeners. Protocol sniffing is not needed.
		if (*currentListenerEntry).locked {
			return false, nil
		}
	}

	// No conflicts. Add a thrift filter chain option to the listenerOpts
	clusterName := model.BuildSubsetKey(model.TrafficDirectionOutbound, "", listenerOpts.service.Hostname, listenerOpts.port.Port)
	thriftOpts := &thriftListenerOpts{
		protocol:    thrift.ProtocolType_AUTO_PROTOCOL,
		transport:   thrift.TransportType_AUTO_TRANSPORT,
		routeConfig: configgen.buildSidecarThriftRouteConfig(clusterName, listenerOpts.push.Mesh.ThriftConfig.RateLimitUrl),
	}

	return true, []*filterChainOpts{{
		thriftOpts: thriftOpts,
	}}
}

func (configgen *ConfigGeneratorImpl) buildSidecarOutboundTCPListenerOptsForPortOrUDS(destinationCIDR *string, listenerMapKey *string,
	currentListenerEntry **outboundListenerEntry, listenerOpts *buildListenerOpts, listenerMap map[string]*outboundListenerEntry,
	virtualServices []config.Config, actualWildcard string) (bool, []*filterChainOpts) {
	// first identify the bind if its not set. Then construct the key
	// used to lookup the listener in the conflict map.

	// Determine the listener address if bind is empty
	// we listen on the service VIP if and only
	// if the address is an IP address. If its a CIDR, we listen on
	// 0.0.0.0, and setup a filter chain match for the CIDR range.
	// As a small optimization, CIDRs with /32 prefix will be converted
	// into listener address so that there is a dedicated listener for this
	// ip:port. This will reduce the impact of a listener reload

	if len(listenerOpts.bind) == 0 {
		svcListenAddress := listenerOpts.service.GetServiceAddressForProxy(listenerOpts.proxy)
		// We should never get an empty address.
		// This is a safety guard, in case some platform adapter isn't doing things
		// properly
		if len(svcListenAddress) > 0 {
			if !strings.Contains(svcListenAddress, "/") {
				listenerOpts.bind = svcListenAddress
			} else {
				// Address is a CIDR. Fall back to 0.0.0.0 and
				// filter chain match
				*destinationCIDR = svcListenAddress
				listenerOpts.bind = actualWildcard
			}
		}
	}

	// could be a unix domain socket or an IP bind
	*listenerMapKey = listenerKey(listenerOpts.bind, listenerOpts.port.Port)

	var exists bool

	// Have we already generated a listener for this Port based on user
	// specified listener ports? if so, we should not add any more
	// services to the port. The user could have specified a sidecar
	// resource with one or more explicit ports and then added a catch
	// all listener, implying add all other ports as usual. When we are
	// iterating through the services for a catchAll egress listener,
	// the caller would have set the locked bit for each listener Entry
	// in the map.
	//
	// Check if this TCP listener conflicts with an existing HTTP listener
	if *currentListenerEntry, exists = listenerMap[*listenerMapKey]; exists {
		// NOTE: This is not a conflict. This is simply filtering the
		// services for a given listener explicitly.
		// When the user declares their own ports in Sidecar.egress
		// with some specific services on those ports, we should not
		// generate any more listeners on that port as the user does
		// not want those listeners. Protocol sniffing is not needed.
		if (*currentListenerEntry).locked {
			return false, nil
		}

		if !features.EnableProtocolSniffingForOutbound {
			// Check for port collisions between TCP/TLS and HTTP (or unknown). If
			// configured correctly, TCP/TLS ports may not collide. We'll
			// need to do additional work to find out if there is a
			// collision within TCP/TLS.
			// If the service port was defined as unknown. It will conflict with all other
			// protocols.
			if !(*currentListenerEntry).servicePort.Protocol.IsTCP() {
				// NOTE: While pluginParams.Service can be nil,
				// this code cannot be reached if Service is nil because a pluginParams.Service can be nil only
				// for user defined Egress listeners with ports. And these should occur in the API before
				// the wildcard egress listener. the check for the "locked" bit will eliminate the collision.
				// User is also not allowed to add duplicate ports in the egress listener
				var newHostname host.Name
				if listenerOpts.service != nil {
					newHostname = listenerOpts.service.Hostname
				} else {
					// user defined outbound listener via sidecar API
					newHostname = "sidecar-config-egress-http-listener"
				}

				// We have a collision with another TCP port. This can happen
				// for headless services, or non-k8s services that do not have
				// a VIP, or when we have two binds on a unix domain socket or
				// on same IP.  Unfortunately we won't know if this is a real
				// conflict or not until we process the VirtualServices, etc.
				// The conflict resolution is done later in this code
				outboundListenerConflict{
					metric:          model.ProxyStatusConflictOutboundListenerHTTPOverTCP,
					node:            listenerOpts.proxy,
					listenerName:    *listenerMapKey,
					currentServices: (*currentListenerEntry).services,
					currentProtocol: (*currentListenerEntry).servicePort.Protocol,
					newHostname:     newHostname,
					newProtocol:     listenerOpts.port.Protocol,
				}.addMetric(listenerOpts.push)
				return false, nil
			}
		}
	}

	meshGateway := map[string]bool{constants.IstioMeshGateway: true}
	return true, buildSidecarOutboundTCPTLSFilterChainOpts(listenerOpts.proxy,
		listenerOpts.push, virtualServices,
		*destinationCIDR, listenerOpts.service,
		listenerOpts.bind, listenerOpts.port, meshGateway)
}

// buildSidecarOutboundListenerForPortOrUDS builds a single listener and
// adds it to the listenerMap provided by the caller.  Listeners are added
// if one doesn't already exist. HTTP listeners on same port are ignored
// (as vhosts are shipped through RDS).  TCP listeners on same port are
// allowed only if they have different CIDR matches.
func (configgen *ConfigGeneratorImpl) buildSidecarOutboundListenerForPortOrUDS(node *model.Proxy, listenerOpts buildListenerOpts,
	listenerMap map[string]*outboundListenerEntry, virtualServices []config.Config, actualWildcard string) {
	var destinationCIDR string
	var listenerMapKey string
	var currentListenerEntry *outboundListenerEntry
	var ret bool
	var opts []*filterChainOpts

	listenerOpts.class = ListenerClassSidecarOutbound

	conflictType := NoConflict

	outboundSniffingEnabled := features.EnableProtocolSniffingForOutbound
	listenerPortProtocol := listenerOpts.port.Protocol
	listenerProtocol := istionetworking.ModelProtocolToListenerProtocol(listenerOpts.port.Protocol, core.TrafficDirection_OUTBOUND)

	// For HTTP_PROXY protocol defined by sidecars, just create the HTTP listener right away.
	if listenerPortProtocol == protocol.HTTP_PROXY {
		if ret, opts = configgen.buildSidecarOutboundHTTPListenerOptsForPortOrUDS(&listenerMapKey, &currentListenerEntry,
			&listenerOpts, listenerMap, actualWildcard); !ret {
			return
		}
		listenerOpts.filterChainOpts = opts
	} else {
		switch listenerProtocol {
		case istionetworking.ListenerProtocolHTTP:
			if ret, opts = configgen.buildSidecarOutboundHTTPListenerOptsForPortOrUDS(&listenerMapKey,
				&currentListenerEntry, &listenerOpts, listenerMap, actualWildcard); !ret {
				return
			}

			// Check if conflict happens
			if outboundSniffingEnabled && currentListenerEntry != nil {
				// Build HTTP listener. If current listener entry is using HTTP or protocol sniffing,
				// append the service. Otherwise (TCP), change current listener to use protocol sniffing.
				if currentListenerEntry.protocol.IsHTTP() {
					// conflictType is HTTPOverHTTP
					// In these cases, we just add the services and exit early rather than recreate an identical listener
					currentListenerEntry.services = append(currentListenerEntry.services, listenerOpts.service)
					return
				} else if currentListenerEntry.protocol.IsTCP() {
					conflictType = HTTPOverTCP
				} else {
					// conflictType is HTTPOverAuto
					// In these cases, we just add the services and exit early rather than recreate an identical listener
					currentListenerEntry.services = append(currentListenerEntry.services, listenerOpts.service)
					return
				}
			}
			// Add application protocol filter chain match to the http filter chain. The application protocol will be set by http inspector
			// Since application protocol filter chain match has been added to the http filter chain, a fall through filter chain will be
			// appended to the listener later to allow arbitrary egress TCP traffic pass through when its port is conflicted with existing
			// HTTP services, which can happen when a pod accesses a non registry service.
			if outboundSniffingEnabled {
				if listenerOpts.bind == actualWildcard {
					for _, opt := range opts {
						if opt.match == nil {
							opt.match = &listener.FilterChainMatch{}
						}

						// Support HTTP/1.0, HTTP/1.1 and HTTP/2
						opt.match.ApplicationProtocols = append(opt.match.ApplicationProtocols, plaintextHTTPALPNs...)
						opt.match.TransportProtocol = xdsfilters.RawBufferTransportProtocol
					}

					listenerOpts.needHTTPInspector = true

					// if we have a tcp fallthrough filter chain, this is no longer an HTTP listener - it
					// is instead "unsupported" (auto detected), as we have a TCP and HTTP filter chain with
					// inspection to route between them
					listenerPortProtocol = protocol.Unsupported
				}
			}
			listenerOpts.filterChainOpts = opts

		case istionetworking.ListenerProtocolThrift:
			// Hard code the service IP for outbound thrift service listeners. HTTP services
			// use RDS but the Thrift stack has no such dynamic configuration option.
			if ret, opts = configgen.buildSidecarOutboundThriftListenerOptsForPortOrUDS(&listenerMapKey,
				&currentListenerEntry, &listenerOpts, listenerMap, actualWildcard); !ret {
				return
			}

			// Protocol sniffing for thrift is not supported.
			if outboundSniffingEnabled && currentListenerEntry != nil {
				// We should not ever end up here, but log a line just in case.
				log.Errorf(
					"Protocol sniffing is not enabled for thrift, but there was a port collision. Debug info: Node: %v, ListenerEntry: %v",
					node,
					currentListenerEntry)
			}

			listenerOpts.filterChainOpts = opts

		case istionetworking.ListenerProtocolTCP:
			if ret, opts = configgen.buildSidecarOutboundTCPListenerOptsForPortOrUDS(&destinationCIDR, &listenerMapKey, &currentListenerEntry,
				&listenerOpts, listenerMap, virtualServices, actualWildcard); !ret {
				return
			}

			// Check if conflict happens
			if outboundSniffingEnabled && currentListenerEntry != nil {
				// Build TCP listener. If current listener entry is using HTTP, add a new TCP filter chain
				// If current listener is using protocol sniffing, merge the TCP filter chains.
				if currentListenerEntry.protocol.IsHTTP() {
					conflictType = TCPOverHTTP
				} else if currentListenerEntry.protocol.IsTCP() {
					conflictType = TCPOverTCP
				} else {
					conflictType = TCPOverAuto
				}
			}

			listenerOpts.filterChainOpts = opts

		case istionetworking.ListenerProtocolAuto:
			// Add tcp filter chain, build TCP filter chain first.
			if ret, opts = configgen.buildSidecarOutboundTCPListenerOptsForPortOrUDS(&destinationCIDR, &listenerMapKey, &currentListenerEntry,
				&listenerOpts, listenerMap, virtualServices, actualWildcard); !ret {
				return
			}
			listenerOpts.filterChainOpts = append(listenerOpts.filterChainOpts, opts...)

			// Add http filter chain and tcp filter chain to the listener opts
			if ret, opts = configgen.buildSidecarOutboundHTTPListenerOptsForPortOrUDS(&listenerMapKey, &currentListenerEntry,
				&listenerOpts, listenerMap, actualWildcard); !ret {
				return
			}

			// Add application protocol filter chain match to the http filter chain. The application protocol will be set by http inspector
			for _, opt := range opts {
				if opt.match == nil {
					opt.match = &listener.FilterChainMatch{}
				}

				// Support HTTP/1.0, HTTP/1.1 and HTTP/2
				opt.match.ApplicationProtocols = append(opt.match.ApplicationProtocols, plaintextHTTPALPNs...)
				opt.match.TransportProtocol = xdsfilters.RawBufferTransportProtocol
			}

			listenerOpts.filterChainOpts = append(listenerOpts.filterChainOpts, opts...)
			listenerOpts.needHTTPInspector = true

			if currentListenerEntry != nil {
				if currentListenerEntry.protocol.IsHTTP() {
					conflictType = AutoOverHTTP
				} else if currentListenerEntry.protocol.IsTCP() {
					conflictType = AutoOverTCP
				} else {
					// conflictType is AutoOverAuto
					// In these cases, we just add the services and exit early rather than recreate an identical listener
					currentListenerEntry.services = append(currentListenerEntry.services, listenerOpts.service)
					return
				}
			}

		default:
			// UDP or other protocols: no need to log, it's too noisy
			return
		}
	}

	// Lets build the new listener with the filter chains. In the end, we will
	// merge the filter chains with any existing listener on the same port/bind point
	l := buildListener(listenerOpts, core.TrafficDirection_OUTBOUND)

	mutable := &istionetworking.MutableObjects{
		Listener:     l,
		FilterChains: getPluginFilterChain(listenerOpts),
	}

	pluginParams := &plugin.InputParams{
		ListenerProtocol: listenerProtocol,
		Node:             listenerOpts.proxy,
		Push:             listenerOpts.push,
	}

	for _, p := range configgen.Plugins {
		if err := p.OnOutboundListener(pluginParams, mutable); err != nil {
			log.Warn(err.Error())
		}
	}

	// Filters are serialized one time into an opaque struct once we have the complete list.
	if err := buildCompleteFilterChain(mutable, listenerOpts); err != nil {
		log.Warn("buildSidecarOutboundListeners: ", err.Error())
		return
	}

	// If there is a TCP listener on well known port, cannot add any http filter chain
	// with the inspector as it will break for server-first protocols. Similarly,
	// if there was a HTTP listener on well known port, cannot add a tcp listener
	// with the inspector as inspector breaks all server-first protocols.
	if currentListenerEntry != nil &&
		!isConflictWithWellKnownPort(listenerOpts.port.Protocol, currentListenerEntry.protocol, conflictType) {
		log.Warnf("conflict happens on a well known port %d, incoming protocol %v, existing protocol %v, conflict type %v",
			listenerOpts.port.Port, listenerOpts.port.Protocol, currentListenerEntry.protocol, conflictType)
		return
	}

	// There are 9 types conflicts
	//    Incoming Existing
	//  1. HTTP -> HTTP
	//  2. HTTP -> TCP
	//  3. HTTP -> unknown
	//  4. TCP  -> HTTP
	//  5. TCP  -> TCP
	//  6. TCP  -> unknown
	//  7. unknown -> HTTP
	//  8. unknown -> TCP
	//  9. unknown -> unknown
	//  Type 1 can be resolved by appending service to existing services
	//  Type 2 can be resolved by merging TCP filter chain with HTTP filter chain
	//  Type 3 can be resolved by appending service to existing services
	//  Type 4 can be resolved by merging HTTP filter chain with TCP filter chain
	//  Type 5 can be resolved by merging TCP filter chains
	//  Type 6 can be resolved by merging TCP filter chains
	//  Type 7 can be resolved by appending service to existing services
	//  Type 8 can be resolved by merging TCP filter chains
	//  Type 9 can be resolved by merging TCP and HTTP filter chains

	switch conflictType {
	case NoConflict:
		if currentListenerEntry != nil {
			currentListenerEntry.listener.FilterChains = mergeTCPFilterChains(mutable.Listener.FilterChains,
				listenerOpts, listenerMapKey, listenerMap)
		} else {
			listenerMap[listenerMapKey] = &outboundListenerEntry{
				services:    []*model.Service{listenerOpts.service},
				servicePort: listenerOpts.port,
				bind:        listenerOpts.bind,
				listener:    mutable.Listener,
				protocol:    listenerPortProtocol,
			}
		}
	case HTTPOverTCP:
		// Merge HTTP filter chain to TCP filter chain
		currentListenerEntry.listener.FilterChains = mergeFilterChains(mutable.Listener.FilterChains, currentListenerEntry.listener.FilterChains)
		currentListenerEntry.protocol = protocol.Unsupported
		currentListenerEntry.listener.ListenerFilters = appendListenerFilters(currentListenerEntry.listener.ListenerFilters)
		currentListenerEntry.services = append(currentListenerEntry.services, listenerOpts.service)

	case TCPOverHTTP:
		// Merge TCP filter chain to HTTP filter chain
		currentListenerEntry.listener.FilterChains = mergeFilterChains(currentListenerEntry.listener.FilterChains, mutable.Listener.FilterChains)
		currentListenerEntry.protocol = protocol.Unsupported
		currentListenerEntry.listener.ListenerFilters = appendListenerFilters(currentListenerEntry.listener.ListenerFilters)
	case TCPOverTCP:
		// Merge two TCP filter chains. HTTP filter chain will not conflict with TCP filter chain because HTTP filter chain match for
		// HTTP filter chain is different from TCP filter chain's.
		currentListenerEntry.listener.FilterChains = mergeTCPFilterChains(mutable.Listener.FilterChains, listenerOpts, listenerMapKey, listenerMap)
	case TCPOverAuto:
		// Merge two TCP filter chains. HTTP filter chain will not conflict with TCP filter chain because HTTP filter chain match for
		// HTTP filter chain is different from TCP filter chain's.
		currentListenerEntry.listener.FilterChains = mergeTCPFilterChains(mutable.Listener.FilterChains, listenerOpts, listenerMapKey, listenerMap)

	case AutoOverHTTP:
		listenerMap[listenerMapKey] = &outboundListenerEntry{
			services:    append(currentListenerEntry.services, listenerOpts.service),
			servicePort: listenerOpts.port,
			bind:        listenerOpts.bind,
			listener:    mutable.Listener,
			protocol:    protocol.Unsupported,
		}
		currentListenerEntry.listener.ListenerFilters = appendListenerFilters(currentListenerEntry.listener.ListenerFilters)

	case AutoOverTCP:
		// Merge two TCP filter chains. HTTP filter chain will not conflict with TCP filter chain because HTTP filter chain match for
		// HTTP filter chain is different from TCP filter chain's.
		currentListenerEntry.listener.FilterChains = mergeTCPFilterChains(mutable.Listener.FilterChains,
			listenerOpts, listenerMapKey, listenerMap)
		currentListenerEntry.protocol = protocol.Unsupported
		currentListenerEntry.listener.ListenerFilters = appendListenerFilters(currentListenerEntry.listener.ListenerFilters)

	default:
		// Covered previously - in this case we return early to prevent creating listeners that we end up throwing away
		// This should never happen
		log.Errorf("Got unexpected conflict type %v. This should never happen", conflictType)
	}

	if log.DebugEnabled() && len(mutable.Listener.FilterChains) > 1 || currentListenerEntry != nil {
		var numChains int
		if currentListenerEntry != nil {
			numChains = len(currentListenerEntry.listener.FilterChains)
		} else {
			numChains = len(mutable.Listener.FilterChains)
		}
		log.Debugf("buildSidecarOutboundListeners: multiple filter chain listener %s with %d chains", mutable.Listener.Name, numChains)
	}
}

// httpListenerOpts are options for an HTTP listener
type httpListenerOpts struct {
	routeConfig *route.RouteConfiguration
	rds         string
	// If set, use this as a basis
	connectionManager *hcm.HttpConnectionManager
	// stat prefix for the http connection manager
	// DO not set this field. Will be overridden by buildCompleteFilterChain
	statPrefix string
	// addGRPCWebFilter specifies whether the envoy.grpc_web HTTP filter
	// should be added.
	addGRPCWebFilter bool
	useRemoteAddress bool
}

// thriftListenerOpts are options for a Thrift listener
type thriftListenerOpts struct {
	// Stats are not provided for the Thrift filter chain
	transport   thrift.TransportType
	protocol    thrift.ProtocolType
	routeConfig *thrift.RouteConfiguration
}

// filterChainOpts describes a filter chain: a set of filters with the same TLS context
type filterChainOpts struct {
	filterChainName  string
	sniHosts         []string
	destinationCIDRs []string
	metadata         *core.Metadata
	tlsContext       *auth.DownstreamTlsContext
	httpOpts         *httpListenerOpts
	thriftOpts       *thriftListenerOpts
	match            *listener.FilterChainMatch
	listenerFilters  []*listener.ListenerFilter
	networkFilters   []*listener.Filter
}

// ListenerClass defines the class of the listener
type ListenerClass int

const (
	ListenerClassUndefined ListenerClass = iota
	ListenerClassSidecarInbound
	ListenerClassSidecarOutbound
	ListenerClassGateway
)

// buildListenerOpts are the options required to build a Listener
type buildListenerOpts struct {
	// nolint: maligned
	push              *model.PushContext
	proxy             *model.Proxy
	bind              string
	port              *model.Port
	filterChainOpts   []*filterChainOpts
	bindToPort        bool
	skipUserFilters   bool
	needHTTPInspector bool
	class             ListenerClass
	service           *model.Service
}

func buildHTTPConnectionManager(listenerOpts buildListenerOpts, httpOpts *httpListenerOpts,
	httpFilters []*hcm.HttpFilter) *hcm.HttpConnectionManager {
	filters := make([]*hcm.HttpFilter, len(httpFilters))
	copy(filters, httpFilters)

	if httpOpts.addGRPCWebFilter {
		filters = append(filters, xdsfilters.GrpcWeb)
	}

	if listenerOpts.port != nil && listenerOpts.port.Protocol.IsGRPC() {
		filters = append(filters, xdsfilters.GrpcStats)
	}

	// append ALPN HTTP filter in HTTP connection manager for outbound listener only.
	if listenerOpts.class == ListenerClassSidecarOutbound {
		filters = append(filters, xdsfilters.Alpn)
	}

	filters = append(filters, xdsfilters.Cors, xdsfilters.Fault, xdsfilters.Router)

	if httpOpts.connectionManager == nil {
		httpOpts.connectionManager = &hcm.HttpConnectionManager{}
	}

	connectionManager := httpOpts.connectionManager
	connectionManager.CodecType = hcm.HttpConnectionManager_AUTO
	connectionManager.AccessLog = []*accesslog.AccessLog{}
	connectionManager.HttpFilters = filters
	connectionManager.StatPrefix = httpOpts.statPrefix
	connectionManager.NormalizePath = proto.BoolTrue
	if httpOpts.useRemoteAddress {
		connectionManager.UseRemoteAddress = proto.BoolTrue
	} else {
		connectionManager.UseRemoteAddress = proto.BoolFalse
	}

	// Allow websocket upgrades
	websocketUpgrade := &hcm.HttpConnectionManager_UpgradeConfig{UpgradeType: "websocket"}
	connectionManager.UpgradeConfigs = []*hcm.HttpConnectionManager_UpgradeConfig{websocketUpgrade}

	idleTimeout, err := time.ParseDuration(listenerOpts.proxy.Metadata.IdleTimeout)
	if err == nil {
		connectionManager.CommonHttpProtocolOptions = &core.HttpProtocolOptions{
			IdleTimeout: ptypes.DurationProto(idleTimeout),
		}
	}

	notimeout := ptypes.DurationProto(0 * time.Second)
	connectionManager.StreamIdleTimeout = notimeout

	if httpOpts.rds != "" {
		rds := &hcm.HttpConnectionManager_Rds{
			Rds: &hcm.Rds{
				ConfigSource: &core.ConfigSource{
					ConfigSourceSpecifier: &core.ConfigSource_Ads{
						Ads: &core.AggregatedConfigSource{},
					},
					ResourceApiVersion: core.ApiVersion_V3,
				},
				RouteConfigName: httpOpts.rds,
			},
		}
		connectionManager.RouteSpecifier = rds
	} else {
		connectionManager.RouteSpecifier = &hcm.HttpConnectionManager_RouteConfig{RouteConfig: httpOpts.routeConfig}
	}

	accessLogBuilder.setHTTPAccessLog(listenerOpts.push.Mesh, connectionManager, listenerOpts.proxy)

	if listenerOpts.push.Mesh.EnableTracing {
		proxyConfig := listenerOpts.proxy.Metadata.ProxyConfigOrDefault(listenerOpts.push.Mesh.DefaultConfig)
		connectionManager.Tracing = buildTracingConfig(proxyConfig)
	}

	return connectionManager
}

func buildTracingConfig(config *meshconfig.ProxyConfig) *hcm.HttpConnectionManager_Tracing {
	tracingCfg := &hcm.HttpConnectionManager_Tracing{}
	updateTraceSamplingConfig(config, tracingCfg)

	if config.Tracing != nil {
		// only specify a MaxPathTagLength if meshconfig has specified one
		// otherwise, rely on upstream envoy defaults
		if config.Tracing.MaxPathTagLength != 0 {
			tracingCfg.MaxPathTagLength =
				&wrappers.UInt32Value{
					Value: config.Tracing.MaxPathTagLength,
				}
		}
		tracingCfg.CustomTags = buildCustomTags(config.Tracing.CustomTags)
	}

	return tracingCfg
}

func defaultTags() []*tracing.CustomTag {
	return []*tracing.CustomTag{
		{
			Tag: "istio.canonical_revision",
			Type: &tracing.CustomTag_Environment_{
				Environment: &tracing.CustomTag_Environment{
					Name:         "CANONICAL_REVISION",
					DefaultValue: "latest",
				},
			},
		},
		{
			Tag: "istio.canonical_service",
			Type: &tracing.CustomTag_Environment_{
				Environment: &tracing.CustomTag_Environment{
					Name:         "CANONICAL_SERVICE",
					DefaultValue: "unknown",
				},
			},
		},
		{
			Tag: "istio.mesh_id",
			Type: &tracing.CustomTag_Environment_{
				Environment: &tracing.CustomTag_Environment{
					Name:         "ISTIO_META_MESH_ID",
					DefaultValue: "unknown",
				},
			},
		},
		{
			Tag: "istio.namespace",
			Type: &tracing.CustomTag_Environment_{
				Environment: &tracing.CustomTag_Environment{
					Name:         "POD_NAMESPACE",
					DefaultValue: "default",
				},
			},
		},
	}
}

func getPilotRandomSamplingEnv() float64 {
	f := features.TraceSampling
	if f < 0.0 || f > 100.0 {
		log.Warnf("PILOT_TRACE_SAMPLING out of range: %v", f)
		return 1.0
	}
	return f
}

func updateTraceSamplingConfig(config *meshconfig.ProxyConfig, cfg *hcm.HttpConnectionManager_Tracing) {
	sampling := pilotTraceSamplingEnv

	if config.Tracing != nil && config.Tracing.Sampling != 0.0 {
		sampling = config.Tracing.Sampling

		if sampling > 100.0 {
			sampling = 1.0
		}
	}
	cfg.ClientSampling = &xdstype.Percent{
		Value: 100.0,
	}
	cfg.RandomSampling = &xdstype.Percent{
		Value: sampling,
	}
	cfg.OverallSampling = &xdstype.Percent{
		Value: 100.0,
	}
}

func buildCustomTags(customTags map[string]*meshconfig.Tracing_CustomTag) []*tracing.CustomTag {
	var tags []*tracing.CustomTag

	if features.EnableIstioTags {
		defaultTags := defaultTags()
		tags = append(tags, defaultTags...)
	}

	for tagName, tagInfo := range customTags {
		switch tag := tagInfo.Type.(type) {
		case *meshconfig.Tracing_CustomTag_Environment:
			env := &tracing.CustomTag{
				Tag: tagName,
				Type: &tracing.CustomTag_Environment_{
					Environment: &tracing.CustomTag_Environment{
						Name:         tag.Environment.Name,
						DefaultValue: tag.Environment.DefaultValue,
					},
				},
			}
			tags = append(tags, env)
		case *meshconfig.Tracing_CustomTag_Header:
			header := &tracing.CustomTag{
				Tag: tagName,
				Type: &tracing.CustomTag_RequestHeader{
					RequestHeader: &tracing.CustomTag_Header{
						Name:         tag.Header.Name,
						DefaultValue: tag.Header.DefaultValue,
					},
				},
			}
			tags = append(tags, header)
		case *meshconfig.Tracing_CustomTag_Literal:
			env := &tracing.CustomTag{
				Tag: tagName,
				Type: &tracing.CustomTag_Literal_{
					Literal: &tracing.CustomTag_Literal{
						Value: tag.Literal.Value,
					},
				},
			}
			tags = append(tags, env)
		}
	}

	// looping over customTags, a map, results in the returned value
	// being non-deterministic when multiple tags were defined; sort by the tag name
	// to rectify this
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Tag < tags[j].Tag
	})

	return tags
}

func buildThriftRatelimit(domain string, thriftconfig *meshconfig.MeshConfig_ThriftConfig) *thrift_ratelimit.RateLimit {
	thriftRateLimit := &thrift_ratelimit.RateLimit{
		Domain:          domain,
		Timeout:         ptypes.DurationProto(ThriftRLSDefaultTimeoutMS * time.Millisecond),
		FailureModeDeny: false,
		RateLimitService: &ratelimit.RateLimitServiceConfig{
			GrpcService: &core.GrpcService{},
		},
	}

	rlsClusterName, err := thriftRLSClusterNameFromAuthority(thriftconfig.RateLimitUrl)
	if err != nil {
		log.Errorf("unable to generate thrift rls cluster name: %s\n", rlsClusterName)
		return nil
	}

	thriftRateLimit.RateLimitService.GrpcService.TargetSpecifier = &core.GrpcService_EnvoyGrpc_{
		EnvoyGrpc: &core.GrpcService_EnvoyGrpc{
			ClusterName: rlsClusterName,
		},
	}

	if meshConfigTimeout := thriftconfig.GetRateLimitTimeout(); meshConfigTimeout != nil {
		thriftRateLimit.Timeout = gogo.DurationToProtoDuration(meshConfigTimeout)
	}

	if err := thriftRateLimit.Validate(); err != nil {
		log.Warn(err.Error())
	}

	return thriftRateLimit
}

func buildThriftProxy(thriftOpts *thriftListenerOpts) *thrift.ThriftProxy {
	return &thrift.ThriftProxy{
		Transport:   thriftOpts.transport,
		Protocol:    thriftOpts.protocol,
		RouteConfig: thriftOpts.routeConfig,
	}
}

// buildListener builds and initializes a Listener proto based on the provided opts. It does not set any filters.
func buildListener(opts buildListenerOpts, trafficDirection core.TrafficDirection) *listener.Listener {
	filterChains := make([]*listener.FilterChain, 0, len(opts.filterChainOpts))
	listenerFiltersMap := make(map[string]bool)
	var listenerFilters []*listener.ListenerFilter

	// add a TLS inspector if we need to detect ServerName or ALPN
	needTLSInspector := false
	for _, chain := range opts.filterChainOpts {
		needsALPN := chain.tlsContext != nil && chain.tlsContext.CommonTlsContext != nil && len(chain.tlsContext.CommonTlsContext.AlpnProtocols) > 0
		if len(chain.sniHosts) > 0 || needsALPN {
			needTLSInspector = true
			break
		}
	}

	if opts.proxy.GetInterceptionMode() == model.InterceptionTproxy && trafficDirection == core.TrafficDirection_INBOUND {
		listenerFiltersMap[xdsfilters.OriginalSrcFilterName] = true
		listenerFilters = append(listenerFilters, xdsfilters.OriginalSrc)
	}

	// We add a TLS inspector when http inspector is needed for outbound only. This
	// is because if we ever set ALPN in the match without
	// transport_protocol=raw_buffer, Envoy will automatically inject a tls
	// inspector: https://github.com/envoyproxy/envoy/issues/13601. This leads to
	// excessive logging and loss of control over the config For inbound this is not
	// needed, since we are explicitly setting transport protocol in every single
	// match. We can do this for outbound as well, at which point this could be
	// removed, but have not yet
	if needTLSInspector || (opts.class == ListenerClassSidecarOutbound && opts.needHTTPInspector) {
		listenerFiltersMap[wellknown.TlsInspector] = true
		listenerFilters = append(listenerFilters, xdsfilters.TLSInspector)
	}

	if opts.needHTTPInspector {
		listenerFiltersMap[wellknown.HttpInspector] = true
		listenerFilters = append(listenerFilters, xdsfilters.HTTPInspector)
	}

	for _, chain := range opts.filterChainOpts {
		for _, filter := range chain.listenerFilters {
			if _, exist := listenerFiltersMap[filter.Name]; !exist {
				listenerFiltersMap[filter.Name] = true
				listenerFilters = append(listenerFilters, filter)
			}
		}
		match := &listener.FilterChainMatch{}
		needMatch := false
		if chain.match != nil {
			needMatch = true
			match = chain.match
		}
		if len(chain.sniHosts) > 0 {
			fullWildcardFound := false
			for _, h := range chain.sniHosts {
				if h == "*" {
					fullWildcardFound = true
					// If we have a host with *, it effectively means match anything, i.e.
					// no SNI based matching for this host.
					break
				}
			}
			if !fullWildcardFound {
				chain.sniHosts = append([]string{}, chain.sniHosts...)
				sort.Stable(sort.StringSlice(chain.sniHosts))
				match.ServerNames = chain.sniHosts
			}
		}
		if len(chain.destinationCIDRs) > 0 {
			chain.destinationCIDRs = append([]string{}, chain.destinationCIDRs...)
			sort.Stable(sort.StringSlice(chain.destinationCIDRs))
			for _, d := range chain.destinationCIDRs {
				if len(d) == 0 {
					continue
				}
				cidr := util.ConvertAddressToCidr(d)
				if cidr != nil && cidr.AddressPrefix != constants.UnspecifiedIP {
					match.PrefixRanges = append(match.PrefixRanges, cidr)
				}
			}
		}

		if !needMatch && filterChainMatchEmpty(match) {
			match = nil
		}
		filterChains = append(filterChains, &listener.FilterChain{
			FilterChainMatch: match,
			TransportSocket:  buildDownstreamTLSTransportSocket(chain.tlsContext),
		})
	}

	var deprecatedV1 *listener.Listener_DeprecatedV1
	if !opts.bindToPort {
		deprecatedV1 = &listener.Listener_DeprecatedV1{
			BindToPort: proto.BoolFalse,
		}
	}

	listener := &listener.Listener{
		// TODO: need to sanitize the opts.bind if its a UDS socket, as it could have colons, that envoy
		// doesn't like
		Name:             opts.bind + "_" + strconv.Itoa(opts.port.Port),
		Address:          util.BuildAddress(opts.bind, uint32(opts.port.Port)),
		TrafficDirection: trafficDirection,
		ListenerFilters:  listenerFilters,
		FilterChains:     filterChains,
		DeprecatedV1:     deprecatedV1,
	}

	accessLogBuilder.setListenerAccessLog(opts.push.Mesh, listener, opts.proxy)

	if opts.proxy.Type != model.Router {
		listener.ListenerFiltersTimeout = gogo.DurationToProtoDuration(opts.push.Mesh.ProtocolDetectionTimeout)
		if listener.ListenerFiltersTimeout != nil {
			listener.ContinueOnListenerFiltersTimeout = true
		}
	}

	return listener
}

func getMatchAllFilterChain(l *listener.Listener) (int, *listener.FilterChain) {
	for i, fc := range l.FilterChains {
		if isMatchAllFilterChain(fc) {
			return i, fc
		}
	}
	return 0, nil
}

// Create pass through filter chain for the listener assuming all the other filter chains are ready.
// The match member of pass through filter chain depends on the existing non-passthrough filter chain.
// TODO(lambdai): Calculate the filter chain match to replace the wildcard and replace appendListenerFallthroughRoute.
func (configgen *ConfigGeneratorImpl) appendListenerFallthroughRouteForCompleteListener(l *listener.Listener, node *model.Proxy, push *model.PushContext) {
	matchIndex, matchAll := getMatchAllFilterChain(l)

	fallthroughNetworkFilters := buildOutboundCatchAllNetworkFiltersOnly(push, node)

	outboundPassThroughFilterChain := &listener.FilterChain{
		FilterChainMatch: &listener.FilterChainMatch{},
		Name:             util.PassthroughFilterChain,
		Filters:          fallthroughNetworkFilters,
	}

	// Set a default filter chain. This allows us to avoid issues where
	// traffic starts to match a filter chain but then doesn't match latter criteria, leading to
	// dropped requests. See https://github.com/istio/istio/issues/26079 for details.
	// If there are multiple filter chains and a match all chain, move it to DefaultFilterChain
	// This ensures it will always be used as the fallback.
	if matchAll != nil && len(l.FilterChains) > 1 {
		copy(l.FilterChains[matchIndex:], l.FilterChains[matchIndex+1:]) // Shift l.FilterChains[i+1:] left one index.
		l.FilterChains[len(l.FilterChains)-1] = nil                      // Erase last element (write zero value).
		l.FilterChains = l.FilterChains[:len(l.FilterChains)-1]          // Truncate slice.
		l.DefaultFilterChain = matchAll
	} else if matchAll == nil {
		// Otherwise, if there is no match all already, set a passthrough match all
		l.DefaultFilterChain = outboundPassThroughFilterChain
	}
}

// buildCompleteFilterChain adds the provided TCP and HTTP filters to the provided Listener and serializes them.
//
// TODO: should we change this from []plugins.FilterChains to [][]listener.Filter, [][]*hcm.HttpFilter?
// TODO: given how tightly tied listener.FilterChains, opts.filterChainOpts, and mutable.FilterChains are to eachother
// we should encapsulate them some way to ensure they remain consistent (mainly that in each an index refers to the same
// chain)
func buildCompleteFilterChain(mutable *istionetworking.MutableObjects, opts buildListenerOpts) error {
	if len(opts.filterChainOpts) == 0 {
		return fmt.Errorf("must have more than 0 chains in listener %q", mutable.Listener.Name)
	}

	httpConnectionManagers := make([]*hcm.HttpConnectionManager, len(mutable.FilterChains))
	thriftProxies := make([]*thrift.ThriftProxy, len(mutable.FilterChains))
	for i := range mutable.FilterChains {
		chain := mutable.FilterChains[i]
		opt := opts.filterChainOpts[i]
		mutable.Listener.FilterChains[i].Metadata = opt.metadata
		mutable.Listener.FilterChains[i].Name = opt.filterChainName

		if opt.thriftOpts != nil && features.EnableThriftFilter {
			// Add the TCP filters first.. and then the Thrift filter
			mutable.Listener.FilterChains[i].Filters = append(mutable.Listener.FilterChains[i].Filters, chain.TCP...)

			thriftProxies[i] = buildThriftProxy(opt.thriftOpts)

			// If the RLS service was provided, add the RLS to the Thrift filter
			// chain. Rate limiting is only applied client-side.
			if rlsURI := opts.push.Mesh.ThriftConfig.RateLimitUrl; rlsURI != "" &&
				mutable.Listener.TrafficDirection == core.TrafficDirection_OUTBOUND &&
				opts.service != nil &&
				opts.service.Hostname != "" && false { // TODO: restore ability to add thrift quota
				rateLimitConfig := buildThriftRatelimit(fmt.Sprint(opts.service.Hostname), opts.push.Mesh.ThriftConfig)
				rateLimitFilter := &thrift.ThriftFilter{
					Name: "envoy.filters.thrift.rate_limit",
				}
				routerFilter := &thrift.ThriftFilter{
					Name:       "envoy.filters.thrift.router",
					ConfigType: &thrift.ThriftFilter_TypedConfig{TypedConfig: util.MessageToAny(rateLimitConfig)},
				}

				thriftProxies[i].ThriftFilters = append(thriftProxies[i].ThriftFilters, rateLimitFilter, routerFilter)

			}

			filter := &listener.Filter{
				Name:       wellknown.ThriftProxy,
				ConfigType: &listener.Filter_TypedConfig{TypedConfig: util.MessageToAny(thriftProxies[i])},
			}

			mutable.Listener.FilterChains[i].Filters = append(mutable.Listener.FilterChains[i].Filters, filter)
			log.Debugf("attached Thrift filter with %d thrift_filter options to listener %q filter chain %d",
				len(thriftProxies[i].ThriftFilters), mutable.Listener.Name, i)
		} else if opt.httpOpts == nil {
			// we are building a network filter chain (no http connection manager) for this filter chain
			// In HTTP, we need to have RBAC, etc. upfront so that they can enforce policies immediately
			// For network filters such as mysql, mongo, etc., we need the filter codec upfront. Data from this
			// codec is used by RBAC later.

			if len(opt.networkFilters) > 0 {
				// this is the terminating filter
				lastNetworkFilter := opt.networkFilters[len(opt.networkFilters)-1]

				for n := 0; n < len(opt.networkFilters)-1; n++ {
					mutable.Listener.FilterChains[i].Filters = append(mutable.Listener.FilterChains[i].Filters, opt.networkFilters[n])
				}
				mutable.Listener.FilterChains[i].Filters = append(mutable.Listener.FilterChains[i].Filters, chain.TCP...)
				mutable.Listener.FilterChains[i].Filters = append(mutable.Listener.FilterChains[i].Filters, lastNetworkFilter)
			} else {
				mutable.Listener.FilterChains[i].Filters = append(mutable.Listener.FilterChains[i].Filters, chain.TCP...)
			}
			log.Debugf("attached %d network filters to listener %q filter chain %d", len(chain.TCP)+len(opt.networkFilters), mutable.Listener.Name, i)
		} else {
			// Add the TCP filters first.. and then the HTTP connection manager
			mutable.Listener.FilterChains[i].Filters = append(mutable.Listener.FilterChains[i].Filters, chain.TCP...)

			// If statPrefix has been set before calling this method, respect that.
			if len(opt.httpOpts.statPrefix) == 0 {
				opt.httpOpts.statPrefix = strings.ToLower(mutable.Listener.TrafficDirection.String()) + "_" + mutable.Listener.Name
			}
			httpConnectionManagers[i] = buildHTTPConnectionManager(opts, opt.httpOpts, chain.HTTP)
			filter := &listener.Filter{
				Name:       wellknown.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{TypedConfig: util.MessageToAny(httpConnectionManagers[i])},
			}
			mutable.Listener.FilterChains[i].Filters = append(mutable.Listener.FilterChains[i].Filters, filter)
			log.Debugf("attached HTTP filter with %d http_filter options to listener %q filter chain %d",
				len(httpConnectionManagers[i].HttpFilters), mutable.Listener.Name, i)
		}
	}

	return nil
}

// getActualWildcardAndLocalHost will return corresponding Wildcard and LocalHost
// depending on value of proxy's IPAddresses. This function checks each element
// and if there is at least one ipv4 address other than 127.0.0.1, it will use ipv4 address,
// if all addresses are ipv6  addresses then ipv6 address will be used to get wildcard and local host address.
func getActualWildcardAndLocalHost(node *model.Proxy) (string, string) {
	if node.SupportsIPv4() {
		return WildcardAddress, LocalhostAddress
	}
	return WildcardIPv6Address, LocalhostIPv6Address
}

func getPassthroughBindIP(node *model.Proxy) string {
	if node.SupportsIPv4() {
		return util.InboundPassthroughBindIpv4
	}
	return util.InboundPassthroughBindIpv6
}

// getSidecarInboundBindIP returns the IP that the proxy can bind to along with the sidecar specified port.
// It looks for an unicast address, if none found, then the default wildcard address is used.
// This will make the inbound listener bind to instance_ip:port instead of 0.0.0.0:port where applicable.
func getSidecarInboundBindIP(node *model.Proxy) string {
	// Return the IP if its a global unicast address.
	if len(node.GlobalUnicastIP) > 0 {
		return node.GlobalUnicastIP
	}
	defaultInboundIP, _ := getActualWildcardAndLocalHost(node)
	return defaultInboundIP
}

func mergeTCPFilterChains(incoming []*listener.FilterChain, listenerOpts buildListenerOpts, listenerMapKey string,
	listenerMap map[string]*outboundListenerEntry) []*listener.FilterChain {
	// TODO(rshriram) merge multiple identical filter chains with just a single destination CIDR based
	// filter chain match, into a single filter chain and array of destinationcidr matches

	// The code below checks for TCP over TCP conflicts and merges listeners

	// Merge the newly built listener with the existing listener, if and only if the filter chains have distinct conditions.
	// Extract the current filter chain matches, for every new filter chain match being added, check if there is a matching
	// one in previous filter chains, if so, skip adding this filter chain with a warning.

	currentListenerEntry := listenerMap[listenerMapKey]
	mergedFilterChains := make([]*listener.FilterChain, 0, len(currentListenerEntry.listener.FilterChains)+len(incoming))
	// Start with the current listener's filter chains.
	mergedFilterChains = append(mergedFilterChains, currentListenerEntry.listener.FilterChains...)

	for _, incomingFilterChain := range incoming {
		conflict := false

		for _, existingFilterChain := range mergedFilterChains {
			conflict = isConflict(existingFilterChain, incomingFilterChain)

			if conflict {
				// NOTE: While pluginParams.Service can be nil,
				// this code cannot be reached if Service is nil because a pluginParams.Service can be nil only
				// for user defined Egress listeners with ports. And these should occur in the API before
				// the wildcard egress listener. the check for the "locked" bit will eliminate the collision.
				// User is also not allowed to add duplicate ports in the egress listener
				var newHostname host.Name
				if listenerOpts.service != nil {
					newHostname = listenerOpts.service.Hostname
				} else {
					// user defined outbound listener via sidecar API
					newHostname = "sidecar-config-egress-tcp-listener"
				}

				outboundListenerConflict{
					metric:          model.ProxyStatusConflictOutboundListenerTCPOverTCP,
					node:            listenerOpts.proxy,
					listenerName:    listenerMapKey,
					currentServices: currentListenerEntry.services,
					currentProtocol: currentListenerEntry.servicePort.Protocol,
					newHostname:     newHostname,
					newProtocol:     listenerOpts.port.Protocol,
				}.addMetric(listenerOpts.push)
				break
			}

		}
		if !conflict {
			// There is no conflict with any filter chain in the existing listener.
			// So append the new filter chains to the existing listener's filter chains
			mergedFilterChains = append(mergedFilterChains, incomingFilterChain)
			if listenerOpts.service != nil {
				lEntry := listenerMap[listenerMapKey]
				lEntry.services = append(lEntry.services, listenerOpts.service)
			}
		}
	}
	return mergedFilterChains
}

// isConflict determines whether the incoming filter chain has conflict with existing filter chain.
func isConflict(existing, incoming *listener.FilterChain) bool {
	return filterChainMatchEqual(existing.FilterChainMatch, incoming.FilterChainMatch)
}

func filterChainMatchEmpty(fcm *listener.FilterChainMatch) bool {
	return fcm == nil || filterChainMatchEqual(fcm, emptyFilterChainMatch)
}

// filterChainMatchEqual returns true if both filter chains are equal otherwise false.
func filterChainMatchEqual(first *listener.FilterChainMatch, second *listener.FilterChainMatch) bool {
	if first == nil || second == nil {
		return first == second
	}
	if first.TransportProtocol != second.TransportProtocol {
		return false
	}
	if !util.StringSliceEqual(first.ApplicationProtocols, second.ApplicationProtocols) {
		return false
	}
	if first.DestinationPort.GetValue() != second.DestinationPort.GetValue() {
		return false
	}
	if !util.CidrRangeSliceEqual(first.PrefixRanges, second.PrefixRanges) {
		return false
	}
	if !util.CidrRangeSliceEqual(first.SourcePrefixRanges, second.SourcePrefixRanges) {
		return false
	}
	if first.AddressSuffix != second.AddressSuffix {
		return false
	}
	if first.SuffixLen.GetValue() != second.SuffixLen.GetValue() {
		return false
	}
	if first.SourceType != second.SourceType {
		return false
	}
	if !util.UInt32SliceEqual(first.SourcePorts, second.SourcePorts) {
		return false
	}
	if !util.StringSliceEqual(first.ServerNames, second.ServerNames) {
		return false
	}
	return true
}

func mergeFilterChains(httpFilterChain, tcpFilterChain []*listener.FilterChain) []*listener.FilterChain {
	var newFilterChan []*listener.FilterChain
	for _, fc := range httpFilterChain {
		if fc.FilterChainMatch == nil {
			fc.FilterChainMatch = &listener.FilterChainMatch{}
		}

		var missingHTTPALPNs []string
		for _, p := range plaintextHTTPALPNs {
			if !contains(fc.FilterChainMatch.ApplicationProtocols, p) {
				missingHTTPALPNs = append(missingHTTPALPNs, p)
			}
		}

		fc.FilterChainMatch.ApplicationProtocols = append(fc.FilterChainMatch.ApplicationProtocols, missingHTTPALPNs...)
		newFilterChan = append(newFilterChan, fc)
	}
	return append(tcpFilterChain, newFilterChan...)
}

// It's fine to use this naive implementation for searching in a very short list like ApplicationProtocols
func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func getPluginFilterChain(opts buildListenerOpts) []istionetworking.FilterChain {
	filterChain := make([]istionetworking.FilterChain, len(opts.filterChainOpts))

	for id := range filterChain {
		if opts.filterChainOpts[id].httpOpts == nil {
			filterChain[id].ListenerProtocol = istionetworking.ListenerProtocolTCP
		} else {
			filterChain[id].ListenerProtocol = istionetworking.ListenerProtocolHTTP
		}
	}

	return filterChain
}

// isConflictWithWellKnownPort checks conflicts between incoming protocol and existing protocol.
// Mongo and MySQL are not allowed to co-exist with other protocols in one port.
func isConflictWithWellKnownPort(incoming, existing protocol.Instance, conflict int) bool {
	if conflict == NoConflict {
		return true
	}

	if (incoming == protocol.Mongo ||
		incoming == protocol.MySQL ||
		existing == protocol.Mongo ||
		existing == protocol.MySQL) && incoming != existing {
		return false
	}

	return true
}

func appendListenerFilters(filters []*listener.ListenerFilter) []*listener.ListenerFilter {
	hasTLSInspector := false
	hasHTTPInspector := false

	for _, f := range filters {
		hasTLSInspector = hasTLSInspector || f.Name == wellknown.TlsInspector
		hasHTTPInspector = hasHTTPInspector || f.Name == wellknown.HttpInspector
	}

	if !hasTLSInspector {
		filters =
			append(filters, xdsfilters.TLSInspector)
	}

	if !hasHTTPInspector {
		filters =
			append(filters, xdsfilters.HTTPInspector)
	}

	return filters
}

// nolint: interfacer
func buildDownstreamTLSTransportSocket(tlsContext *auth.DownstreamTlsContext) *core.TransportSocket {
	if tlsContext == nil {
		return nil
	}
	return &core.TransportSocket{Name: util.EnvoyTLSSocketName, ConfigType: &core.TransportSocket_TypedConfig{TypedConfig: util.MessageToAny(tlsContext)}}
}

func isMatchAllFilterChain(fc *listener.FilterChain) bool {
	// See if it is empty filter chain.
	return filterChainMatchEmpty(fc.FilterChainMatch)
}

func removeListenerFilterTimeout(listeners []*listener.Listener) {
	for _, l := range listeners {
		// Remove listener filter timeout for
		// 	1. outbound listeners AND
		// 	2. without HTTP inspector
		hasHTTPInspector := false
		for _, lf := range l.ListenerFilters {
			if lf.Name == wellknown.HttpInspector {
				hasHTTPInspector = true
				break
			}
		}

		if !hasHTTPInspector && l.TrafficDirection == core.TrafficDirection_OUTBOUND {
			l.ListenerFiltersTimeout = nil
			l.ContinueOnListenerFiltersTimeout = false
		}
	}
}

// listenerKey builds the key for a given bind and port
func listenerKey(bind string, port int) string {
	return bind + ":" + strconv.Itoa(port)
}

func dropAlpnFromList(alpnProtocols []string, alpnToDrop string) []string {
	var newAlpnProtocols []string
	for _, alpn := range alpnProtocols {
		if alpn == alpnToDrop {
			continue
		}
		newAlpnProtocols = append(newAlpnProtocols, alpn)
	}
	return newAlpnProtocols
}
