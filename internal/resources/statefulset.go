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
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1alpha1 "github.com/opentalon/tln-db-k8s-operator/api/v1alpha1"
)

const containerName = "tlndb"

// defaultFSGroup is the distroless "nonroot" GID; the mounted /data volume is
// chowned to this group so the non-root process can write the bbolt file.
const defaultFSGroup int64 = 65532

// stsParams captures the per-role knobs that differ between the
// standalone / leader / follower StatefulSets.
type stsParams struct {
	name         string // StatefulSet name
	role         string // RoleStandalone | RoleLeader | RoleFollower
	replicas     int32
	selectorRole bool            // include the role label in the (immutable) selector
	extraEnv     []corev1.EnvVar // operator-injected env (role, replicate-from, ...)
}

// BuildStatefulSet constructs the standalone StatefulSet for a TlnDB
// instance. configHash is the hash of the rendered config; it is stamped
// on the pod template so config changes trigger a rolling restart.
func BuildStatefulSet(instance *v1alpha1.TlnDB, configHash string) *appsv1.StatefulSet {
	replicas := int32(1)
	if instance.Spec.Replicas != nil {
		replicas = *instance.Spec.Replicas
	}
	return buildStatefulSet(instance, configHash, stsParams{
		name: ResourceName(instance), role: RoleStandalone, replicas: replicas,
	})
}

// BuildLeaderStatefulSet builds the single-writer leader (1 replica) for
// replicated mode. retention bounds the op-log (0 = server default).
func BuildLeaderStatefulSet(instance *v1alpha1.TlnDB, configHash string, retention int64) *appsv1.StatefulSet {
	env := []corev1.EnvVar{{Name: "TLNDB_ROLE", Value: RoleLeader}}
	env = append(env, oplogRetentionEnv(retention)...)
	return buildStatefulSet(instance, configHash, stsParams{
		name: LeaderStatefulSetName(instance), role: RoleLeader, replicas: 1,
		selectorRole: true, extraEnv: env,
	})
}

// BuildFollowerStatefulSet builds the read-only follower set for
// replicated mode. leaderAddr is the gRPC address followers replicate
// from (the write Service).
func BuildFollowerStatefulSet(instance *v1alpha1.TlnDB, configHash, leaderAddr string, replicas int32, retention int64) *appsv1.StatefulSet {
	env := []corev1.EnvVar{
		{Name: "TLNDB_ROLE", Value: RoleFollower},
		{Name: "TLNDB_REPLICATE_FROM", Value: leaderAddr},
	}
	env = append(env, oplogRetentionEnv(retention)...)
	return buildStatefulSet(instance, configHash, stsParams{
		name: FollowerStatefulSetName(instance), role: RoleFollower, replicas: replicas,
		selectorRole: true, extraEnv: env,
	})
}

func oplogRetentionEnv(retention int64) []corev1.EnvVar {
	if retention <= 0 {
		return nil
	}
	return []corev1.EnvVar{{Name: "TLNDB_OPLOG_RETENTION", Value: strconv.FormatInt(retention, 10)}}
}

func buildStatefulSet(instance *v1alpha1.TlnDB, configHash string, p stsParams) *appsv1.StatefulSet {
	replicas := p.replicas

	podAnnotations := map[string]string{ConfigHashAnnotation: configHash}
	for k, v := range instance.Spec.PodAnnotations {
		podAnnotations[k] = v
	}

	selector := SelectorLabels(instance)
	if p.selectorRole {
		selector = roleSelector(instance, p.role)
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.name,
			Namespace: instance.Namespace,
			Labels:    withRole(Labels(instance), p.role),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:            &replicas,
			ServiceName:         HeadlessServiceName(instance),
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      withRole(Labels(instance), p.role),
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName(instance),
					SecurityContext:    buildPodSecurityContext(instance),
					ImagePullSecrets:   instance.Spec.Image.PullSecrets,
					NodeSelector:       instance.Spec.NodeSelector,
					Tolerations:        instance.Spec.Tolerations,
					Affinity:           instance.Spec.Affinity,
					InitContainers:     instance.Spec.InitContainers,
					Containers:         buildContainers(instance, p.extraEnv),
					Volumes:            buildVolumes(instance),
				},
			},
		},
	}

	// Provision a PVC per pod via VolumeClaimTemplates unless persistence is
	// disabled or an existing claim is referenced (both handled in buildVolumes).
	if persistenceEnabled(instance) && instance.Spec.Storage.Persistence.ExistingClaim == "" {
		sts.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{buildVolumeClaimTemplate(instance)}
	}

	return sts
}

func buildContainers(instance *v1alpha1.TlnDB, extraEnv []corev1.EnvVar) []corev1.Container {
	main := buildMainContainer(instance, extraEnv)
	containers := []corev1.Container{main}
	containers = append(containers, instance.Spec.AdditionalContainers...)
	return containers
}

