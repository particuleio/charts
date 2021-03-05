// GENERATED FILE -- DO NOT EDIT
//

package msg

import (
	"istio.io/istio/galley/pkg/config/analysis/diag"
	"istio.io/istio/pkg/config/resource"
)

var (
	// InternalError defines a diag.MessageType for message "InternalError".
	// Description: There was an internal error in the toolchain. This is almost always a bug in the implementation.
	InternalError = diag.NewMessageType(diag.Error, "IST0001", "Internal error: %v")

	// Deprecated defines a diag.MessageType for message "Deprecated".
	// Description: A feature that the configuration is depending on is now deprecated.
	Deprecated = diag.NewMessageType(diag.Warning, "IST0002", "Deprecated: %s")

	// ReferencedResourceNotFound defines a diag.MessageType for message "ReferencedResourceNotFound".
	// Description: A resource being referenced does not exist.
	ReferencedResourceNotFound = diag.NewMessageType(diag.Error, "IST0101", "Referenced %s not found: %q")

	// NamespaceNotInjected defines a diag.MessageType for message "NamespaceNotInjected".
	// Description: A namespace is not enabled for Istio injection.
	NamespaceNotInjected = diag.NewMessageType(diag.Info, "IST0102", "The namespace is not enabled for Istio injection. Run 'kubectl label namespace %s istio-injection=enabled' to enable it, or 'kubectl label namespace %s istio-injection=disabled' to explicitly mark it as not needing injection.")

	// PodMissingProxy defines a diag.MessageType for message "PodMissingProxy".
	// Description: A pod is missing the Istio proxy.
	PodMissingProxy = diag.NewMessageType(diag.Warning, "IST0103", "The pod is missing the Istio proxy. This can often be resolved by restarting or redeploying the workload.")

	// GatewayPortNotOnWorkload defines a diag.MessageType for message "GatewayPortNotOnWorkload".
	// Description: Unhandled gateway port
	GatewayPortNotOnWorkload = diag.NewMessageType(diag.Warning, "IST0104", "The gateway refers to a port that is not exposed on the workload (pod selector %s; port %d)")

	// IstioProxyImageMismatch defines a diag.MessageType for message "IstioProxyImageMismatch".
	// Description: The image of the Istio proxy running on the pod does not match the image defined in the injection configuration.
	IstioProxyImageMismatch = diag.NewMessageType(diag.Warning, "IST0105", "The image of the Istio proxy running on the pod does not match the image defined in the injection configuration (pod image: %s; injection configuration image: %s). This often happens after upgrading the Istio control-plane and can be fixed by redeploying the pod.")

	// SchemaValidationError defines a diag.MessageType for message "SchemaValidationError".
	// Description: The resource has a schema validation error.
	SchemaValidationError = diag.NewMessageType(diag.Error, "IST0106", "Schema validation error: %v")

	// MisplacedAnnotation defines a diag.MessageType for message "MisplacedAnnotation".
	// Description: An Istio annotation is applied to the wrong kind of resource.
	MisplacedAnnotation = diag.NewMessageType(diag.Warning, "IST0107", "Misplaced annotation: %s can only be applied to %s")

	// UnknownAnnotation defines a diag.MessageType for message "UnknownAnnotation".
	// Description: An Istio annotation is not recognized for any kind of resource
	UnknownAnnotation = diag.NewMessageType(diag.Warning, "IST0108", "Unknown annotation: %s")

	// ConflictingMeshGatewayVirtualServiceHosts defines a diag.MessageType for message "ConflictingMeshGatewayVirtualServiceHosts".
	// Description: Conflicting hosts on VirtualServices associated with mesh gateway
	ConflictingMeshGatewayVirtualServiceHosts = diag.NewMessageType(diag.Error, "IST0109", "The VirtualServices %s associated with mesh gateway define the same host %s which can lead to undefined behavior. This can be fixed by merging the conflicting VirtualServices into a single resource.")

	// ConflictingSidecarWorkloadSelectors defines a diag.MessageType for message "ConflictingSidecarWorkloadSelectors".
	// Description: A Sidecar resource selects the same workloads as another Sidecar resource
	ConflictingSidecarWorkloadSelectors = diag.NewMessageType(diag.Error, "IST0110", "The Sidecars %v in namespace %q select the same workload pod %q, which can lead to undefined behavior.")

	// MultipleSidecarsWithoutWorkloadSelectors defines a diag.MessageType for message "MultipleSidecarsWithoutWorkloadSelectors".
	// Description: More than one sidecar resource in a namespace has no workload selector
	MultipleSidecarsWithoutWorkloadSelectors = diag.NewMessageType(diag.Error, "IST0111", "The Sidecars %v in namespace %q have no workload selector, which can lead to undefined behavior.")

	// VirtualServiceDestinationPortSelectorRequired defines a diag.MessageType for message "VirtualServiceDestinationPortSelectorRequired".
	// Description: A VirtualService routes to a service with more than one port exposed, but does not specify which to use.
	VirtualServiceDestinationPortSelectorRequired = diag.NewMessageType(diag.Error, "IST0112", "This VirtualService routes to a service %q that exposes multiple ports %v. Specifying a port in the destination is required to disambiguate.")

	// MTLSPolicyConflict defines a diag.MessageType for message "MTLSPolicyConflict".
	// Description: A DestinationRule and Policy are in conflict with regards to mTLS.
	MTLSPolicyConflict = diag.NewMessageType(diag.Error, "IST0113", "A DestinationRule and Policy are in conflict with regards to mTLS for host %s. The DestinationRule %q specifies that mTLS must be %t but the Policy object %q specifies %s.")

	// DeploymentAssociatedToMultipleServices defines a diag.MessageType for message "DeploymentAssociatedToMultipleServices".
	// Description: The resulting pods of a service mesh deployment can't be associated with multiple services using the same port but different protocols.
	DeploymentAssociatedToMultipleServices = diag.NewMessageType(diag.Warning, "IST0116", "This deployment %s is associated with multiple services using port %d but different protocols: %v")

	// DeploymentRequiresServiceAssociated defines a diag.MessageType for message "DeploymentRequiresServiceAssociated".
	// Description: The resulting pods of a service mesh deployment must be associated with at least one service.
	DeploymentRequiresServiceAssociated = diag.NewMessageType(diag.Warning, "IST0117", "No service associated with this deployment. Service mesh deployments must be associated with a service.")

	// PortNameIsNotUnderNamingConvention defines a diag.MessageType for message "PortNameIsNotUnderNamingConvention".
	// Description: Port name is not under naming convention. Protocol detection is applied to the port.
	PortNameIsNotUnderNamingConvention = diag.NewMessageType(diag.Info, "IST0118", "Port name %s (port: %d, targetPort: %s) doesn't follow the naming convention of Istio port.")

	// JwtFailureDueToInvalidServicePortPrefix defines a diag.MessageType for message "JwtFailureDueToInvalidServicePortPrefix".
	// Description: Authentication policy with JWT targets Service with invalid port specification.
	JwtFailureDueToInvalidServicePortPrefix = diag.NewMessageType(diag.Warning, "IST0119", "Authentication policy with JWT targets Service with invalid port specification (port: %d, name: %s, protocol: %s, targetPort: %s).")

	// InvalidRegexp defines a diag.MessageType for message "InvalidRegexp".
	// Description: Invalid Regex
	InvalidRegexp = diag.NewMessageType(diag.Warning, "IST0122", "Field %q regular expression invalid: %q (%s)")

	// NamespaceMultipleInjectionLabels defines a diag.MessageType for message "NamespaceMultipleInjectionLabels".
	// Description: A namespace has both new and legacy injection labels
	NamespaceMultipleInjectionLabels = diag.NewMessageType(diag.Warning, "IST0123", "The namespace has both new and legacy injection labels. Run 'kubectl label namespace %s istio.io/rev-' or 'kubectl label namespace %s istio-injection-'")

	// InvalidAnnotation defines a diag.MessageType for message "InvalidAnnotation".
	// Description: An Istio annotation that is not valid
	InvalidAnnotation = diag.NewMessageType(diag.Warning, "IST0125", "Invalid annotation %s: %s")

	// UnknownMeshNetworksServiceRegistry defines a diag.MessageType for message "UnknownMeshNetworksServiceRegistry".
	// Description: A service registry in Mesh Networks is unknown
	UnknownMeshNetworksServiceRegistry = diag.NewMessageType(diag.Error, "IST0126", "Unknown service registry %s in network %s")

	// NoMatchingWorkloadsFound defines a diag.MessageType for message "NoMatchingWorkloadsFound".
	// Description: There aren't workloads matching the resource labels
	NoMatchingWorkloadsFound = diag.NewMessageType(diag.Warning, "IST0127", "No matching workloads for this resource with the following labels: %s")

	// NoServerCertificateVerificationDestinationLevel defines a diag.MessageType for message "NoServerCertificateVerificationDestinationLevel".
	// Description: No caCertificates are set in DestinationRule, this results in no verification of presented server certificate.
	NoServerCertificateVerificationDestinationLevel = diag.NewMessageType(diag.Error, "IST0128", "DestinationRule %s in namespace %s has TLS mode set to %s but no caCertificates are set to validate server identity for host: %s")

	// NoServerCertificateVerificationPortLevel defines a diag.MessageType for message "NoServerCertificateVerificationPortLevel".
	// Description: No caCertificates are set in DestinationRule, this results in no verification of presented server certificate for traffic to a given port.
	NoServerCertificateVerificationPortLevel = diag.NewMessageType(diag.Warning, "IST0129", "DestinationRule %s in namespace %s has TLS mode set to %s but no caCertificates are set to validate server identity for host: %s at port %s")

	// VirtualServiceUnreachableRule defines a diag.MessageType for message "VirtualServiceUnreachableRule".
	// Description: A VirtualService rule will never be used because a previous rule uses the same match.
	VirtualServiceUnreachableRule = diag.NewMessageType(diag.Warning, "IST0130", "VirtualService rule %v not used (%s).")

	// VirtualServiceIneffectiveMatch defines a diag.MessageType for message "VirtualServiceIneffectiveMatch".
	// Description: A VirtualService rule match duplicates a match in a previous rule.
	VirtualServiceIneffectiveMatch = diag.NewMessageType(diag.Info, "IST0131", "VirtualService rule %v match %v is not used (duplicates a match in rule %v).")

	// VirtualServiceHostNotFoundInGateway defines a diag.MessageType for message "VirtualServiceHostNotFoundInGateway".
	// Description: Host defined in VirtualService not found in Gateway.
	VirtualServiceHostNotFoundInGateway = diag.NewMessageType(diag.Warning, "IST0132", "one or more host %v defined in VirtualService %s not found in Gateway %s.")

	// SchemaWarning defines a diag.MessageType for message "SchemaWarning".
	// Description: The resource has a schema validation warning.
	SchemaWarning = diag.NewMessageType(diag.Warning, "IST0133", "Schema validation warning: %v")

	// ServiceEntryAddressesRequired defines a diag.MessageType for message "ServiceEntryAddressesRequired".
	// Description: Virtual IP addresses are required for ports serving TCP (or unset) protocol
	ServiceEntryAddressesRequired = diag.NewMessageType(diag.Warning, "IST0134", "ServiceEntry addresses are required for this protocol.")

	// DeprecatedAnnotation defines a diag.MessageType for message "DeprecatedAnnotation".
	// Description: A resource is using a deprecated Istio annotation.
	DeprecatedAnnotation = diag.NewMessageType(diag.Info, "IST0135", "Annotation %q has been deprecated and may not work in future Istio versions.")

	// AlphaAnnotation defines a diag.MessageType for message "AlphaAnnotation".
	// Description: An Istio annotation may not be suitable for production.
	AlphaAnnotation = diag.NewMessageType(diag.Info, "IST0136", "Annotation %q is part of an alpha-phase feature and may be incompletely supported.")

	// DeploymentConflictingPorts defines a diag.MessageType for message "DeploymentConflictingPorts".
	// Description: Two services selecting the same workload with same target port are MUST refer to the same port.
	DeploymentConflictingPorts = diag.NewMessageType(diag.Warning, "IST0137", "This deployment %s is associated with multiple services %v using targetPort %q but different ports: %v.")

	// GatewayDuplicateCertificate defines a diag.MessageType for message "GatewayDuplicateCertificate".
	// Description: Duplicate certificate in multiple gateways may cause 404s if clients re-use HTTP2 connections.
	GatewayDuplicateCertificate = diag.NewMessageType(diag.Warning, "IST0138", "Duplicate certificate in multiple gateways %v may cause 404s if clients re-use HTTP2 connections.")

	// InvalidWebhook defines a diag.MessageType for message "InvalidWebhook".
	// Description: Webhook is invalid or references a control plane service that does not exist.
	InvalidWebhook = diag.NewMessageType(diag.Error, "IST0139", "%v")
)

