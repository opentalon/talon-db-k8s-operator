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
	"context"
	"fmt"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	talondbv1alpha1 "github.com/opentalon/talon-db-k8s-operator/api/v1alpha1"
	"github.com/opentalon/talon-db-k8s-operator/internal/resources"
)

// +kubebuilder:rbac:groups=db.opentalon.io,resources=talondbs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=db.opentalon.io,resources=talondbs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=db.opentalon.io,resources=talondbs/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services;configmaps;secrets;persistentvolumeclaims;serviceaccounts;events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses;networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete

// TalonDBReconciler reconciles a TalonDB resource.
type TalonDBReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// SetupWithManager registers the controller and its owned-resource watches.
func (r *TalonDBReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&talondbv1alpha1.TalonDB{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Complete(r)
}

// Reconcile is the main reconciliation loop.
func (r *TalonDBReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	instance := &talondbv1alpha1.TalonDB{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch TalonDB")
		return ctrl.Result{}, err
	}

	// Deletion / finalizer handling.
	if !instance.DeletionTimestamp.IsZero() {
		return r.reconcileDeletion(ctx, instance)
	}
	if !controllerutil.ContainsFinalizer(instance, resources.FinalizerName) {
		controllerutil.AddFinalizer(instance, resources.FinalizerName)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if instance.Status.Phase == "" {
		if err := r.setPhase(ctx, instance, talondbv1alpha1.PhaseProvisioning); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.reconcileResources(ctx, instance); err != nil {
		r.Recorder.Eventf(instance, corev1.EventTypeWarning, "ReconcileError", "Failed to reconcile resources: %v", err)
		_ = r.setPhase(ctx, instance, talondbv1alpha1.PhaseFailed)
		return ctrl.Result{}, err
	}

	return r.syncStatus(ctx, instance)
}

func (r *TalonDBReconciler) reconcileDeletion(ctx context.Context, instance *talondbv1alpha1.TalonDB) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(instance, resources.FinalizerName) {
		if err := r.setPhase(ctx, instance, talondbv1alpha1.PhaseTerminating); err != nil {
			return ctrl.Result{}, err
		}
		// Owned resources are garbage-collected via OwnerReferences.
		controllerutil.RemoveFinalizer(instance, resources.FinalizerName)
		if err := r.Update(ctx, instance); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *TalonDBReconciler) reconcileResources(ctx context.Context, instance *talondbv1alpha1.TalonDB) error {
	managed := []string{}

	// ServiceAccount (only when the user hasn't supplied one).
	if instance.Spec.ServiceAccountName == "" {
		sa := resources.BuildServiceAccount(instance)
		if err := r.createOrUpdateServiceAccount(ctx, instance, sa); err != nil {
			return fmt.Errorf("ServiceAccount: %w", err)
		}
		managed = append(managed, "ServiceAccount/"+sa.Name)

		role := resources.BuildRole(instance)
		if err := r.createOrUpdateRole(ctx, instance, role); err != nil {
			return fmt.Errorf("role: %w", err)
		}
		managed = append(managed, "Role/"+role.Name)

		rb := resources.BuildRoleBinding(instance)
		if err := r.createOrUpdateRoleBinding(ctx, instance, rb); err != nil {
			return fmt.Errorf("RoleBinding: %w", err)
		}
		managed = append(managed, "RoleBinding/"+rb.Name)
		r.setCondition(ctx, instance, talondbv1alpha1.ConditionRBACReady, metav1.ConditionTrue, "RBACReady", "RBAC reconciled")
	}

	// ConfigMap (rendered or external via ConfigFrom).
	var configHash string
	if instance.Spec.ConfigFrom != nil {
		externalCM := &corev1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Spec.ConfigFrom.Name}, externalCM); err != nil {
			if apierrors.IsNotFound(err) {
				r.setCondition(ctx, instance, talondbv1alpha1.ConditionConfigMapReady, metav1.ConditionFalse,
					"ExternalConfigMapNotFound", fmt.Sprintf("referenced ConfigMap %q not found", instance.Spec.ConfigFrom.Name))
				return fmt.Errorf("external ConfigMap %q not found: %w", instance.Spec.ConfigFrom.Name, err)
			}
			return err
		}
		configHash = resources.HashStringData(externalCM.Data)
		r.setCondition(ctx, instance, talondbv1alpha1.ConditionConfigMapReady, metav1.ConditionTrue, "ExternalConfigMapFound", "External ConfigMap found")
	} else {
		cm := resources.BuildConfigMap(instance)
		if err := r.createOrUpdateConfigMap(ctx, instance, cm); err != nil {
			return fmt.Errorf("ConfigMap: %w", err)
		}
		configHash = resources.HashStringData(cm.Data)
		managed = append(managed, "ConfigMap/"+cm.Name)
		r.setCondition(ctx, instance, talondbv1alpha1.ConditionConfigMapReady, metav1.ConditionTrue, "ConfigMapReady", "ConfigMap reconciled")
	}

	// Headless Service (stable pod DNS).
	headless := resources.BuildHeadlessService(instance)
	if err := r.createOrUpdateService(ctx, instance, headless); err != nil {
		return fmt.Errorf("headless Service: %w", err)
	}
	managed = append(managed, "Service/"+headless.Name)

	// StatefulSet(s) + serving Service(s).
	if resources.Replicated(instance) {
		retention := instance.Spec.Replication.OplogRetention
		grpcPort := resources.PortFromAddr(instance.Spec.Config.TCP, resources.DefaultGRPCPort)
		leaderAddr := fmt.Sprintf("%s.%s.svc.cluster.local:%d", resources.ResourceName(instance), instance.Namespace, grpcPort)

		leader := resources.BuildLeaderStatefulSet(instance, configHash, retention)
		if err := r.createOrUpdateStatefulSet(ctx, instance, leader); err != nil {
			return fmt.Errorf("leader StatefulSet: %w", err)
		}
		managed = append(managed, "StatefulSet/"+leader.Name)

		if instance.Spec.Replication.ReadReplicas > 0 {
			followers := resources.BuildFollowerStatefulSet(instance, configHash, leaderAddr, instance.Spec.Replication.ReadReplicas, retention)
			if err := r.createOrUpdateStatefulSet(ctx, instance, followers); err != nil {
				return fmt.Errorf("follower StatefulSet: %w", err)
			}
			managed = append(managed, "StatefulSet/"+followers.Name)
		} else {
			// Scaled to zero followers: remove any previously-created set.
			if err := r.deleteStatefulSetIfExists(ctx, instance.Namespace, resources.FollowerStatefulSetName(instance)); err != nil {
				return fmt.Errorf("prune follower StatefulSet: %w", err)
			}
		}

		writeSvc := resources.BuildWriteService(instance)
		if err := r.createOrUpdateService(ctx, instance, writeSvc); err != nil {
			return fmt.Errorf("write Service: %w", err)
		}
		managed = append(managed, "Service/"+writeSvc.Name)

		readSvc := resources.BuildReadService(instance)
		if err := r.createOrUpdateService(ctx, instance, readSvc); err != nil {
			return fmt.Errorf("read Service: %w", err)
		}
		managed = append(managed, "Service/"+readSvc.Name)
		r.setCondition(ctx, instance, talondbv1alpha1.ConditionServiceReady, metav1.ConditionTrue, "ServiceReady", "Services reconciled")
	} else {
		sts := resources.BuildStatefulSet(instance, configHash)
		if err := r.createOrUpdateStatefulSet(ctx, instance, sts); err != nil {
			return fmt.Errorf("StatefulSet: %w", err)
		}
		managed = append(managed, "StatefulSet/"+sts.Name)

		svc := resources.BuildService(instance)
		if err := r.createOrUpdateService(ctx, instance, svc); err != nil {
			return fmt.Errorf("service: %w", err)
		}
		managed = append(managed, "Service/"+svc.Name)
		r.setCondition(ctx, instance, talondbv1alpha1.ConditionServiceReady, metav1.ConditionTrue, "ServiceReady", "Service reconciled")
	}

	// Ingress (optional).
	if instance.Spec.Networking.Ingress.Enabled {
		ingress := resources.BuildIngress(instance)
		if err := r.createOrUpdateIngress(ctx, instance, ingress); err != nil {
			return fmt.Errorf("ingress: %w", err)
		}
		managed = append(managed, "Ingress/"+ingress.Name)
	}

	// NetworkPolicy (optional).
	if instance.Spec.Networking.NetworkPolicy.Enabled {
		np := resources.BuildNetworkPolicy(instance)
		if err := r.createOrUpdateNetworkPolicy(ctx, instance, np); err != nil {
			return fmt.Errorf("NetworkPolicy: %w", err)
		}
		managed = append(managed, "NetworkPolicy/"+np.Name)
		r.setCondition(ctx, instance, talondbv1alpha1.ConditionNetworkPolicyReady, metav1.ConditionTrue, "NetworkPolicyReady", "NetworkPolicy reconciled")
	}

	// ServiceMonitor (optional; degrades gracefully without the CRD).
	if instance.Spec.Observability.Metrics.ServiceMonitor.Enabled {
		sm := resources.BuildServiceMonitor(instance)
		if err := r.createOrUpdateServiceMonitor(ctx, instance, sm); err != nil {
			log.FromContext(ctx).Info("ServiceMonitor reconcile skipped (CRD may not be installed)", "error", err.Error())
			r.setCondition(ctx, instance, talondbv1alpha1.ConditionServiceMonitorReady, metav1.ConditionFalse, "ServiceMonitorCRDMissing", err.Error())
		} else {
			managed = append(managed, "ServiceMonitor/"+sm.GetName())
			r.setCondition(ctx, instance, talondbv1alpha1.ConditionServiceMonitorReady, metav1.ConditionTrue, "ServiceMonitorReady", "ServiceMonitor reconciled")
		}
	}

	// PodDisruptionBudget (optional).
	if instance.Spec.Availability.PodDisruptionBudget.Enabled {
		pdb := resources.BuildPDB(instance)
		if err := r.createOrUpdatePDB(ctx, instance, pdb); err != nil {
			return fmt.Errorf("PodDisruptionBudget: %w", err)
		}
		managed = append(managed, "PodDisruptionBudget/"+pdb.Name)
	}

	// HorizontalPodAutoscaler (optional).
	if instance.Spec.Availability.HorizontalPodAutoscaler.Enabled {
		hpa := resources.BuildHPA(instance)
		if err := r.createOrUpdateHPA(ctx, instance, hpa); err != nil {
			return fmt.Errorf("HorizontalPodAutoscaler: %w", err)
		}
		managed = append(managed, "HorizontalPodAutoscaler/"+hpa.Name)
	}

	// Persist the managed-resource list and image.
	sort.Strings(managed)
	patch := client.MergeFrom(instance.DeepCopy())
	instance.Status.ManagedResources = managed
	instance.Status.CurrentImage = resources.ImageRef(instance.Spec.Image)
	now := metav1.Now()
	instance.Status.LastUpdateTime = &now
	instance.Status.ObservedGeneration = instance.Generation
	if err := r.Status().Patch(ctx, instance, patch); err != nil && !apierrors.IsConflict(err) {
		return err
	}
	return nil
}

// primaryStatefulSetName is the StatefulSet whose readiness gates the
// instance phase: the leader in replicated mode, else the sole set.
func primaryStatefulSetName(instance *talondbv1alpha1.TalonDB) string {
	if resources.Replicated(instance) {
		return resources.LeaderStatefulSetName(instance)
	}
	return resources.ResourceName(instance)
}

func (r *TalonDBReconciler) syncStatus(ctx context.Context, instance *talondbv1alpha1.TalonDB) (ctrl.Result, error) {
	sts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: primaryStatefulSetName(instance)}, sts); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	// The leader always runs a single writer; standalone honors spec.replicas.
	desiredReplicas := int32(1)
	if !resources.Replicated(instance) && instance.Spec.Replicas != nil {
		desiredReplicas = *instance.Spec.Replicas
	}
	ready := sts.Status.ReadyReplicas

	patch := client.MergeFrom(instance.DeepCopy())
	instance.Status.ReadyReplicas = ready

	var phase string
	switch {
	case sts.Status.ObservedGeneration < sts.Generation:
		phase = talondbv1alpha1.PhaseProvisioning
	case ready < desiredReplicas:
		phase = talondbv1alpha1.PhaseDegraded
	default:
		phase = talondbv1alpha1.PhaseRunning
	}
	instance.Status.Phase = phase

	r.setConditionOnInstance(instance, talondbv1alpha1.ConditionStatefulSetReady, func() (metav1.ConditionStatus, string, string) {
		if phase == talondbv1alpha1.PhaseRunning {
			return metav1.ConditionTrue, "StatefulSetReady", fmt.Sprintf("%d/%d replicas ready", ready, desiredReplicas)
		}
		return metav1.ConditionFalse, "StatefulSetNotReady", fmt.Sprintf("%d/%d replicas ready", ready, desiredReplicas)
	})

	if err := r.Status().Patch(ctx, instance, patch); err != nil && !apierrors.IsConflict(err) {
		return ctrl.Result{}, err
	}

	if phase != talondbv1alpha1.PhaseRunning {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	return ctrl.Result{}, nil
}

