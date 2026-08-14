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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Phase constants describe the high-level lifecycle state of a TlnDB.
const (
	PhasePending      = "Pending"
	PhaseProvisioning = "Provisioning"
	PhaseRunning      = "Running"
	PhaseDegraded     = "Degraded"
	PhaseFailed       = "Failed"
	PhaseTerminating  = "Terminating"
)

// Condition type constants tracked in status.conditions.
const (
	ConditionStatefulSetReady    = "StatefulSetReady"
	ConditionConfigMapReady      = "ConfigMapReady"
	ConditionServiceReady        = "ServiceReady"
	ConditionRBACReady           = "RBACReady"
	ConditionNetworkPolicyReady  = "NetworkPolicyReady"
	ConditionServiceMonitorReady = "ServiceMonitorReady"
)

// ImageSpec configures the tln-db container image.
type ImageSpec struct {
	// Repository is the image repository, e.g. ghcr.io/opentalon/tln-db.
	// +optional
	Repository string `json:"repository,omitempty"`
	// Tag is the image tag. Ignored when Digest is set.
	// +optional
	Tag string `json:"tag,omitempty"`
	// Digest pins the image by digest (repo@sha256:...); takes precedence over Tag.
	// +optional
	Digest string `json:"digest,omitempty"`
	// PullPolicy is the image pull policy.
	// +optional
	// +kubebuilder:validation:Enum=Always;IfNotPresent;Never
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
	// PullSecrets references image pull secrets in the instance namespace.
	// +optional
	PullSecrets []corev1.LocalObjectReference `json:"pullSecrets,omitempty"`
}

// TlnDBConfig maps to the tlndb-server config.yaml that the operator
// renders into a ConfigMap and mounts at /etc/tlndb/config.yaml.
type TlnDBConfig struct {
	// DBPath is the path to the bbolt data file inside the container.
	// +optional
	// +kubebuilder:default="/data/tlndb.bbolt"
	DBPath string `json:"dbPath,omitempty"`
	// TCP is the gRPC listen address, e.g. ":9899".
	// +optional
	// +kubebuilder:default=":9899"
	TCP string `json:"tcp,omitempty"`
	// HTTP is the HTTP/JSON listen address, e.g. ":8080". It must stay set
	// because the health probes target GET /v1/health on this port.
	// +optional
	// +kubebuilder:default=":8080"
	// +kubebuilder:validation:MinLength=1
	HTTP string `json:"http,omitempty"`
	// Socket is an optional Unix-socket path for gRPC.
	// +optional
	Socket string `json:"socket,omitempty"`
	// ExtraConfig is raw YAML appended verbatim to the rendered config file.
	// +optional
	ExtraConfig string `json:"extraConfig,omitempty"`
}

// PersistenceSpec configures the /data persistent volume.
type PersistenceSpec struct {
	// Enabled turns on a PersistentVolumeClaim for /data. Defaults to true.
	// When false, an emptyDir is used (data is lost on pod restart).
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`
	// Size is the requested storage size, e.g. "1Gi".
	// +optional
	Size resource.Quantity `json:"size,omitempty"`
	// StorageClassName overrides the default storage class.
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`
	// AccessModes for the PVC. Defaults to [ReadWriteOnce].
	// +optional
	AccessModes []corev1.PersistentVolumeAccessMode `json:"accessModes,omitempty"`
	// ExistingClaim mounts a pre-existing PVC instead of provisioning one.
	// +optional
	ExistingClaim string `json:"existingClaim,omitempty"`
}

// StorageSpec groups storage configuration.
type StorageSpec struct {
	// +optional
	Persistence PersistenceSpec `json:"persistence,omitempty"`
}

// ReplicationSpec configures replicated mode (one leader + N followers).
type ReplicationSpec struct {
	// ReadReplicas is the number of follower pods. 0 runs a leader only
	// (the read Service then falls back to the leader).
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	ReadReplicas int32 `json:"readReplicas,omitempty"`
	// OplogRetention bounds the leader's op-log (max entries kept). 0 uses
	// the server default (100000). A follower behind the retained window
	// re-bootstraps from a fresh snapshot.
	// +optional
	// +kubebuilder:validation:Minimum=0
	OplogRetention int64 `json:"oplogRetention,omitempty"`
}