// All returns a list of all known message types.
func All() []*diag.MessageType {
	return []*diag.MessageType{
		InternalError,
		Deprecated,
		ReferencedResourceNotFound,
		NamespaceNotInjected,
		PodMissingProxy,
		GatewayPortNotOnWorkload,
		IstioProxyImageMismatch,
		SchemaValidationError,
		MisplacedAnnotation,
		UnknownAnnotation,
		ConflictingMeshGatewayVirtualServiceHosts,
		ConflictingSidecarWorkloadSelectors,
		MultipleSidecarsWithoutWorkloadSelectors,
		VirtualServiceDestinationPortSelectorRequired,
		MTLSPolicyConflict,
		DeploymentAssociatedToMultipleServices,
		DeploymentRequiresServiceAssociated,
		PortNameIsNotUnderNamingConvention,
		JwtFailureDueToInvalidServicePortPrefix,
		InvalidRegexp,
		NamespaceMultipleInjectionLabels,
		InvalidAnnotation,
		UnknownMeshNetworksServiceRegistry,
		NoMatchingWorkloadsFound,
		NoServerCertificateVerificationDestinationLevel,
		NoServerCertificateVerificationPortLevel,
		VirtualServiceUnreachableRule,
		VirtualServiceIneffectiveMatch,
		VirtualServiceHostNotFoundInGateway,
		SchemaWarning,
		ServiceEntryAddressesRequired,
		DeprecatedAnnotation,
		AlphaAnnotation,
		DeploymentConflictingPorts,
		GatewayDuplicateCertificate,
		InvalidWebhook,
	}
}