// ── Create-or-update helpers ──────────────────────────────────────────────────

func (r *TalonDBReconciler) createOrUpdateConfigMap(ctx context.Context, instance *talondbv1alpha1.TalonDB, desired *corev1.ConfigMap) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if !equality.Semantic.DeepEqual(existing.Data, desired.Data) {
		existing.Data = desired.Data
		existing.Labels = desired.Labels
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *TalonDBReconciler) createOrUpdateServiceAccount(ctx context.Context, instance *talondbv1alpha1.TalonDB, desired *corev1.ServiceAccount) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &corev1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

func (r *TalonDBReconciler) createOrUpdateRole(ctx context.Context, instance *talondbv1alpha1.TalonDB, desired *rbacv1.Role) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &rbacv1.Role{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if !equality.Semantic.DeepEqual(existing.Rules, desired.Rules) {
		existing.Rules = desired.Rules
		existing.Labels = desired.Labels
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *TalonDBReconciler) createOrUpdateRoleBinding(ctx context.Context, instance *talondbv1alpha1.TalonDB, desired *rbacv1.RoleBinding) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &rbacv1.RoleBinding{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if !equality.Semantic.DeepEqual(existing.RoleRef, desired.RoleRef) || !equality.Semantic.DeepEqual(existing.Subjects, desired.Subjects) {
		existing.RoleRef = desired.RoleRef
		existing.Subjects = desired.Subjects
		existing.Labels = desired.Labels
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *TalonDBReconciler) createOrUpdateStatefulSet(ctx context.Context, instance *talondbv1alpha1.TalonDB, desired *appsv1.StatefulSet) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		r.Recorder.Eventf(instance, corev1.EventTypeNormal, "Created", "Created StatefulSet %s", desired.Name)
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}

	// VolumeClaimTemplates are immutable after creation; preserve them.
	desired.ResourceVersion = existing.ResourceVersion
	desired.Spec.VolumeClaimTemplates = existing.Spec.VolumeClaimTemplates

	if !statefulSetNeedsUpdate(existing, desired) {
		return nil
	}
	r.Recorder.Eventf(instance, corev1.EventTypeNormal, "Updated", "Updated StatefulSet %s", desired.Name)
	existing.Spec.Replicas = desired.Spec.Replicas
	existing.Spec.Template = desired.Spec.Template
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

// deleteStatefulSetIfExists removes a StatefulSet by name, ignoring
// not-found. Used to prune the follower set when readReplicas drops to 0.
func (r *TalonDBReconciler) deleteStatefulSetIfExists(ctx context.Context, namespace, name string) error {
	existing := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, existing); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := r.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *TalonDBReconciler) createOrUpdateService(ctx context.Context, instance *talondbv1alpha1.TalonDB, desired *corev1.Service) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	desired.Spec.ClusterIP = existing.Spec.ClusterIP // immutable once assigned
	desired.ResourceVersion = existing.ResourceVersion
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	existing.Annotations = desired.Annotations
	return r.Update(ctx, existing)
}

