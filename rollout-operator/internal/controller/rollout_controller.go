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

// 	deliveryv1alpha1 "github.com/ormasia/rollout-operator/api/v1alpha1"
// )

// // RolloutReconciler reconciles a Rollout object
// type RolloutReconciler struct {
// 	client.Client
// 	Scheme *runtime.Scheme
// }

// //+kubebuilder:rbac:groups=delivery.example.com,resources=rollouts,verbs=get;list;watch;create;update;patch;delete
// //+kubebuilder:rbac:groups=delivery.example.com,resources=rollouts/status,verbs=get;update;patch
// //+kubebuilder:rbac:groups=delivery.example.com,resources=rollouts/finalizers,verbs=update

// // Reconcile is part of the main kubernetes reconciliation loop which aims to
// // move the current state of the cluster closer to the desired state.
// // TODO(user): Modify the Reconcile function to compare the state specified by
// // the Rollout object against the actual cluster state, and then
// // perform operations to make the cluster state reflect the state specified by
// // the user.
// //
// // For more details, check Reconcile and its Result here:
// // - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
// func (r *RolloutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
// 	_ = log.FromContext(ctx)

// 	// TODO(user): your logic here

// 	return ctrl.Result{}, nil
// }

// // SetupWithManager sets up the controller with the Manager.
//
//	func (r *RolloutReconciler) SetupWithManager(mgr ctrl.Manager) error {
//		return ctrl.NewControllerManagedBy(mgr).
//			For(&deliveryv1alpha1.Rollout{}).
//			Complete(r)
//	}
package controller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	dlv1 "github.com/ormasia/rollout-operator/api/v1alpha1"
	"github.com/ormasia/rollout-operator/pkg/analysis"
	"github.com/ormasia/rollout-operator/pkg/traffic"
)

type RolloutReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Traffic  traffic.Provider
	Analysis analysis.Engine
}

// +kubebuilder:rbac:groups=delivery.example.com,resources=rollouts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=delivery.example.com,resources=rollouts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=delivery.example.com,resources=rollouts/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

