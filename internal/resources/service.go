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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1alpha1 "github.com/opentalon/talon-db-k8s-operator/api/v1alpha1"
)

// BuildService creates the client-facing Service exposing the gRPC, HTTP and
// (optionally) metrics ports.
func BuildService(instance *v1alpha1.TalonDB) *corev1.Service {
	svcSpec := instance.Spec.Networking.Service
	svcType := svcSpec.Type
	if svcType == "" {
		svcType = corev1.ServiceTypeClusterIP
	}

	labels := Labels(instance)
	svcLabels := make(map[string]string, len(labels)+len(svcSpec.Labels))
	for k, v := range labels {
		svcLabels[k] = v
	}
	for k, v := range svcSpec.Labels {
		svcLabels[k] = v
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ResourceName(instance),
			Namespace:   instance.Namespace,
			Labels:      svcLabels,
			Annotations: svcSpec.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     svcType,
			Selector: SelectorLabels(instance),
			Ports:    buildServicePorts(instance),
		},
	}
}

// BuildHeadlessService creates the headless Service that backs the
// StatefulSet's stable per-pod DNS names.
func BuildHeadlessService(instance *v1alpha1.TalonDB) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      HeadlessServiceName(instance),
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP:                corev1.ClusterIPNone,
			Selector:                 SelectorLabels(instance),
			Ports:                    buildServicePorts(instance),
			PublishNotReadyAddresses: true,
		},
	}
}

func buildServicePorts(instance *v1alpha1.TalonDB) []corev1.ServicePort {
	grpcPort := PortFromAddr(instance.Spec.Config.TCP, DefaultGRPCPort)
	httpPort := PortFromAddr(instance.Spec.Config.HTTP, DefaultHTTPPort)

	ports := []corev1.ServicePort{
		{Name: "grpc", Port: grpcPort, TargetPort: intstr.FromString("grpc"), Protocol: corev1.ProtocolTCP},
		{Name: "http", Port: httpPort, TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP},
	}
	if MetricsEnabled(instance) {
		ports = append(ports, corev1.ServicePort{
			Name: "metrics", Port: MetricsPort(instance), TargetPort: intstr.FromString("metrics"), Protocol: corev1.ProtocolTCP,
		})
	}
	return ports
}