// NewInternalError returns a new diag.Message based on InternalError.
func NewInternalError(r *resource.Instance, detail string) diag.Message {
	return diag.NewMessage(
		InternalError,
		r,
		detail,
	)
}

// NewDeprecated returns a new diag.Message based on Deprecated.
func NewDeprecated(r *resource.Instance, detail string) diag.Message {
	return diag.NewMessage(
		Deprecated,
		r,
		detail,
	)
}

// NewReferencedResourceNotFound returns a new diag.Message based on ReferencedResourceNotFound.
func NewReferencedResourceNotFound(r *resource.Instance, reftype string, refval string) diag.Message {
	return diag.NewMessage(
		ReferencedResourceNotFound,
		r,
		reftype,
		refval,
	)
}

// NewNamespaceNotInjected returns a new diag.Message based on NamespaceNotInjected.
func NewNamespaceNotInjected(r *resource.Instance, namespace string, namespace2 string) diag.Message {
	return diag.NewMessage(
		NamespaceNotInjected,
		r,
		namespace,
		namespace2,
	)
}

// NewPodMissingProxy returns a new diag.Message based on PodMissingProxy.
func NewPodMissingProxy(r *resource.Instance) diag.Message {
	return diag.NewMessage(
		PodMissingProxy,
		r,
	)
}

