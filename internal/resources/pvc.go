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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/opentalon/tln-db-k8s-operator/api/v1alpha1"
)

// PVCName returns the name of the standalone data PVC.
func PVCName(instance *v1alpha1.TlnDB) string {
	return ResourceName(instance) + "-data"
}

// BuildPVC builds a standalone PersistentVolumeClaim for /data. This is used
// when the operator manages the claim directly rather than via the
// StatefulSet's VolumeClaimTemplates.
func BuildPVC(instance *v1alpha1.TlnDB) *corev1.PersistentVolumeClaim {
	persistence := instance.Spec.Storage.Persistence

	size := persistence.Size
	if size.IsZero() {
		size = resource.MustParse("1Gi")
	}
	accessModes := persistence.AccessModes
	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      PVCName(instance),
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
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
