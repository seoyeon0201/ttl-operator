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
	"time" //TTL 계산에 필요

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1" //metav1.Time 타입 사용
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ttlv1alpha1 "github.com/seoyeon0201/ttl-operator/api/v1alpha1"
)

// TTLResourceReconciler reconciles a TTLResource object
type TTLResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=ttl.example.com,resources=ttlresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ttl.example.com,resources=ttlresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ttl.example.com,resources=ttlresources/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the TTLResource object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile

// 기존 코드 !
// func (r *TTLResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
// 	_ = logf.FromContext(ctx)

// 	// TODO(user): your logic here

//		return ctrl.Result{}, nil
//	}
func (r *TTLResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	var ttlResource ttlv1alpha1.TTLResource
	if err := r.Get(ctx, req.NamespacedName, &ttlResource); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	now := metav1.Now()

	// 1. TTLSeconds가 0이면 삭제하지 않고 종료
	if ttlResource.Spec.TTLSeconds == 0 {
		return ctrl.Result{}, nil
	}

	// 2. 최초 Reconcile 시 CreatedAt 기록
	if ttlResource.Status.CreatedAt.IsZero() {
		ttlResource.Status.CreatedAt = ttlResource.ObjectMeta.CreationTimestamp
		if err := r.Status().Update(ctx, &ttlResource); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// 3. ExpiredAt 계산
	if ttlResource.Status.ExpiredAt == nil {
		expireTime := ttlResource.Status.CreatedAt.Add(time.Duration(ttlResource.Spec.TTLSeconds) * time.Second)
		ttlResource.Status.ExpiredAt = &metav1.Time{Time: expireTime}
		if err := r.Status().Update(ctx, &ttlResource); err != nil {
			return ctrl.Result{}, err
		}
	}

	// 4. TTL 만료 확인 및 삭제
	if !ttlResource.Status.Expired && now.Time.After(ttlResource.Status.ExpiredAt.Time) {
		ttlResource.Status.Expired = true
		if err := r.Status().Update(ctx, &ttlResource); err != nil {
			return ctrl.Result{}, err
		}

		// TTL 만료 시 삭제
		if err := r.Delete(ctx, &ttlResource); err != nil {
			return ctrl.Result{}, err
		}

		logger.Info("TTLResource expired and deleted", "name", ttlResource.Name)
		return ctrl.Result{}, nil
	}

	// 5. TTL 만료 전, 남은 시간만큼 재큐잉
	requeueAfter := ttlResource.Status.ExpiredAt.Time.Sub(now.Time)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TTLResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ttlv1alpha1.TTLResource{}).
		Named("ttlresource").
		Complete(r)
}