// NewGatewayPortNotOnWorkload returns a new diag.Message based on GatewayPortNotOnWorkload.
func NewGatewayPortNotOnWorkload(r *resource.Instance, selector string, port int) diag.Message {
	return diag.NewMessage(
		GatewayPortNotOnWorkload,
		r,
		selector,
		port,
	)
}

// NewIstioProxyImageMismatch returns a new diag.Message based on IstioProxyImageMismatch.
func NewIstioProxyImageMismatch(r *resource.Instance, proxyImage string, injectionImage string) diag.Message {
	return diag.NewMessage(
		IstioProxyImageMismatch,
		r,
		proxyImage,
		injectionImage,
	)
}

// NewSchemaValidationError returns a new diag.Message based on SchemaValidationError.
func NewSchemaValidationError(r *resource.Instance, err error) diag.Message {
	return diag.NewMessage(
		SchemaValidationError,
		r,
		err,
	)
}

// NewMisplacedAnnotation returns a new diag.Message based on MisplacedAnnotation.
func NewMisplacedAnnotation(r *resource.Instance, annotation string, kind string) diag.Message {
	return diag.NewMessage(
		MisplacedAnnotation,
		r,
		annotation,
		kind,
	)
}

// NewUnknownAnnotation returns a new diag.Message based on UnknownAnnotation.
func NewUnknownAnnotation(r *resource.Instance, annotation string) diag.Message {
	return diag.NewMessage(
		UnknownAnnotation,
		r,
		annotation,
	)
}

// NewConflictingMeshGatewayVirtualServiceHosts returns a new diag.Message based on ConflictingMeshGatewayVirtualServiceHosts.
func NewConflictingMeshGatewayVirtualServiceHosts(r *resource.Instance, virtualServices string, host string) diag.Message {
	return diag.NewMessage(
		ConflictingMeshGatewayVirtualServiceHosts,
		r,
		virtualServices,
		host,
	)
}

