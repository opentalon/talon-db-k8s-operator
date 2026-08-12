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
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1alpha1 "github.com/opentalon/talon-db-k8s-operator/api/v1alpha1"
)

// BuildPDB creates a PodDisruptionBudget for the TalonDB instance.
func BuildPDB(instance *v1alpha1.TalonDB) *policyv1.PodDisruptionBudget {
	pdbSpec := instance.Spec.Availability.PodDisruptionBudget

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ResourceName(instance),
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: SelectorLabels(instance)},
		},
	}

	switch {
	case pdbSpec.MinAvailable != nil:
		v := intstr.FromInt32(*pdbSpec.MinAvailable)
		pdb.Spec.MinAvailable = &v
	case pdbSpec.MaxUnavailable != nil:
		v := intstr.FromInt32(*pdbSpec.MaxUnavailable)
		pdb.Spec.MaxUnavailable = &v
	default:
		v := intstr.FromInt32(1)
		pdb.Spec.MaxUnavailable = &v
	}

	return pdb
}
