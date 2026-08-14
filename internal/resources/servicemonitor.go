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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1alpha1 "github.com/opentalon/tln-db-k8s-operator/api/v1alpha1"
)

// Prometheus Operator ServiceMonitor CRD identifiers.
const (
	ServiceMonitorGroup   = "monitoring.coreos.com"
	ServiceMonitorVersion = "v1"
	ServiceMonitorKind    = "ServiceMonitor"
)

// BuildServiceMonitor returns an unstructured ServiceMonitor scraping the
// tln-db metrics port. Using unstructured avoids a hard dependency on the
// prometheus-operator Go types, so the operator still runs when the CRD is
// absent (the controller degrades gracefully in that case).
func BuildServiceMonitor(instance *v1alpha1.TlnDB) *unstructured.Unstructured {
	smSpec := instance.Spec.Observability.Metrics.ServiceMonitor

	interval := smSpec.Interval
	if interval == "" {
		interval = "30s"
	}
	metricsPath := instance.Spec.Observability.Metrics.Path
	if metricsPath == "" {
		metricsPath = "/metrics"
	}

	matchLabels := Labels(instance)
	smLabels := Labels(instance)
	for k, v := range smSpec.Labels {
		smLabels[k] = v
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": ServiceMonitorGroup + "/" + ServiceMonitorVersion,
			"kind":       ServiceMonitorKind,
			"metadata": map[string]interface{}{
				"name":      ResourceName(instance),
				"namespace": instance.Namespace,
				"labels":    toStringMap(smLabels),
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"matchLabels": toStringMap(matchLabels),
				},
				"namespaceSelector": map[string]interface{}{
					"matchNames": []interface{}{instance.Namespace},
				},
				"endpoints": []interface{}{
					map[string]interface{}{
						"port":     "metrics",
						"path":     metricsPath,
						"interval": interval,
						"scheme":   "http",
					},
				},
			},
		},
	}
}

func toStringMap(m map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