func (r *TalonDBReconciler) createOrUpdateIngress(ctx context.Context, instance *talondbv1alpha1.TalonDB, desired *networkingv1.Ingress) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &networkingv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	desired.ResourceVersion = existing.ResourceVersion
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	existing.Annotations = desired.Annotations
	return r.Update(ctx, existing)
}

func (r *TalonDBReconciler) createOrUpdateNetworkPolicy(ctx context.Context, instance *talondbv1alpha1.TalonDB, desired *networkingv1.NetworkPolicy) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

func (r *TalonDBReconciler) createOrUpdateServiceMonitor(ctx context.Context, instance *talondbv1alpha1.TalonDB, desired *unstructured.Unstructured) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		log.FromContext(ctx).V(1).Info("could not set owner reference on ServiceMonitor", "error", err)
	}
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(desired.GroupVersionKind())
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.GetNamespace(), Name: desired.GetName()}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	desired.SetResourceVersion(existing.GetResourceVersion())
	return r.Update(ctx, desired)
}

func (r *TalonDBReconciler) createOrUpdatePDB(ctx context.Context, instance *talondbv1alpha1.TalonDB, desired *policyv1.PodDisruptionBudget) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &policyv1.PodDisruptionBudget{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	if err := r.Update(ctx, existing); err != nil && !apierrors.IsConflict(err) {
		return err
	}
	return nil
}