// NewConflictingSidecarWorkloadSelectors returns a new diag.Message based on ConflictingSidecarWorkloadSelectors.
func NewConflictingSidecarWorkloadSelectors(r *resource.Instance, conflictingSidecars []string, namespace string, workloadPod string) diag.Message {
	return diag.NewMessage(
		ConflictingSidecarWorkloadSelectors,
		r,
		conflictingSidecars,
		namespace,
		workloadPod,
	)
}

// NewMultipleSidecarsWithoutWorkloadSelectors returns a new diag.Message based on MultipleSidecarsWithoutWorkloadSelectors.
func NewMultipleSidecarsWithoutWorkloadSelectors(r *resource.Instance, conflictingSidecars []string, namespace string) diag.Message {
	return diag.NewMessage(
		MultipleSidecarsWithoutWorkloadSelectors,
		r,
		conflictingSidecars,
		namespace,
	)
}

// NewVirtualServiceDestinationPortSelectorRequired returns a new diag.Message based on VirtualServiceDestinationPortSelectorRequired.
func NewVirtualServiceDestinationPortSelectorRequired(r *resource.Instance, destHost string, destPorts []int) diag.Message {
	return diag.NewMessage(
		VirtualServiceDestinationPortSelectorRequired,
		r,
		destHost,
		destPorts,
	)
}

// NewMTLSPolicyConflict returns a new diag.Message based on MTLSPolicyConflict.
func NewMTLSPolicyConflict(r *resource.Instance, host string, destinationRuleName string, destinationRuleMTLSMode bool, policyName string, policyMTLSMode string) diag.Message {
	return diag.NewMessage(
		MTLSPolicyConflict,
		r,
		host,
		destinationRuleName,
		destinationRuleMTLSMode,
		policyName,
		policyMTLSMode,
	)
}

// NewDeploymentAssociatedToMultipleServices returns a new diag.Message based on DeploymentAssociatedToMultipleServices.
func NewDeploymentAssociatedToMultipleServices(r *resource.Instance, deployment string, port int32, services []string) diag.Message {
	return diag.NewMessage(
		DeploymentAssociatedToMultipleServices,
		r,
		deployment,
		port,
		services,
	)
}

// NewDeploymentRequiresServiceAssociated returns a new diag.Message based on DeploymentRequiresServiceAssociated.
func NewDeploymentRequiresServiceAssociated(r *resource.Instance) diag.Message {
	return diag.NewMessage(
		DeploymentRequiresServiceAssociated,
		r,
	)
}

// NewPortNameIsNotUnderNamingConvention returns a new diag.Message based on PortNameIsNotUnderNamingConvention.
func NewPortNameIsNotUnderNamingConvention(r *resource.Instance, portName string, port int, targetPort string) diag.Message {
	return diag.NewMessage(
		PortNameIsNotUnderNamingConvention,
		r,
		portName,
		port,
		targetPort,
	)
}

// NewJwtFailureDueToInvalidServicePortPrefix returns a new diag.Message based on JwtFailureDueToInvalidServicePortPrefix.
func NewJwtFailureDueToInvalidServicePortPrefix(r *resource.Instance, port int, portName string, protocol string, targetPort string) diag.Message {
	return diag.NewMessage(
		JwtFailureDueToInvalidServicePortPrefix,
		r,
		port,
		portName,
		protocol,
		targetPort,
	)
}

// NewInvalidRegexp returns a new diag.Message based on InvalidRegexp.
func NewInvalidRegexp(r *resource.Instance, where string, re string, problem string) diag.Message {
	return diag.NewMessage(
		InvalidRegexp,
		r,
		where,
		re,
		problem,
	)
}

// NewNamespaceMultipleInjectionLabels returns a new diag.Message based on NamespaceMultipleInjectionLabels.
func NewNamespaceMultipleInjectionLabels(r *resource.Instance, namespace string, namespace2 string) diag.Message {
	return diag.NewMessage(
		NamespaceMultipleInjectionLabels,
		r,
		namespace,
		namespace2,
	)
}

// NewInvalidAnnotation returns a new diag.Message based on InvalidAnnotation.
func NewInvalidAnnotation(r *resource.Instance, annotation string, problem string) diag.Message {
	return diag.NewMessage(
		InvalidAnnotation,
		r,
		annotation,
		problem,
	)
}

// NewUnknownMeshNetworksServiceRegistry returns a new diag.Message based on UnknownMeshNetworksServiceRegistry.
func NewUnknownMeshNetworksServiceRegistry(r *resource.Instance, serviceregistry string, network string) diag.Message {
	return diag.NewMessage(
		UnknownMeshNetworksServiceRegistry,
		r,
		serviceregistry,
		network,
	)
}

