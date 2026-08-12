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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/opentalon/talon-db-k8s-operator/api/v1alpha1"
)

func newInstance() *v1alpha1.TalonDB {
	return &v1alpha1.TalonDB{
		ObjectMeta: metav1.ObjectMeta{Name: "db1", Namespace: "ns1"},
	}
}

func TestImageRef(t *testing.T) {
	cases := []struct {
		spec v1alpha1.ImageSpec
		want string
	}{
		{v1alpha1.ImageSpec{}, "ghcr.io/opentalon/talon-db:latest"},
		{v1alpha1.ImageSpec{Tag: "v1"}, "ghcr.io/opentalon/talon-db:v1"},
		{v1alpha1.ImageSpec{Repository: "r", Tag: "v1"}, "r:v1"},
		{v1alpha1.ImageSpec{Repository: "r", Digest: "sha256:abc", Tag: "v1"}, "r@sha256:abc"},
	}
	for _, c := range cases {
		if got := ImageRef(c.spec); got != c.want {
			t.Errorf("ImageRef(%+v) = %q, want %q", c.spec, got, c.want)
		}
	}
}

func TestPortFromAddr(t *testing.T) {
	cases := []struct {
		addr string
		def  int32
		want int32
	}{
		{":9899", 1, 9899},
		{"0.0.0.0:8080", 1, 8080},
		{"", 7, 7},
		{"garbage", 7, 7},
		{":99999", 7, 7},
	}
	for _, c := range cases {
		if got := PortFromAddr(c.addr, c.def); got != c.want {
			t.Errorf("PortFromAddr(%q,%d) = %d, want %d", c.addr, c.def, got, c.want)
		}
	}
}

func TestRenderConfigYAML(t *testing.T) {
	inst := newInstance()
	inst.Spec.Config = v1alpha1.TalonDBConfig{
		DBPath:      "/data/x.bbolt",
		TCP:         ":9899",
		HTTP:        ":8080",
		ExtraConfig: "# extra",
	}
	out := renderConfigYAML(inst)
	for _, want := range []string{`db: "/data/x.bbolt"`, `tcp: ":9899"`, `http: ":8080"`, `metrics: ":9090"`, "# extra"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered config missing %q\n%s", want, out)
		}
	}

	// Metrics disabled → no metrics line.
	dis := false
	inst.Spec.Observability.Metrics.Enabled = &dis
	if strings.Contains(renderConfigYAML(inst), "metrics:") {
		t.Error("metrics line present when metrics disabled")
	}
}

func TestBuildStatefulSet_PortsProbesVolumes(t *testing.T) {
	inst := newInstance()
	inst.Spec.Config = v1alpha1.TalonDBConfig{TCP: ":9899", HTTP: ":8080"}

	sts := BuildStatefulSet(inst, "hash123")

	if got := sts.Spec.Template.Annotations[ConfigHashAnnotation]; got != "hash123" {
		t.Errorf("config hash annotation = %q, want hash123", got)
	}
	if len(sts.Spec.VolumeClaimTemplates) != 1 {
		t.Fatalf("expected 1 VolumeClaimTemplate, got %d", len(sts.Spec.VolumeClaimTemplates))
	}

	c := sts.Spec.Template.Spec.Containers[0]
	wantPorts := map[string]int32{"grpc": 9899, "http": 8080, "metrics": 9090}
	got := map[string]int32{}
	for _, p := range c.Ports {
		got[p.Name] = p.ContainerPort
	}
	for name, port := range wantPorts {
		if got[name] != port {
			t.Errorf("port %q = %d, want %d", name, got[name], port)
		}
	}

	if c.ReadinessProbe == nil || c.ReadinessProbe.HTTPGet == nil || c.ReadinessProbe.HTTPGet.Path != "/v1/health" {
		t.Errorf("readiness probe not GET /v1/health: %+v", c.ReadinessProbe)
	}
	if c.LivenessProbe == nil || c.StartupProbe == nil {
		t.Error("liveness/startup probes must be set")
	}
	if c.SecurityContext == nil || c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("expected readOnlyRootFilesystem=true")
	}

	// config + data volume mounts present.
	mounts := map[string]bool{}
	for _, m := range c.VolumeMounts {
		mounts[m.Name] = true
	}
	if !mounts[ConfigVolumeName] || !mounts[DataVolumeName] {
		t.Errorf("expected config and data mounts, got %+v", c.VolumeMounts)
	}
}

func TestBuildStatefulSet_EmptyDirWhenPersistenceDisabled(t *testing.T) {
	inst := newInstance()
	disabled := false
	inst.Spec.Storage.Persistence.Enabled = &disabled
	sts := BuildStatefulSet(inst, "h")
	if len(sts.Spec.VolumeClaimTemplates) != 0 {
		t.Error("no VolumeClaimTemplates expected when persistence disabled")
	}
	found := false
	for _, v := range sts.Spec.Template.Spec.Volumes {
		if v.Name == DataVolumeName && v.EmptyDir != nil {
			found = true
		}
	}
	if !found {
		t.Error("expected emptyDir data volume when persistence disabled")
	}
}

