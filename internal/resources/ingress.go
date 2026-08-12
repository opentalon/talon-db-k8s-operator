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
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/opentalon/talon-db-k8s-operator/api/v1alpha1"
)

// BuildIngress creates an Ingress routing to the talon-db HTTP/JSON port.
func BuildIngress(instance *v1alpha1.TalonDB) *networkingv1.Ingress {
	ingressSpec := instance.Spec.Networking.Ingress

	httpPort := PortFromAddr(instance.Spec.Config.HTTP, DefaultHTTPPort)

	path := ingressSpec.Path
	if path == "" {
		path = "/"
	}
	pathType := networkingv1.PathTypePrefix

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ResourceName(instance),
			Namespace:   instance.Namespace,
			Labels:      Labels(instance),
			Annotations: ingressSpec.Annotations,
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: ingressSpec.ClassName,
			Rules: []networkingv1.IngressRule{
				{
					Host: ingressSpec.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     path,
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: ResourceName(instance),
											Port: networkingv1.ServiceBackendPort{Number: httpPort},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if ingressSpec.TLSSecretName != "" {
		ingress.Spec.TLS = []networkingv1.IngressTLS{
			{Hosts: []string{ingressSpec.Host}, SecretName: ingressSpec.TLSSecretName},
		}
	}

	return ingress
}