func buildMainContainer(instance *v1alpha1.TlnDB, extraEnv []corev1.EnvVar) corev1.Container {
	httpPort := PortFromAddr(instance.Spec.Config.HTTP, DefaultHTTPPort)
	grpcPort := PortFromAddr(instance.Spec.Config.TCP, DefaultGRPCPort)

	ports := []corev1.ContainerPort{
		{Name: "grpc", ContainerPort: grpcPort, Protocol: corev1.ProtocolTCP},
		{Name: "http", ContainerPort: httpPort, Protocol: corev1.ProtocolTCP},
	}
	if MetricsEnabled(instance) {
		ports = append(ports, corev1.ContainerPort{
			Name: "metrics", ContainerPort: MetricsPort(instance), Protocol: corev1.ProtocolTCP,
		})
	}

	volumeMounts := []corev1.VolumeMount{
		{Name: ConfigVolumeName, MountPath: ConfigMountPath, ReadOnly: true},
		{Name: DataVolumeName, MountPath: DataMountPath},
	}
	volumeMounts = append(volumeMounts, instance.Spec.AdditionalVolumeMounts...)

	c := corev1.Container{
		Name:            containerName,
		Image:           ImageRef(instance.Spec.Image),
		ImagePullPolicy: instance.Spec.Image.PullPolicy,
		Args:            []string{"--config", ConfigMountPath + "/" + ConfigFileName},
		Ports:           ports,
		Env:             append(append([]corev1.EnvVar(nil), instance.Spec.Env...), extraEnv...),
		EnvFrom:         instance.Spec.EnvFrom,
		Resources:       instance.Spec.Resources,
		VolumeMounts:    volumeMounts,
		SecurityContext: buildContainerSecurityContext(instance),
		LivenessProbe:   buildProbe(instance, httpPort, false),
		ReadinessProbe:  buildProbe(instance, httpPort, false),
		StartupProbe:    buildProbe(instance, httpPort, true),
	}
	return c
}

// buildProbe returns an HTTP GET /v1/health probe on the HTTP port. tln-db's
// gRPC Health is a custom method (not grpc.health.v1), so HTTP is used.
func buildProbe(instance *v1alpha1.TlnDB, httpPort int32, startup bool) *corev1.Probe {
	handler := corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path: "/v1/health",
			Port: intstr.FromInt32(httpPort),
		},
	}
	if startup {
		timeout := instance.Spec.Observability.Health.StartupTimeoutSeconds
		if timeout <= 0 {
			timeout = 600
		}
		period := int32(5)
		failures := timeout / period
		if failures < 1 {
			failures = 1
		}
		return &corev1.Probe{
			ProbeHandler:     handler,
			PeriodSeconds:    period,
			TimeoutSeconds:   3,
			FailureThreshold: failures,
		}
	}

	initialDelay := instance.Spec.Observability.Health.InitialDelaySeconds
	if initialDelay <= 0 {
		initialDelay = 10
	}
	return &corev1.Probe{
		ProbeHandler:        handler,
		InitialDelaySeconds: initialDelay,
		PeriodSeconds:       10,
		TimeoutSeconds:      3,
		FailureThreshold:    3,
	}
}

func buildVolumes(instance *v1alpha1.TlnDB) []corev1.Volume {
	// Config volume: rendered ConfigMap, or an external one via ConfigFrom.
	configMapName := ConfigMapName(instance)
	if instance.Spec.ConfigFrom != nil && instance.Spec.ConfigFrom.Name != "" {
		configMapName = instance.Spec.ConfigFrom.Name
	}
	defaultMode := int32(0o444)
	volumes := []corev1.Volume{
		{
			Name: ConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
					DefaultMode:          &defaultMode,
				},
			},
		},
	}

	// Data volume. When using VolumeClaimTemplates the StatefulSet supplies the
	// "data" volume automatically, so only add one here for the emptyDir and
	// existing-claim cases.
	switch {
	case !persistenceEnabled(instance):
		volumes = append(volumes, corev1.Volume{
			Name:         DataVolumeName,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
	case instance.Spec.Storage.Persistence.ExistingClaim != "":
		volumes = append(volumes, corev1.Volume{
			Name: DataVolumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: instance.Spec.Storage.Persistence.ExistingClaim,
				},
			},
		})
	}

	volumes = append(volumes, instance.Spec.AdditionalVolumes...)
	return volumes
}

func buildVolumeClaimTemplate(instance *v1alpha1.TlnDB) corev1.PersistentVolumeClaim {
	persistence := instance.Spec.Storage.Persistence

	size := persistence.Size
	if size.IsZero() {
		size = resource.MustParse("1Gi")
	}
	accessModes := persistence.AccessModes
	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	pvc := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:   DataVolumeName,
			Labels: Labels(instance),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: size},
			},
		},
	}
	if persistence.StorageClassName != nil {
		pvc.Spec.StorageClassName = persistence.StorageClassName
	}
	return pvc
}

func persistenceEnabled(instance *v1alpha1.TlnDB) bool {
	e := instance.Spec.Storage.Persistence.Enabled
	return e == nil || *e
}

func serviceAccountName(instance *v1alpha1.TlnDB) string {
	if instance.Spec.ServiceAccountName != "" {
		return instance.Spec.ServiceAccountName
	}
	return ResourceName(instance)
}

func buildPodSecurityContext(instance *v1alpha1.TlnDB) *corev1.PodSecurityContext {
	if instance.Spec.Security.PodSecurityContext != nil {
		return instance.Spec.Security.PodSecurityContext
	}
	fsGroup := defaultFSGroup
	if instance.Spec.Security.FSGroup != nil {
		fsGroup = *instance.Spec.Security.FSGroup
	}
	return &corev1.PodSecurityContext{
		RunAsNonRoot:   boolPtr(true),
		FSGroup:        &fsGroup,
		SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
	}
}

func buildContainerSecurityContext(instance *v1alpha1.TlnDB) *corev1.SecurityContext {
	if instance.Spec.Security.ContainerSecurityContext != nil {
		return instance.Spec.Security.ContainerSecurityContext
	}
	return &corev1.SecurityContext{
		RunAsNonRoot:             boolPtr(true),
		AllowPrivilegeEscalation: boolPtr(false),
		ReadOnlyRootFilesystem:   boolPtr(true),
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
	}
}
