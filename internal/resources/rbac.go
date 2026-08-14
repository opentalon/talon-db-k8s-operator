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
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/opentalon/tln-db-k8s-operator/api/v1alpha1"
)

// BuildServiceAccount creates the ServiceAccount used by the tln-db pod.
func BuildServiceAccount(instance *v1alpha1.TlnDB) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ResourceName(instance),
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
		},
		AutomountServiceAccountToken: boolPtr(false),
	}
}

// BuildRole grants the pod permission to read its own ConfigMap and emit events.
func BuildRole(instance *v1alpha1.TlnDB) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ResourceName(instance),
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"configmaps"},
				ResourceNames: []string{ConfigMapName(instance)},
				Verbs:         []string{"get", "watch", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"events"},
				Verbs:     []string{"create", "patch"},
			},
		},
	}
}

// BuildRoleBinding binds the instance Role to its ServiceAccount.
func BuildRoleBinding(instance *v1alpha1.TlnDB) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ResourceName(instance),
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     ResourceName(instance),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      ResourceName(instance),
				Namespace: instance.Namespace,
			},
		},
	}
}
