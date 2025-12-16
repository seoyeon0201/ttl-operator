/*
Copyright 2025 seoyeon.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ttlv1alpha1 "github.com/seoyeon0201/ttl-operator/api/v1alpha1"
)

const (
	// TTLAnnotationKey는 리소스에 TTL을 지정하는 annotation 키입니다
	TTLAnnotationKey = "ttl.example.com/ttl-seconds"
	// TTLResourceLabelKey는 자동 생성된 TTLResource를 식별하는 label 키입니다
	TTLResourceLabelKey = "ttl.example.com/managed-by"
	// TTLResourceLabelValue는 resource 컨트롤러가 생성한 TTLResource임을 나타냅니다
	TTLResourceLabelValue = "resource-controller"
)

// ResourceReconciler는 Pod, Service, Deployment 등의 리소스를 감시하여 TTL을 적용합니다.
type ResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=pods;services,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=ttl.example.com,resources=ttlresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ttl.example.com,resources=ttlresources/status,verbs=get;update;patch

// Reconcile는 리소스의 annotation을 확인하고 TTLResource를 생성/관리합니다.
// TTLResource도 watch하여 만료 시 리소스를 삭제합니다.
func (r *ResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// TTLResource인지 확인 (TTLResource도 watch하므로)
	ttlResource := &ttlv1alpha1.TTLResource{}
	if err := r.Get(ctx, req.NamespacedName, ttlResource); err == nil {
		// TTLResource인 경우 만료 관리
		return r.reconcileTTLResource(ctx, ttlResource, logger)
	} else if !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// Pod, Service, Deployment를 순서대로 시도
	var obj client.Object
	var gvk string
	var apiVersion string

	// Pod 시도
	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err == nil {
		obj = pod
		gvk = "Pod"
		apiVersion = "v1"
	} else if !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	} else {
		// Service 시도
		svc := &corev1.Service{}
		if err := r.Get(ctx, req.NamespacedName, svc); err == nil {
			obj = svc
			gvk = "Service"
			apiVersion = "v1"
		} else if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		} else {
			// Deployment 시도
			deploy := &appsv1.Deployment{}
			if err := r.Get(ctx, req.NamespacedName, deploy); err == nil {
				obj = deploy
				gvk = "Deployment"
				apiVersion = "apps/v1"
			} else if !errors.IsNotFound(err) {
				return ctrl.Result{}, err
			} else {
				// 리소스를 찾지 못했으면 관련 TTLResource 정리
				return r.cleanupTTLResource(ctx, req.NamespacedName)
			}
		}
	}

	// 리소스가 삭제 중이면 TTLResource 정리
	if obj.GetDeletionTimestamp() != nil {
		return r.cleanupTTLResource(ctx, req.NamespacedName)
	}

	// TTL annotation 확인
	annotations := obj.GetAnnotations()
	ttlSecondsStr, hasTTL := annotations[TTLAnnotationKey]
	if !hasTTL {
		// TTL annotation이 없으면 기존 TTLResource 삭제 (있는 경우)
		return r.cleanupTTLResource(ctx, req.NamespacedName)
	}

	// TTL 값 파싱
	ttlSeconds, err := strconv.Atoi(ttlSecondsStr)
	if err != nil || ttlSeconds <= 0 {
		logger.Info("Invalid TTL annotation value, ignoring", "value", ttlSecondsStr, "resource", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	logger.Info("[Step1] Found resource", "resource", req.NamespacedName, "kind", gvk, "apiVersion", apiVersion)

	// TTLResource 이름 생성
	ttlResourceName := "ttl-" + obj.GetName()

	// 기존 TTLResource 확인
	var existingTTLResource ttlv1alpha1.TTLResource
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: req.Namespace,
		Name:      ttlResourceName,
	}, &existingTTLResource); err == nil {
		// 이미 존재하면 업데이트 (TTL 값이 변경되었을 수 있음)
		if existingTTLResource.Spec.TTLSeconds != ttlSeconds {
			existingTTLResource.Spec.TTLSeconds = ttlSeconds
			// TTL이 변경되면 상태 초기화
			existingTTLResource.Status = ttlv1alpha1.TTLResourceStatus{}
			if err := r.Update(ctx, &existingTTLResource); err != nil {
				if errors.IsConflict(err) {
					// 충돌 발생 시 재시도하지 않고 TTLResource reconcile에 맡김
					logger.V(1).Info("[Reconcile1] Conflict updating TTLResource spec, will be handled by TTLResource reconcile", "name", ttlResourceName)
					return ctrl.Result{}, nil
				}
				logger.Error(err, "Failed to update TTLResource", "name", ttlResourceName)
				return ctrl.Result{}, err
			}
			logger.Info("Updated TTLResource", "name", ttlResourceName, "ttlSeconds", ttlSeconds)
		}
		// TTLResource가 이미 존재하고 TTL 값이 같으면 reconcile하지 않음
		// TTLResource 자체의 reconcile이 만료 관리를 담당
		return ctrl.Result{}, nil
	} else if !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// TTLResource 생성
	ttlResource = &ttlv1alpha1.TTLResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ttlResourceName,
			Namespace: req.Namespace,
			Labels: map[string]string{
				TTLResourceLabelKey:            TTLResourceLabelValue,
				"app.kubernetes.io/managed-by": "ttl-operator",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: apiVersion,
					Kind:       gvk,
					Name:       obj.GetName(),
					UID:        obj.GetUID(),
				},
			},
		},
		Spec: ttlv1alpha1.TTLResourceSpec{
			TTLSeconds: ttlSeconds,
		},
	}

	logger.Info("[Step2] Creating TTLResource", "resource", req.NamespacedName, "kind", gvk, "apiVersion", apiVersion)

	if err := r.Create(ctx, ttlResource); err != nil {
		if errors.IsAlreadyExists(err) {
			// 이미 존재하면 무시
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to create TTLResource", "name", ttlResourceName)
		return ctrl.Result{}, err
	}

	// logger.Info("Created TTLResource for resource",
	// 	"resource", req.NamespacedName,
	// 	"kind", gvk,
	// 	"ttlResource", ttlResourceName,
	// 	"ttlSeconds", ttlSeconds)

	// TTLResource 생성 후 TTLResource reconcile이 자동으로 트리거되므로 재시도하지 않음
	return ctrl.Result{}, nil
}

// cleanupTTLResource는 리소스와 관련된 TTLResource를 삭제합니다.
func (r *ResourceReconciler) cleanupTTLResource(ctx context.Context, namespacedName client.ObjectKey) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	ttlResourceName := "ttl-" + namespacedName.Name
	var ttlResource ttlv1alpha1.TTLResource
	if err := r.Get(ctx, client.ObjectKey{
		Namespace: namespacedName.Namespace,
		Name:      ttlResourceName,
	}, &ttlResource); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Resource 컨트롤러가 생성한 TTLResource인지 확인
	if ttlResource.Labels[TTLResourceLabelKey] == TTLResourceLabelValue {
		if err := r.Delete(ctx, &ttlResource); err != nil {
			if !errors.IsNotFound(err) {
				logger.Error(err, "Failed to delete TTLResource", "name", ttlResourceName)
				return ctrl.Result{}, err
			}
		}
		logger.Info("Deleted TTLResource", "name", ttlResourceName)
	}

	return ctrl.Result{}, nil
}

// reconcileTTLResource는 TTLResource의 만료를 관리하고 만료 시 대상 리소스를 삭제합니다.
func (r *ResourceReconciler) reconcileTTLResource(ctx context.Context, ttlResource *ttlv1alpha1.TTLResource, logger logr.Logger) (ctrl.Result, error) {
	now := metav1.Now()
	// TTLSeconds가 0이면 삭제하지 않고 종료
	if ttlResource.Spec.TTLSeconds == 0 {
		return ctrl.Result{}, nil
	}

	// Status 업데이트 후 최신 버전을 사용하기 위한 변수
	var currentTTLResource *ttlv1alpha1.TTLResource

	// Status 업데이트가 필요한지 확인하고 한 번에 처리
	needsUpdate := false

	// 최초 Reconcile 시 CreatedAt 기록
	if ttlResource.Status.CreatedAt.IsZero() {
		ttlResource.Status.CreatedAt = ttlResource.ObjectMeta.CreationTimestamp
		needsUpdate = true
	}

	// ExpiredAt 계산
	if ttlResource.Status.ExpiredAt == nil && !ttlResource.Status.CreatedAt.IsZero() {
		expireTime := ttlResource.Status.CreatedAt.Add(time.Duration(ttlResource.Spec.TTLSeconds) * time.Second)
		ttlResource.Status.ExpiredAt = &metav1.Time{Time: expireTime}
		needsUpdate = true
	}

	// Status 업데이트가 필요하면 한 번에 업데이트
	if needsUpdate {
		// 충돌 방지를 위해 최신 버전 다시 가져오기
		latestTTLResource := &ttlv1alpha1.TTLResource{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: ttlResource.Namespace,
			Name:      ttlResource.Name,
		}, latestTTLResource); err != nil {
			if errors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, err
		}

		// UID가 일치하는지 확인 (리소스가 삭제 후 재생성되었는지 확인)
		if ttlResource.UID != latestTTLResource.UID {
			logger.V(1).Info("TTLResource UID mismatch, resource may have been recreated",
				"name", latestTTLResource.Name,
				"oldUID", ttlResource.UID,
				"newUID", latestTTLResource.UID)
			// 리소스가 재생성되었으므로 새로운 reconcile을 기다림
			return ctrl.Result{}, nil
		}

		// 최신 버전에서 Status 업데이트
		if latestTTLResource.Status.CreatedAt.IsZero() {
			latestTTLResource.Status.CreatedAt = latestTTLResource.ObjectMeta.CreationTimestamp
		}
		if latestTTLResource.Status.ExpiredAt == nil && !latestTTLResource.Status.CreatedAt.IsZero() {
			expireTime := latestTTLResource.Status.CreatedAt.Add(time.Duration(latestTTLResource.Spec.TTLSeconds) * time.Second)
			latestTTLResource.Status.ExpiredAt = &metav1.Time{Time: expireTime}
		}

		if err := r.Status().Update(ctx, latestTTLResource); err != nil {
			if errors.IsConflict(err) {
				// 충돌 발생 시 짧은 지연 후 재시도 (무한 루프 방지)
				logger.V(1).Info("Conflict updating TTLResource status, will retry", "name", latestTTLResource.Name)
				return ctrl.Result{RequeueAfter: time.Second}, nil
			}
			// 리소스가 삭제되었을 수 있음
			if errors.IsNotFound(err) {
				logger.V(1).Info("TTLResource not found, may have been deleted", "name", latestTTLResource.Name)
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, err
		}
		logger.Info("[Step4] Completely Updated TTLResource status!", "name", latestTTLResource.Name)
		// Status 업데이트 후 최신 버전으로 만료 확인을 계속 진행
		currentTTLResource = latestTTLResource
		// Status 업데이트 후 now를 다시 계산하여 만료 확인
		now = metav1.Now()
	} else {
		// Status 업데이트가 필요 없으면 현재 버전 사용
		currentTTLResource = ttlResource
	}

	// 이미 만료 처리된 경우 삭제 진행
	if currentTTLResource.Status.Expired {
		logger.Info("[Step5] TTLResource already expired, deleting resources",
			"name", currentTTLResource.Name,
			"expiredAt", currentTTLResource.Status.ExpiredAt)
		// 최신 버전 다시 가져오기 (UID 확인을 위해)
		latestTTLResource := &ttlv1alpha1.TTLResource{}
		if err := r.Get(ctx, client.ObjectKey{
			Namespace: currentTTLResource.Namespace,
			Name:      currentTTLResource.Name,
		}, latestTTLResource); err != nil {
			if errors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
			return ctrl.Result{}, err
		}

		// UID가 일치하는지 확인
		if currentTTLResource.UID != latestTTLResource.UID {
			logger.V(1).Info("TTLResource UID mismatch, resource may have been recreated", "name", latestTTLResource.Name)
			return ctrl.Result{}, nil
		}

		// 리소스 삭제 진행
		return r.deleteExpiredResources(ctx, latestTTLResource, logger)
	}

	// TTL 만료 확인 및 삭제
	if currentTTLResource.Status.ExpiredAt != nil {
		// 만료 시간이 지났는지 확인
		if !now.Time.Before(currentTTLResource.Status.ExpiredAt.Time) {
			// 만료 시간이 지났음 - 삭제 진행
			logger.Info("[Step5] TTL expired, starting deletion process",
				"name", currentTTLResource.Name,
				"expiredAt", currentTTLResource.Status.ExpiredAt.Time,
				"now", now.Time)
			// 최신 버전 다시 가져오기 (UID 확인을 위해)
			latestTTLResource := &ttlv1alpha1.TTLResource{}
			if err := r.Get(ctx, client.ObjectKey{
				Namespace: currentTTLResource.Namespace,
				Name:      currentTTLResource.Name,
			}, latestTTLResource); err != nil {
				if errors.IsNotFound(err) {
					return ctrl.Result{}, nil
				}
				return ctrl.Result{}, err
			}

			// UID가 일치하는지 확인 (리소스가 삭제 후 재생성되었는지 확인)
			if currentTTLResource.UID != latestTTLResource.UID {
				logger.V(1).Info("TTLResource UID mismatch, resource may have been recreated",
					"name", latestTTLResource.Name,
					"oldUID", ttlResource.UID,
					"newUID", latestTTLResource.UID)
				// 리소스가 재생성되었으므로 새로운 reconcile을 기다림
				return ctrl.Result{}, nil
			}

			// Expired 상태로 업데이트 시도
			if !latestTTLResource.Status.Expired {
				latestTTLResource.Status.Expired = true
				if err := r.Status().Update(ctx, latestTTLResource); err != nil {
					if errors.IsConflict(err) {
						// 충돌 발생 시 짧은 지연 후 재시도 (무한 루프 방지)
						logger.V(1).Info("Conflict updating TTLResource status, will retry", "name", latestTTLResource.Name)
						return ctrl.Result{RequeueAfter: time.Second}, nil
					}
					// 리소스가 삭제되었을 수 있음
					if errors.IsNotFound(err) {
						logger.V(1).Info("TTLResource not found, may have been deleted", "name", latestTTLResource.Name)
						return ctrl.Result{}, nil
					}
					logger.Error(err, "Failed to update TTLResource status", "name", latestTTLResource.Name)
					return ctrl.Result{}, err
				}
			}

			// 리소스 삭제 진행
			return r.deleteExpiredResources(ctx, latestTTLResource, logger)
		} else {
			// 만료 시간 전 - 남은 시간만큼 재큐잉
			requeueAfter := currentTTLResource.Status.ExpiredAt.Time.Sub(now.Time)
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
	}

	return ctrl.Result{}, nil
}

// deleteExpiredResources는 만료된 리소스를 삭제합니다.
func (r *ResourceReconciler) deleteExpiredResources(ctx context.Context, ttlResource *ttlv1alpha1.TTLResource, logger logr.Logger) (ctrl.Result, error) {
	// OwnerReference를 통해 대상 리소스 삭제
	logger.Info("[Step6] deleteExpiredResources() Deleting expired resources", "name", ttlResource.Name)
	if len(ttlResource.OwnerReferences) > 0 {
		ownerRef := ttlResource.OwnerReferences[0]
		if err := r.deleteOwnerResource(ctx, ownerRef, ttlResource.Namespace); err != nil {
			logger.Error(err, "Failed to delete owner resource", "ownerRef", ownerRef)
			// Owner 리소스 삭제 실패해도 TTLResource는 삭제
		} else {
			logger.Info("Deleted owner resource", "kind", ownerRef.Kind, "name", ownerRef.Name)
		}
	}

	// TTL 만료 시 TTLResource 삭제
	logger.Info("[Step7] deleteExpiredResources() Deleting TTLResource", "name", ttlResource.Name)
	if err := r.Delete(ctx, ttlResource); err != nil {
		if errors.IsNotFound(err) {
			// 이미 삭제된 경우 무시
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("TTLResource expired and deleted", "name", ttlResource.Name)
	return ctrl.Result{}, nil
}

// deleteOwnerResource는 OwnerReference를 통해 대상 리소스를 삭제합니다.
func (r *ResourceReconciler) deleteOwnerResource(ctx context.Context, ownerRef metav1.OwnerReference, namespace string) error {
	gv, err := schema.ParseGroupVersion(ownerRef.APIVersion)
	if err != nil {
		return fmt.Errorf("invalid apiVersion: %w", err)
	}

	gvk := schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    ownerRef.Kind,
	}

	var obj client.Object
	switch gvk {
	case schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}:
		obj = &corev1.Pod{}
	case schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}:
		obj = &corev1.Service{}
	case schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}:
		obj = &appsv1.Deployment{}
	default:
		return fmt.Errorf("unsupported resource type: %s", gvk.String())
	}

	obj.SetName(ownerRef.Name)
	obj.SetNamespace(namespace)

	if err := r.Delete(ctx, obj); err != nil {
		if errors.IsNotFound(err) {
			// 이미 삭제된 경우는 정상으로 처리
			return nil
		}
		return fmt.Errorf("failed to delete owner resource %s/%s/%s: %w", gvk.Kind, namespace, ownerRef.Name, err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
// Pod, Service, Deployment, TTLResource를 모두 watch합니다.
func (r *ResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Pod를 primary resource로 설정
	builder := ctrl.NewControllerManagedBy(mgr).
		Named("resource-ttl").
		For(&corev1.Pod{})

	// Service, Deployment, TTLResource도 watch
	builder = builder.
		Watches(&corev1.Service{}, &handler.EnqueueRequestForObject{}).
		Watches(&appsv1.Deployment{}, &handler.EnqueueRequestForObject{}).
		Watches(&ttlv1alpha1.TTLResource{}, &handler.EnqueueRequestForObject{})

	return builder.Complete(r)
}
