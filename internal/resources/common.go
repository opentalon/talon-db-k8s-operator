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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	v1alpha1 "github.com/opentalon/talon-db-k8s-operator/api/v1alpha1"
)

const (
	// DefaultImage is the default talon-db container image repository.
	DefaultImage = "ghcr.io/opentalon/talon-db"
	// DefaultTag is the default talon-db image tag.
	DefaultTag = "latest"
	// ManagedByLabel is the standard label value indicating operator ownership.
	ManagedByLabel = "talon-db-operator"
	// FinalizerName is the finalizer added to TalonDB resources.
	FinalizerName = "db.opentalon.io/finalizer"

	// ConfigHashAnnotation carries the rendered-config hash on the pod template
	// so config changes trigger a rolling restart.
	ConfigHashAnnotation = "db.opentalon.io/config-hash"

	// ConfigVolumeName / ConfigMountPath are where the rendered config.yaml is mounted.
	ConfigVolumeName = "config"
	ConfigMountPath  = "/etc/talondb"
	ConfigFileName   = "config.yaml"

	// DataVolumeName / DataMountPath hold the bbolt data file.
	DataVolumeName = "data"
	DataMountPath  = "/data"

	// Default listener ports (used when addresses cannot be parsed).
	DefaultGRPCPort    int32 = 9899
	DefaultHTTPPort    int32 = 8080
	DefaultMetricsPort int32 = 9090
)

// ResourceName returns the canonical base name for all resources managed for
// the given instance. Child resources share this name.
func ResourceName(instance *v1alpha1.TalonDB) string {
	return instance.Name
}

// ConfigMapName returns the name of the rendered config ConfigMap.
func ConfigMapName(instance *v1alpha1.TalonDB) string {
	return instance.Name + "-config"
}

// HeadlessServiceName returns the name of the headless Service backing the
// StatefulSet's stable pod DNS.
func HeadlessServiceName(instance *v1alpha1.TalonDB) string {
	return instance.Name + "-headless"
}

// Labels returns the recommended Kubernetes labels for resources owned by instance.
func Labels(instance *v1alpha1.TalonDB) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "talon-db",
		"app.kubernetes.io/instance":   instance.Name,
		"app.kubernetes.io/component":  "database",
		"app.kubernetes.io/part-of":    "talon-db",
		"app.kubernetes.io/managed-by": ManagedByLabel,
	}
}

// SelectorLabels returns the minimal, stable set of labels used as a pod selector.
func SelectorLabels(instance *v1alpha1.TalonDB) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "talon-db",
		"app.kubernetes.io/instance": instance.Name,
	}
}

// ImageRef builds the fully-qualified image reference. A digest takes
// precedence over the tag; defaults fill in when unset.
func ImageRef(spec v1alpha1.ImageSpec) string {
	repo := spec.Repository
	if repo == "" {
		repo = DefaultImage
	}
	if spec.Digest != "" {
		return fmt.Sprintf("%s@%s", repo, spec.Digest)
	}
	tag := spec.Tag
	if tag == "" {
		tag = DefaultTag
	}
	return fmt.Sprintf("%s:%s", repo, tag)
}

// PortFromAddr extracts the numeric port from a listen address such as ":9899"
// or "0.0.0.0:9899". It returns def when the address is empty or unparseable.
func PortFromAddr(addr string, def int32) int32 {
	if addr == "" {
		return def
	}
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return def
	}
	p, err := strconv.Atoi(addr[idx+1:])
	if err != nil || p <= 0 || p > 65535 {
		return def
	}
	return int32(p)
}

// MetricsEnabled reports whether Prometheus metrics are enabled (default true).
func MetricsEnabled(instance *v1alpha1.TalonDB) bool {
	e := instance.Spec.Observability.Metrics.Enabled
	return e == nil || *e
}

// MetricsPort returns the configured metrics port (default 9090).
func MetricsPort(instance *v1alpha1.TalonDB) int32 {
	if p := instance.Spec.Observability.Metrics.Port; p > 0 {
		return p
	}
	return DefaultMetricsPort
}

// HashSecretData computes a deterministic SHA-256 hex digest of the map.
// Keys are sorted so the result is stable regardless of insertion order.
func HashSecretData(data map[string][]byte) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte("="))
		h.Write(data[k])
		h.Write([]byte("\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// HashStringData computes a deterministic SHA-256 hex digest of string map data.
func HashStringData(data map[string]string) string {
	raw := make(map[string][]byte, len(data))
	for k, v := range data {
		raw[k] = []byte(v)
	}
	return HashSecretData(raw)
}

func boolPtr(b bool) *bool { return &b }