func (r *RolloutReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	lg := log.FromContext(ctx)

	lg.Info("Reconciling Rollout", "namespace", req.Namespace, "name", req.Name)
	var ro dlv1.Rollout
	if err := r.Get(ctx, req.NamespacedName, &ro); err != nil {
		if apierrors.IsNotFound(err) {
			lg.Info("Rollout resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		lg.Error(err, "Failed to get Rollout")
		return ctrl.Result{}, err
	}

	// 确保 stable/canary 资源存在
	if err := r.ensureWorkloads(ctx, &ro); err != nil {
		lg.Error(err, "Failed to ensure workloads")
		return ctrl.Result{}, err
	}

	// 初始化状态
	if ro.Status.Phase == "" {
		lg.Info("Initialize rollout status")
		ro.Status.Phase = dlv1.PhaseProgressing
		ro.Status.StepIndex = 0
		if err := r.Status().Update(ctx, &ro); err != nil {
			lg.Error(err, "Failed to update rollout status")
			return ctrl.Result{}, err
		}
	}

	switch ro.Spec.Strategy.Type {
	case dlv1.BlueGreen:
		lg.Info("BlueGreen strategy: promoting canary to 100%", "host", ro.Spec.Traffic.Host)
		if err := r.Traffic.Promote(ctx, ro.Spec.Traffic.Host, ro.Spec.Traffic.StableService, ro.Spec.Traffic.CanaryService); err != nil {
			lg.Error(err, "Failed to promote traffic")
			return ctrl.Result{}, err
		}
		lg.Info("BlueGreen promoted, marking Succeeded")
		ro.Status.Phase = dlv1.PhaseSucceeded
		return r.updateStatus(ctx, &ro)

	default: // Canary
		steps := ro.Spec.Strategy.Steps
		idx := int(ro.Status.StepIndex)
		if idx >= len(steps) {
			lg.Info("Canary finished all steps, promoting", "host", ro.Spec.Traffic.Host)
			if err := r.Traffic.Promote(ctx, ro.Spec.Traffic.Host, ro.Spec.Traffic.StableService, ro.Spec.Traffic.CanaryService); err != nil {
				lg.Error(err, "Failed to promote traffic")
				return ctrl.Result{}, err
			}
			lg.Info("Canary promoted, marking Succeeded")
			ro.Status.Phase = dlv1.PhaseSucceeded
			return r.updateStatus(ctx, &ro)
		}
		step := steps[idx]
		lg.Info("Canary step", "index", idx, "weight", step.Weight, "holdSeconds", step.HoldSeconds)

		// 调整权重
		if err := r.Traffic.SetWeight(ctx, ro.Spec.Traffic.Host, ro.Spec.Traffic.StableService, ro.Spec.Traffic.CanaryService, step.Weight); err != nil {
			lg.Error(err, "Failed to set traffic weight")
			return ctrl.Result{}, err
		}
		lg.Info("Traffic weight set", "host", ro.Spec.Traffic.Host, "weight", step.Weight)
		ro.Status.Phase = dlv1.PhaseAnalyzing
		if err := r.Status().Update(ctx, &ro); err != nil {
			lg.Error(err, "Failed to update rollout status")
			return ctrl.Result{}, err
		}

		// 调用分析引擎，检查本次 Canary 对应的 Deployment 是否就绪
		lg.Info("Evaluating canary readiness", "deployment", ro.Name+"-canary", "namespace", ro.Namespace)
		res, err := r.Analysis.Evaluate(ctx, analysis.Spec{}, map[string]string{
			"app":        ro.Spec.TargetRef.Name,
			"deployment": ro.Name + "-canary",
			"namespace":  ro.Namespace,
		})
		if err != nil {
			lg.Error(err, "Failed to evaluate analysis")
			return ctrl.Result{}, err
		}
		lg.Info("Analysis result", "passed", res.Passed, "reason", res.Reason)

		if res.Passed {
			lg.Info("Analysis passed, advancing to next step", "nextStepIndex", ro.Status.StepIndex+1)
			ro.Status.StepIndex++
			ro.Status.Phase = dlv1.PhaseProgressing
			if err := r.Status().Update(ctx, &ro); err != nil {
				lg.Error(err, "Failed to update rollout status")
				return ctrl.Result{}, err
			}
			lg.Info("Requeueing after hold seconds", "seconds", step.HoldSeconds)
			return ctrl.Result{RequeueAfter: time.Duration(step.HoldSeconds) * time.Second}, nil
		} else {
			if ro.Spec.RollbackOnFailure {
				lg.Info("Analysis failed, rollback enabled -> resetting traffic")
				_ = r.Traffic.Reset(ctx, ro.Spec.Traffic.Host, ro.Spec.Traffic.StableService, ro.Spec.Traffic.CanaryService)
				ro.Status.Phase = dlv1.PhaseRolledBack
			} else {
				lg.Info("Analysis failed, rollback disabled -> marking Failed")
				ro.Status.Phase = dlv1.PhaseFailed
			}
			return r.updateStatus(ctx, &ro)
		}
	}
}

func (r *RolloutReconciler) ensureWorkloads(ctx context.Context, ro *dlv1.Rollout) error {
	lg := log.FromContext(ctx)
	for _, track := range []string{"stable", "canary"} {
		depName := ro.Name + "-" + track
		svcName := ro.Spec.Traffic.StableService
		if track == "canary" {
			svcName = ro.Spec.Traffic.CanaryService
		}

		// 统一的对象标签（用于 kubectl -l 选择器）
		objLabels := map[string]string{
			"app":   ro.Spec.TargetRef.Name,
			"track": track,
		}
		lg.Info("Ensuring workload", "deployment", depName, "service", svcName, "labels", objLabels)

		// Deployment
		var dep appsv1.Deployment
		if err := r.Get(ctx, client.ObjectKey{Name: depName, Namespace: ro.Namespace}, &dep); err != nil {
			lg.Info("Deployment status",
				"name", dep.Name,
				"replicas", dep.Status.Replicas,
				"readyReplicas", dep.Status.ReadyReplicas,
				"availableReplicas", dep.Status.AvailableReplicas)
			if apierrors.IsNotFound(err) {
				replicas := int32(2)
				newDep := appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      depName,
						Namespace: ro.Namespace,
						Labels:    map[string]string{"app": ro.Spec.TargetRef.Name, "track": track},
					},
					Spec: appsv1.DeploymentSpec{
						Replicas: &replicas,
						Selector: &metav1.LabelSelector{
							MatchLabels: map[string]string{"app": ro.Spec.TargetRef.Name, "track": track},
						},
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": ro.Spec.TargetRef.Name, "track": track},
							},
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{{
									Name:  ro.Spec.TargetRef.Name,
									Image: "nginx:1.25",
									Ports: []corev1.ContainerPort{{ContainerPort: ro.Spec.TargetRef.Port}},
								}},
							},
						},
					},
				}
				// 设置 OwnerReference，方便级联与事件追踪
				if err := controllerutil.SetControllerReference(ro, &newDep, r.Scheme); err != nil {
					return err
				}
				log.FromContext(ctx).Info("Creating Deployment", "name", depName, "labels", objLabels)
				if err := r.Create(ctx, &newDep); err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			// 已存在：确保对象标签齐全
			if dep.Labels == nil {
				dep.Labels = map[string]string{}
			}
			changed := false
			for k, v := range objLabels {
				if dep.Labels[k] != v {
					dep.Labels[k] = v
					changed = true
				}
			}
			// 确保已有 Deployment 的 OwnerReference 指向当前 Rollout
			if !metav1.IsControlledBy(&dep, ro) {
				if err := controllerutil.SetControllerReference(ro, &dep, r.Scheme); err != nil {
					log.FromContext(ctx).Info("Skip setting ownerRef for Deployment (already controlled)", "name", depName, "err", err.Error())
				} else {
					changed = true
				}
			}
			if changed {
				log.FromContext(ctx).Info("Patching Deployment labels", "name", depName, "labels", dep.Labels)
				if err := r.Update(ctx, &dep); err != nil {
					return err
				}
			}
		}

		// Service
		var svc corev1.Service
		if err := r.Get(ctx, client.ObjectKey{Name: svcName, Namespace: ro.Namespace}, &svc); err != nil {
			if apierrors.IsNotFound(err) {
				newSvc := corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      svcName,
						Namespace: ro.Namespace,
						Labels:    map[string]string{"app": ro.Spec.TargetRef.Name, "track": track},
					},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": ro.Spec.TargetRef.Name, "track": track},
						Ports: []corev1.ServicePort{{
							Port:       ro.Spec.TargetRef.Port,
							TargetPort: intstr.FromInt(int(ro.Spec.TargetRef.Port)),
						}},
					},
				}
				if err := controllerutil.SetControllerReference(ro, &newSvc, r.Scheme); err != nil {
					return err
				}
				log.FromContext(ctx).Info("Creating Service", "name", svcName, "labels", objLabels)
				if err := r.Create(ctx, &newSvc); err != nil {
					return err
				}
			} else {
				return err
			}
		} else {
			// 已存在：确保对象标签齐全
			if svc.Labels == nil {
				svc.Labels = map[string]string{}
			}
			changed := false
			for k, v := range objLabels {
				if svc.Labels[k] != v {
					svc.Labels[k] = v
					changed = true
				}
			}
			// 确保已有 Service 的 OwnerReference 指向当前 Rollout
			if !metav1.IsControlledBy(&svc, ro) {
				if err := controllerutil.SetControllerReference(ro, &svc, r.Scheme); err != nil {
					log.FromContext(ctx).Info("Skip setting ownerRef for Service (already controlled)", "name", svcName, "err", err.Error())
				} else {
					changed = true
				}
			}
			if changed {
				log.FromContext(ctx).Info("Patching Service labels", "name", svcName, "labels", svc.Labels)
				if err := r.Update(ctx, &svc); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (r *RolloutReconciler) updateStatus(ctx context.Context, ro *dlv1.Rollout) (ctrl.Result, error) {
	lg := log.FromContext(ctx)
	lg.Info("Updating rollout status", "phase", ro.Status.Phase, "stepIndex", ro.Status.StepIndex)

	if err := r.Status().Update(ctx, ro); err != nil {
		lg.Error(err, "Failed to update rollout status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *RolloutReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dlv1.Rollout{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