func TestBuildStatefulSet_ExistingClaim(t *testing.T) {
	inst := newInstance()
	inst.Spec.Storage.Persistence.ExistingClaim = "my-claim"
	sts := BuildStatefulSet(inst, "h")
	if len(sts.Spec.VolumeClaimTemplates) != 0 {
		t.Error("no VolumeClaimTemplates expected with existingClaim")
	}
	found := false
	for _, v := range sts.Spec.Template.Spec.Volumes {
		if v.Name == DataVolumeName && v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == "my-claim" {
			found = true
		}
	}
	if !found {
		t.Error("expected data volume bound to existing claim")
	}
}

func TestBuildStatefulSet_EnvPassthrough(t *testing.T) {
	inst := newInstance()
	inst.Spec.Env = []corev1.EnvVar{{Name: "FOO", Value: "bar"}}
	inst.Spec.EnvFrom = []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "s"}}}}
	c := BuildStatefulSet(inst, "h").Spec.Template.Spec.Containers[0]
	if len(c.Env) != 1 || c.Env[0].Name != "FOO" {
		t.Errorf("env not passed through: %+v", c.Env)
	}
	if len(c.EnvFrom) != 1 || c.EnvFrom[0].SecretRef == nil {
		t.Errorf("envFrom not passed through: %+v", c.EnvFrom)
	}
}

func envValue(env []corev1.EnvVar, name string) (string, bool) {
	for _, e := range env {
		if e.Name == name {
			return e.Value, true
		}
	}
	return "", false
}

func TestReplicatedStatefulSets(t *testing.T) {
	inst := newInstance()
	inst.Spec.Mode = "replicated"
	inst.Spec.Config = v1alpha1.TalonDBConfig{TCP: ":9899", HTTP: ":8080"}

	leader := BuildLeaderStatefulSet(inst, "h", 100000)
	if leader.Name != "db1-leader" {
		t.Errorf("leader name = %q", leader.Name)
	}
	if leader.Spec.Selector.MatchLabels[RoleLabelKey] != RoleLeader {
		t.Errorf("leader selector missing role=leader: %v", leader.Spec.Selector.MatchLabels)
	}
	if *leader.Spec.Replicas != 1 {
		t.Errorf("leader replicas = %d, want 1", *leader.Spec.Replicas)
	}
	lc := leader.Spec.Template.Spec.Containers[0]
	if v, _ := envValue(lc.Env, "TALONDB_ROLE"); v != RoleLeader {
		t.Errorf("leader TALONDB_ROLE = %q", v)
	}
	if v, _ := envValue(lc.Env, "TALONDB_OPLOG_RETENTION"); v != "100000" {
		t.Errorf("leader TALONDB_OPLOG_RETENTION = %q", v)
	}

	fol := BuildFollowerStatefulSet(inst, "h", "db1.ns1.svc.cluster.local:9899", 3, 0)
	if fol.Name != "db1-follower" {
		t.Errorf("follower name = %q", fol.Name)
	}
	if *fol.Spec.Replicas != 3 {
		t.Errorf("follower replicas = %d, want 3", *fol.Spec.Replicas)
	}
	if fol.Spec.Selector.MatchLabels[RoleLabelKey] != RoleFollower {
		t.Errorf("follower selector missing role=follower")
	}
	fc := fol.Spec.Template.Spec.Containers[0]
	if v, _ := envValue(fc.Env, "TALONDB_ROLE"); v != RoleFollower {
		t.Errorf("follower TALONDB_ROLE = %q", v)
	}
	if v, _ := envValue(fc.Env, "TALONDB_REPLICATE_FROM"); v != "db1.ns1.svc.cluster.local:9899" {
		t.Errorf("follower TALONDB_REPLICATE_FROM = %q", v)
	}
	if _, ok := envValue(fc.Env, "TALONDB_OPLOG_RETENTION"); ok {
		t.Errorf("follower should omit retention env when 0")
	}
}

func TestReplicatedServices(t *testing.T) {
	inst := newInstance()
	inst.Spec.Mode = "replicated"
	inst.Spec.Replication.ReadReplicas = 2

	write := BuildWriteService(inst)
	if write.Name != "db1" || write.Spec.Selector[RoleLabelKey] != RoleLeader {
		t.Errorf("write service name/selector wrong: %s %v", write.Name, write.Spec.Selector)
	}
	read := BuildReadService(inst)
	if read.Name != "db1-read" || read.Spec.Selector[RoleLabelKey] != RoleFollower {
		t.Errorf("read service name/selector wrong: %s %v", read.Name, read.Spec.Selector)
	}

	// With 0 read replicas the read service falls back to the leader.
	inst.Spec.Replication.ReadReplicas = 0
	if BuildReadService(inst).Spec.Selector[RoleLabelKey] != RoleLeader {
		t.Errorf("read service should fall back to leader at 0 replicas")
	}
}

func TestBuildService_Ports(t *testing.T) {
	inst := newInstance()
	inst.Spec.Config = v1alpha1.TalonDBConfig{TCP: ":9899", HTTP: ":8080"}
	svc := BuildService(inst)
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("default service type = %q", svc.Spec.Type)
	}
	names := map[string]bool{}
	for _, p := range svc.Spec.Ports {
		names[p.Name] = true
	}
	for _, n := range []string{"grpc", "http", "metrics"} {
		if !names[n] {
			t.Errorf("service missing port %q", n)
		}
	}

	headless := BuildHeadlessService(inst)
	if headless.Spec.ClusterIP != corev1.ClusterIPNone {
		t.Error("headless service must have ClusterIP None")
	}
}