// ServiceSpec configures the client-facing Service.
type ServiceSpec struct {
	// +optional
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	Type corev1.ServiceType `json:"type,omitempty"`
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// IngressSpec configures an optional Ingress for the HTTP port.
type IngressSpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// +optional
	ClassName *string `json:"className,omitempty"`
	// +optional
	Host string `json:"host,omitempty"`
	// +optional
	// +kubebuilder:default="/"
	Path string `json:"path,omitempty"`
	// +optional
	TLSSecretName string `json:"tlsSecretName,omitempty"`
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// NetworkPolicySpec configures an optional NetworkPolicy.
type NetworkPolicySpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// IngressRules are extra ingress rules merged onto the defaults.
	// +optional
	IngressRules []networkingv1.NetworkPolicyIngressRule `json:"ingressRules,omitempty"`
	// EgressRules are extra egress rules merged onto the defaults.
	// +optional
	EgressRules []networkingv1.NetworkPolicyEgressRule `json:"egressRules,omitempty"`
}

// NetworkingSpec groups Service/Ingress/NetworkPolicy configuration.
type NetworkingSpec struct {
	// +optional
	Service ServiceSpec `json:"service,omitempty"`
	// +optional
	Ingress IngressSpec `json:"ingress,omitempty"`
	// +optional
	NetworkPolicy NetworkPolicySpec `json:"networkPolicy,omitempty"`
}

// ServiceMonitorSpec configures a Prometheus Operator ServiceMonitor.
type ServiceMonitorSpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Interval is the scrape interval, e.g. "30s".
	// +optional
	Interval string `json:"interval,omitempty"`
	// Labels are extra labels added to the ServiceMonitor (e.g. release: kube-prometheus).
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// MetricsSpec configures Prometheus metrics exposure.
type MetricsSpec struct {
	// Enabled turns on the tln-db Prometheus /metrics listener and the
	// corresponding Service port. Defaults to true.
	// +optional
	// +kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`
	// Port is the metrics listen port.
	// +optional
	// +kubebuilder:default=9090
	Port int32 `json:"port,omitempty"`
	// Path is the metrics HTTP path.
	// +optional
	// +kubebuilder:default="/metrics"
	Path string `json:"path,omitempty"`
	// +optional
	ServiceMonitor ServiceMonitorSpec `json:"serviceMonitor,omitempty"`
}

// HealthSpec tunes the health probes (targeting GET /v1/health on the HTTP port).
type HealthSpec struct {
	// +optional
	// +kubebuilder:default=10
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`
	// StartupTimeoutSeconds bounds how long the startup probe waits for the DB
	// to open (bbolt recovery / index rebuild on large files).
	// +optional
	// +kubebuilder:default=600
	StartupTimeoutSeconds int32 `json:"startupTimeoutSeconds,omitempty"`
}

// ObservabilitySpec groups metrics and health configuration.
type ObservabilitySpec struct {
	// +optional
	Metrics MetricsSpec `json:"metrics,omitempty"`
	// +optional
	Health HealthSpec `json:"health,omitempty"`
}

// PDBSpec configures an optional PodDisruptionBudget.
type PDBSpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// +optional
	MinAvailable *int32 `json:"minAvailable,omitempty"`
	// +optional
	MaxUnavailable *int32 `json:"maxUnavailable,omitempty"`
}

// HPASpec configures an optional HorizontalPodAutoscaler.
type HPASpec struct {
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	// +optional
	MaxReplicas int32 `json:"maxReplicas,omitempty"`
	// +optional
	CPUUtilization *int32 `json:"cpuUtilization,omitempty"`
	// +optional
	MemoryUtilization *int32 `json:"memoryUtilization,omitempty"`
}

// AvailabilitySpec groups PDB and HPA configuration.
type AvailabilitySpec struct {
	// +optional
	PodDisruptionBudget PDBSpec `json:"podDisruptionBudget,omitempty"`
	// +optional
	HorizontalPodAutoscaler HPASpec `json:"horizontalPodAutoscaler,omitempty"`
}

// SecuritySpec configures pod and container security contexts.
type SecuritySpec struct {
	// PodSecurityContext fully overrides the pod-level security context.
	// +optional
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`
	// ContainerSecurityContext fully overrides the container-level security context.
	// +optional
	ContainerSecurityContext *corev1.SecurityContext `json:"containerSecurityContext,omitempty"`
	// FSGroup sets the pod fsGroup so the mounted /data volume is writable by
	// the non-root user. Defaults to 65532 (distroless nonroot).
	// +optional
	FSGroup *int64 `json:"fsGroup,omitempty"`
}