func (r *TalonDBReconciler) createOrUpdateHPA(ctx context.Context, instance *talondbv1alpha1.TalonDB, desired *autoscalingv2.HorizontalPodAutoscaler) error {
	if err := controllerutil.SetControllerReference(instance, desired, r.Scheme); err != nil {
		return err
	}
	existing := &autoscalingv2.HorizontalPodAutoscaler{}
	err := r.Get(ctx, types.NamespacedName{Namespace: desired.Namespace, Name: desired.Name}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	if err := r.Update(ctx, existing); err != nil && !apierrors.IsConflict(err) {
		return err
	}
	return nil
}

// ── Status helpers ────────────────────────────────────────────────────────────

func (r *TalonDBReconciler) setPhase(ctx context.Context, instance *talondbv1alpha1.TalonDB, phase string) error {
	patch := client.MergeFrom(instance.DeepCopy())
	instance.Status.Phase = phase
	now := metav1.Now()
	instance.Status.LastUpdateTime = &now
	if err := r.Status().Patch(ctx, instance, patch); err != nil && !apierrors.IsConflict(err) {
		return err
	}
	return nil
}

func (r *TalonDBReconciler) setCondition(ctx context.Context, instance *talondbv1alpha1.TalonDB, condType string, status metav1.ConditionStatus, reason, message string) {
	patch := client.MergeFrom(instance.DeepCopy())
	apimeta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: instance.Generation,
	})
	if err := r.Status().Patch(ctx, instance, patch); err != nil && !apierrors.IsConflict(err) {
		log.FromContext(ctx).V(1).Info("failed to patch condition", "type", condType, "error", err)
	}
}

func (r *TalonDBReconciler) setConditionOnInstance(instance *talondbv1alpha1.TalonDB, condType string, fn func() (metav1.ConditionStatus, string, string)) {
	status, reason, message := fn()
	apimeta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: instance.Generation,
	})
}

// statefulSetNeedsUpdate reports whether the operator-managed parts of the
// StatefulSet spec (replicas, config hash, image) have changed.
func statefulSetNeedsUpdate(existing, desired *appsv1.StatefulSet) bool {
	if !equality.Semantic.DeepEqual(existing.Spec.Replicas, desired.Spec.Replicas) {
		return true
	}
	if existing.Spec.Template.Annotations[resources.ConfigHashAnnotation] != desired.Spec.Template.Annotations[resources.ConfigHashAnnotation] {
		return true
	}
	if len(existing.Spec.Template.Spec.Containers) > 0 && len(desired.Spec.Template.Spec.Containers) > 0 {
		if existing.Spec.Template.Spec.Containers[0].Image != desired.Spec.Template.Spec.Containers[0].Image {
			return true
		}
	}
	return false
}
