// Copyright 2026 OpenTalon Authors
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

package resources

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1alpha1 "github.com/opentalon/talon-db-k8s-operator/api/v1alpha1"
)

// BuildNetworkPolicy creates a NetworkPolicy for the TalonDB instance.
//
// Default behaviour:
//   - Allow ingress on the gRPC, HTTP and (when enabled) metrics ports.
//   - Allow egress on port 53 (DNS).
//   - User-supplied extra ingress/egress rules are merged in.
func BuildNetworkPolicy(instance *v1alpha1.TalonDB) *networkingv1.NetworkPolicy {
	npSpec := instance.Spec.Networking.NetworkPolicy

	ingressRules := buildDefaultIngressRules(instance)
	ingressRules = append(ingressRules, npSpec.IngressRules...)

	egressRules := buildDefaultEgressRules()
	egressRules = append(egressRules, npSpec.EgressRules...)

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ResourceName(instance),
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: SelectorLabels(instance)},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: ingressRules,
			Egress:  egressRules,
		},
	}
}

func buildDefaultIngressRules(instance *v1alpha1.TalonDB) []networkingv1.NetworkPolicyIngressRule {
	ports := []networkingv1.NetworkPolicyPort{
		{Protocol: protocolTCP(), Port: portPtr(PortFromAddr(instance.Spec.Config.TCP, DefaultGRPCPort))},
		{Protocol: protocolTCP(), Port: portPtr(PortFromAddr(instance.Spec.Config.HTTP, DefaultHTTPPort))},
	}
	if MetricsEnabled(instance) {
		ports = append(ports, networkingv1.NetworkPolicyPort{
			Protocol: protocolTCP(), Port: portPtr(MetricsPort(instance)),
		})
	}
	return []networkingv1.NetworkPolicyIngressRule{{Ports: ports}}
}

func buildDefaultEgressRules() []networkingv1.NetworkPolicyEgressRule {
	tcpProto := corev1.ProtocolTCP
	udpProto := corev1.ProtocolUDP
	dns := intstr.FromInt32(53)
	return []networkingv1.NetworkPolicyEgressRule{
		{
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &tcpProto, Port: &dns},
				{Protocol: &udpProto, Port: &dns},
			},
		},
	}
}

func protocolTCP() *corev1.Protocol {
	p := corev1.ProtocolTCP
	return &p
}

func portPtr(port int32) *intstr.IntOrString {
	v := intstr.FromInt32(port)
	return &v
}