// TlnDBSpec defines the desired state of a TlnDB instance.
//
// tln-db is a single-node, bbolt-backed database. Running more than one
// replica creates independent databases (each pod owns its own volume); it is
// NOT a replicated/HA cluster.
type TlnDBSpec struct {
	// Mode selects the topology. "standalone" (default) runs a single
	// StatefulSet. "replicated" runs a single-writer leader plus N
	// read-only follower replicas that stream the leader's op-log.
	// +optional
	// +kubebuilder:validation:Enum=standalone;replicated
	// +kubebuilder:default=standalone
	Mode string `json:"mode,omitempty"`

	// Replication configures the replicated mode (ignored when standalone).
	// +optional
	Replication ReplicationSpec `json:"replication,omitempty"`

	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// Replicas is the number of pods. Defaults to 1. Values >1 create
	// independent bbolt instances, not a replicated cluster.
	// +optional
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	Replicas *int32 `json:"replicas,omitempty"`

	// +optional
	Config TlnDBConfig `json:"config,omitempty"`

	// ConfigFrom mounts an externally-managed ConfigMap (key config.yaml)
	// instead of the operator-rendered one.
	// +optional
	ConfigFrom *corev1.LocalObjectReference `json:"configFrom,omitempty"`

	// Env injects environment variables into the container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// EnvFrom injects environment variables from Secrets/ConfigMaps
	// (e.g. TLNDB_* values or sensitive settings).
	// +optional
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// +optional
	Storage StorageSpec `json:"storage,omitempty"`

	// +optional
	Networking NetworkingSpec `json:"networking,omitempty"`

	// +optional
	Security SecuritySpec `json:"security,omitempty"`

	// +optional
	Observability ObservabilitySpec `json:"observability,omitempty"`

	// +optional
	Availability AvailabilitySpec `json:"availability,omitempty"`

	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`
	// ServiceAccountName uses an existing ServiceAccount instead of creating one.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// +optional
	InitContainers []corev1.Container `json:"initContainers,omitempty"`
	// +optional
	AdditionalContainers []corev1.Container `json:"additionalContainers,omitempty"`
	// +optional
	AdditionalVolumes []corev1.Volume `json:"additionalVolumes,omitempty"`
	// +optional
	AdditionalVolumeMounts []corev1.VolumeMount `json:"additionalVolumeMounts,omitempty"`
}

// TlnDBStatus defines the observed state of a TlnDB instance.
type TlnDBStatus struct {
	// +optional
	Phase string `json:"phase,omitempty"`
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	// +optional
	CurrentImage string `json:"currentImage,omitempty"`
	// +optional
	ManagedResources []string `json:"managedResources,omitempty"`
	// +optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=tdb,categories=opentalon
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=".status.readyReplicas"
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=".status.currentImage"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// TlnDB is the Schema for the tlndbs API.
type TlnDB struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TlnDBSpec   `json:"spec,omitempty"`
	Status TlnDBStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TlnDBList contains a list of TlnDB.
type TlnDBList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TlnDB `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TlnDB{}, &TlnDBList{})
}
