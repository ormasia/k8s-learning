// /*
// Copyright 2025.

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// */

// package controller

// import (
// 	"context"

// 	"k8s.io/apimachinery/pkg/runtime"
// 	ctrl "sigs.k8s.io/controller-runtime"
// 	"sigs.k8s.io/controller-runtime/pkg/client"
// 	"sigs.k8s.io/controller-runtime/pkg/log"
// )

// // PodReconciler reconciles a Pod object
// type PodReconciler struct {
// 	client.Client
// 	Scheme *runtime.Scheme
// }

// //+kubebuilder:rbac:groups=example.com,resources=pods,verbs=get;list;watch;create;update;patch;delete
// //+kubebuilder:rbac:groups=example.com,resources=pods/status,verbs=get;update;patch
// //+kubebuilder:rbac:groups=example.com,resources=pods/finalizers,verbs=update

// // Reconcile is part of the main kubernetes reconciliation loop which aims to
// // move the current state of the cluster closer to the desired state.
// // TODO(user): Modify the Reconcile function to compare the state specified by
// // the Pod object against the actual cluster state, and then
// // perform operations to make the cluster state reflect the state specified by
// // the user.
// //
// // For more details, check Reconcile and its Result here:
// // - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
// func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
// 	_ = log.FromContext(ctx)

// 	// TODO(user): your logic here

// 	return ctrl.Result{}, nil
// }

// // SetupWithManager sets up the controller with the Manager.
//
//	func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
//		return ctrl.NewControllerManagedBy(mgr).
//			// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
//			// For().
//			Complete(r)
//	}
package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/ormasia/aiops-operator/api/v1alpha1"
	"github.com/ormasia/aiops-operator/pkg/evidence"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PodDetectorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// RBAC：读 Pod、读 Events、写 Remediation
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=get;list;watch
//+kubebuilder:rbac:groups=aiops.example.com,resources=remediations,verbs=get;list;watch;create;update;patch
//+kubebuilder:rbac:groups=aiops.example.com,resources=remediations/status,verbs=get;update;patch

func (r *PodDetectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// 1. 拿 Pod
	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// 非异常，忽略
	if !evidence.IsAnomalous(pod) {
		return ctrl.Result{}, nil
	}

	// 2. 采证据
	evJSON, err := evidence.Collect(ctx, r.Client, pod)
	if err != nil {
		log.Error(err, "collect evidence failed")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// 3. 幂等 upsert Remediation（按 Pod 名 + 命名空间生成）
	rem := &v1alpha1.Remediation{}
	remName := fmt.Sprintf("pod-%s", pod.Name)
	err = r.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: remName}, rem)
	if apierrors.IsNotFound(err) {
		rem = &v1alpha1.Remediation{
			ObjectMeta: meta.ObjectMeta{
				Namespace: pod.Namespace,
				Name:      remName,
				OwnerReferences: []meta.OwnerReference{
					*meta.NewControllerRef(pod, corev1.SchemeGroupVersion.WithKind("Pod")),
				},
			},
			Spec: v1alpha1.RemediationSpec{
				TargetRef: corev1.ObjectReference{
					APIVersion: "v1", Kind: "Pod",
					Namespace: pod.Namespace, Name: pod.Name,
				},
				Evidence: apiextensionsv1.JSON{Raw: evJSON},
				Approved: false, // 默认未审批，需要人工设置为 true
			},
		}
		if err := r.Create(ctx, rem); err != nil {
			return ctrl.Result{}, err
		}
	} else if err == nil {
		// 更新证据（可能演进）
		rem.Spec.Evidence = apiextensionsv1.JSON{Raw: evJSON}
		if err := r.Update(ctx, rem); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		return ctrl.Result{}, err
	}

	// 4. 写状态：Diagnosing=True
	setCond(&rem.Status.Conditions, "Diagnosing", meta.ConditionTrue, "Detector", "CaseOpened")
	rem.Status.LastUpdateTime = meta.Now()
	if err := r.Status().Update(ctx, rem); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Remediation opened/updated", "remediation", client.ObjectKeyFromObject(rem))
	return ctrl.Result{}, nil
}

func (r *PodDetectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}

func setCond(list *[]meta.Condition, t string, st meta.ConditionStatus, reason, msg string) {
	apimeta.SetStatusCondition(list, meta.Condition{
		Type: t, Status: st, Reason: reason, Message: msg, LastTransitionTime: meta.Now(),
	})
}

// func jsonPretty(v any) string {
// 	b, _ := json.MarshalIndent(v, "", "  ")
// 	return string(b)
// }
