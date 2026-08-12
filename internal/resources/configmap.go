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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/opentalon/talon-db-k8s-operator/api/v1alpha1"
)

// BuildConfigMap renders the talondb-server config.yaml into a ConfigMap.
func BuildConfigMap(instance *v1alpha1.TalonDB) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName(instance),
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
		},
		Data: map[string]string{
			ConfigFileName: renderConfigYAML(instance),
		},
	}
}

// renderConfigYAML produces the YAML consumed by talondb-server via --config.
// Keys mirror the server's serverConfig struct (db/tcp/http/socket/metrics).
func renderConfigYAML(instance *v1alpha1.TalonDB) string {
	cfg := instance.Spec.Config

	dbPath := cfg.DBPath
	if dbPath == "" {
		dbPath = DataMountPath + "/talondb.bbolt"
	}
	tcp := cfg.TCP
	if tcp == "" {
		tcp = fmt.Sprintf(":%d", DefaultGRPCPort)
	}
	httpAddr := cfg.HTTP
	if httpAddr == "" {
		httpAddr = fmt.Sprintf(":%d", DefaultHTTPPort)
	}

	var b strings.Builder
	b.WriteString("# Rendered by talon-db-operator. Do not edit.\n")
	fmt.Fprintf(&b, "db: %q\n", dbPath)
	fmt.Fprintf(&b, "tcp: %q\n", tcp)
	fmt.Fprintf(&b, "http: %q\n", httpAddr)
	if cfg.Socket != "" {
		fmt.Fprintf(&b, "socket: %q\n", cfg.Socket)
	}
	if MetricsEnabled(instance) {
		fmt.Fprintf(&b, "metrics: %q\n", fmt.Sprintf(":%d", MetricsPort(instance)))
	}
	if extra := strings.TrimSpace(cfg.ExtraConfig); extra != "" {
		b.WriteString(extra)
		b.WriteString("\n")
	}
	return b.String()
}