// NewNoMatchingWorkloadsFound returns a new diag.Message based on NoMatchingWorkloadsFound.
func NewNoMatchingWorkloadsFound(r *resource.Instance, labels string) diag.Message {
	return diag.NewMessage(
		NoMatchingWorkloadsFound,
		r,
		labels,
	)
}

// NewNoServerCertificateVerificationDestinationLevel returns a new diag.Message based on NoServerCertificateVerificationDestinationLevel.
func NewNoServerCertificateVerificationDestinationLevel(r *resource.Instance, destinationrule string, namespace string, mode string, host string) diag.Message {
	return diag.NewMessage(
		NoServerCertificateVerificationDestinationLevel,
		r,
		destinationrule,
		namespace,
		mode,
		host,
	)
}

// NewNoServerCertificateVerificationPortLevel returns a new diag.Message based on NoServerCertificateVerificationPortLevel.
func NewNoServerCertificateVerificationPortLevel(r *resource.Instance, destinationrule string, namespace string, mode string, host string, port string) diag.Message {
	return diag.NewMessage(
		NoServerCertificateVerificationPortLevel,
		r,
		destinationrule,
		namespace,
		mode,
		host,
		port,
	)
}

// NewVirtualServiceUnreachableRule returns a new diag.Message based on VirtualServiceUnreachableRule.
func NewVirtualServiceUnreachableRule(r *resource.Instance, ruleno string, reason string) diag.Message {
	return diag.NewMessage(
		VirtualServiceUnreachableRule,
		r,
		ruleno,
		reason,
	)
}

// NewVirtualServiceIneffectiveMatch returns a new diag.Message based on VirtualServiceIneffectiveMatch.
func NewVirtualServiceIneffectiveMatch(r *resource.Instance, ruleno string, matchno string, dupno string) diag.Message {
	return diag.NewMessage(
		VirtualServiceIneffectiveMatch,
		r,
		ruleno,
		matchno,
		dupno,
	)
}

// NewVirtualServiceHostNotFoundInGateway returns a new diag.Message based on VirtualServiceHostNotFoundInGateway.
func NewVirtualServiceHostNotFoundInGateway(r *resource.Instance, host []string, virtualservice string, gateway string) diag.Message {
	return diag.NewMessage(
		VirtualServiceHostNotFoundInGateway,
		r,
		host,
		virtualservice,
		gateway,
	)
}

// NewSchemaWarning returns a new diag.Message based on SchemaWarning.
func NewSchemaWarning(r *resource.Instance, err error) diag.Message {
	return diag.NewMessage(
		SchemaWarning,
		r,
		err,
	)
}

// NewServiceEntryAddressesRequired returns a new diag.Message based on ServiceEntryAddressesRequired.
func NewServiceEntryAddressesRequired(r *resource.Instance) diag.Message {
	return diag.NewMessage(
		ServiceEntryAddressesRequired,
		r,
	)
}

// NewDeprecatedAnnotation returns a new diag.Message based on DeprecatedAnnotation.
func NewDeprecatedAnnotation(r *resource.Instance, annotation string) diag.Message {
	return diag.NewMessage(
		DeprecatedAnnotation,
		r,
		annotation,
	)
}

// NewAlphaAnnotation returns a new diag.Message based on AlphaAnnotation.
func NewAlphaAnnotation(r *resource.Instance, annotation string) diag.Message {
	return diag.NewMessage(
		AlphaAnnotation,
		r,
		annotation,
	)
}

// NewDeploymentConflictingPorts returns a new diag.Message based on DeploymentConflictingPorts.
func NewDeploymentConflictingPorts(r *resource.Instance, deployment string, services []string, targetPort string, ports []int32) diag.Message {
	return diag.NewMessage(
		DeploymentConflictingPorts,
		r,
		deployment,
		services,
		targetPort,
		ports,
	)
}

// NewGatewayDuplicateCertificate returns a new diag.Message based on GatewayDuplicateCertificate.
func NewGatewayDuplicateCertificate(r *resource.Instance, gateways []string) diag.Message {
	return diag.NewMessage(
		GatewayDuplicateCertificate,
		r,
		gateways,
	)
}

// NewInvalidWebhook returns a new diag.Message based on InvalidWebhook.
func NewInvalidWebhook(r *resource.Instance, error string) diag.Message {
	return diag.NewMessage(
		InvalidWebhook,
		r,
		error,
	)
}
