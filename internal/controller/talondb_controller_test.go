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

package controller

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	talondbv1alpha1 "github.com/opentalon/talon-db-k8s-operator/api/v1alpha1"
	"github.com/opentalon/talon-db-k8s-operator/internal/resources"
)

func TestReconcilerImplementsInterface(t *testing.T) {
	var _ reconcile.Reconciler = &TalonDBReconciler{}
}

func TestSetConditionOnInstance(t *testing.T) {
	r := &TalonDBReconciler{}
	inst := &talondbv1alpha1.TalonDB{}
	r.setConditionOnInstance(inst, talondbv1alpha1.ConditionStatefulSetReady, func() (metav1.ConditionStatus, string, string) {
		return metav1.ConditionTrue, "Ready", "ok"
	})
	cond := apimeta.FindStatusCondition(inst.Status.Conditions, talondbv1alpha1.ConditionStatefulSetReady)
	if cond == nil {
		t.Fatal("condition not set")
	}
	if cond.Status != metav1.ConditionTrue || cond.Reason != "Ready" {
		t.Errorf("unexpected condition: %+v", cond)
	}
}

func stsWithHashImage(hash, image string, replicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{resources.ConfigHashAnnotation: hash}},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Image: image}}},
			},
		},
	}
}

func TestStatefulSetNeedsUpdate(t *testing.T) {
	base := stsWithHashImage("h1", "img:1", 1)

	if statefulSetNeedsUpdate(base, stsWithHashImage("h1", "img:1", 1)) {
		t.Error("identical StatefulSets should not need update")
	}
	if !statefulSetNeedsUpdate(base, stsWithHashImage("h2", "img:1", 1)) {
		t.Error("config hash change should trigger update")
	}
	if !statefulSetNeedsUpdate(base, stsWithHashImage("h1", "img:2", 1)) {
		t.Error("image change should trigger update")
	}
	if !statefulSetNeedsUpdate(base, stsWithHashImage("h1", "img:1", 2)) {
		t.Error("replica change should trigger update")
	}
}
