package controller

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ormasia/aiops-operator/api/v1alpha1"
	"github.com/ormasia/aiops-operator/pkg/llm"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrl "sigs.k8s.io/controller-runtime"
)

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

// 	aiopsv1alpha1 "github.com/ormasia/aiops-operator/api/v1alpha1"
// )

// // RemediationReconciler reconciles a Remediation object
// type RemediationReconciler struct {
// 	client.Client
// 	Scheme *runtime.Scheme
// }

// //+kubebuilder:rbac:groups=aiops.example.com,resources=remediations,verbs=get;list;watch;create;update;patch;delete
// //+kubebuilder:rbac:groups=aiops.example.com,resources=remediations/status,verbs=get;update;patch
// //+kubebuilder:rbac:groups=aiops.example.com,resources=remediations/finalizers,verbs=update

// // Reconcile is part of the main kubernetes reconciliation loop which aims to
// // move the current state of the cluster closer to the desired state.
// // TODO(user): Modify the Reconcile function to compare the state specified by
// // the Remediation object against the actual cluster state, and then
// // perform operations to make the cluster state reflect the state specified by
// // the user.
// //
// // For more details, check Reconcile and its Result here:
// // - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
// func (r *RemediationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
// 	_ = log.FromContext(ctx)

// 	// TODO(user): your logic here

// 	return ctrl.Result{}, nil
// }

// // SetupWithManager sets up the controller with the Manager.

// func (r *RemediationReconciler) SetupWithManager(mgr ctrl.Manager) error {
// 	return ctrl.NewControllerManagedBy(mgr).
// 		For(&aiopsv1alpha1.Remediation{}).
// 		Complete(r)
// }

type RemediationExecutorReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	OllamaURL   string
	OllamaModel string
}

//+kubebuilder:rbac:groups=aiops.example.com,resources=remediations,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=aiops.example.com,resources=remediations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="apps",resources=deployments,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;update;patch
// 若你需要修改更多目标对象，请相应补充 RBAC

func (r *RemediationExecutorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	rem := &v1alpha1.Remediation{}
	if err := r.Get(ctx, req.NamespacedName, rem); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 已经 Applied，无事可做
	if hasCond(rem, "Applied", meta.ConditionTrue) {
		return ctrl.Result{}, nil
	}

	// 1) 若未 Proposed，调用 LLM 产出补丁
	if !hasCond(rem, "Proposed", meta.ConditionTrue) {
		sys := `你是Kubernetes SRE助手。基于给定Evidence仅提出“最小补丁”。
严格禁止 :latest 等可变镜像标签；若建议修改 image，必须使用不可变的数字版本。
仅允许以下改动：镜像tag、imagePullSecrets、探针、资源配额。严格按 schema 输出。`

		client := llm.New(r.OllamaURL, r.OllamaModel, llm.DefaultSchema())
		res, err := client.Propose(ctx, sys, rem.Spec.Evidence.Raw)
		if err != nil {
			setCond(&rem.Status.Conditions, "Failed", meta.ConditionTrue, "LLMError", err.Error())
			rem.Status.LastUpdateTime = meta.Now()
			_ = r.Status().Update(ctx, rem)
			return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
		}

		patchRaw, _ := json.Marshal(res) // 保留完整 LLM 输出，前端/审计可看
		rem.Status.ProposedPatch = &runtime.RawExtension{Raw: patchRaw}
		setCond(&rem.Status.Conditions, "Proposed", meta.ConditionTrue, "OK", "PatchProposed")
		setCond(&rem.Status.Conditions, "ReadyForReview", meta.ConditionTrue, "OK", "WaitForApproval")
		rem.Status.LastUpdateTime = meta.Now()
		if err := r.Status().Update(ctx, rem); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Proposed patch", "remediation", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	// 2) 已 Proposed，等待审批；审批后执行（SSA + dry-run）
	if rem.Spec.Approved && !hasCond(rem, "Applied", meta.ConditionTrue) {
		if rem.Status.ProposedPatch == nil || len(rem.Status.ProposedPatch.Raw) == 0 {
			setCond(&rem.Status.Conditions, "Failed", meta.ConditionTrue, "NoPatch", "empty proposedPatch")
			rem.Status.LastUpdateTime = meta.Now()
			_ = r.Status().Update(ctx, rem)
			return ctrl.Result{}, nil
		}

		// 从 ProposedPatch 中提取第一条 action，拼成 SSA 片段（演示：实际可支持多 action / 多对象）
		var parsed struct {
			Actions []struct {
				ObjectRef struct {
					APIVersion, Kind, Namespace, Name string
				} `json:"objectRef"`
				Patch    map[string]any `json:"patch"`
				Strategy string         `json:"strategy"`
			} `json:"actions"`
		}
		if err := json.Unmarshal(rem.Status.ProposedPatch.Raw, &parsed); err != nil {
			return ctrl.Result{}, err
		}
		if len(parsed.Actions) == 0 {
			return ctrl.Result{}, nil
		}
		act := parsed.Actions[0]

		// 生成“目标对象的期望状态片段”
		ssaObj := map[string]any{
			"apiVersion": act.ObjectRef.APIVersion,
			"kind":       act.ObjectRef.Kind,
			"metadata": map[string]any{
				"name":      act.ObjectRef.Name,
				"namespace": act.ObjectRef.Namespace,
			},
		}
		for k, v := range act.Patch {
			ssaObj[k] = v
		}
		// 先做 server-side dry-run
		if err := r.serverSideApply(ctx, ssaObj, true); err != nil {
			setCond(&rem.Status.Conditions, "Failed", meta.ConditionTrue, "DryRun", err.Error())
			rem.Status.LastUpdateTime = meta.Now()
			_ = r.Status().Update(ctx, rem)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		// 再正式 SSA
		if err := r.serverSideApply(ctx, ssaObj, false); err != nil {
			setCond(&rem.Status.Conditions, "Failed", meta.ConditionTrue, "SSA", err.Error())
			rem.Status.LastUpdateTime = meta.Now()
			_ = r.Status().Update(ctx, rem)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		setCond(&rem.Status.Conditions, "Applied", meta.ConditionTrue, "OK", "PatchApplied")
		rem.Status.LastUpdateTime = meta.Now()
		if err := r.Status().Update(ctx, rem); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *RemediationExecutorReconciler) serverSideApply(ctx context.Context, obj map[string]any, dryRun bool) error {
	// 这里演示用 Unstructured + Patch Apply（等价 kubectl apply --server-side）
	// 生产中可按具体对象转强类型（apps/v1.Deployment 等）
	u := &unstructured.Unstructured{Object: obj}
	patch := client.Apply
	opts := []client.PatchOption{
		client.ForceOwnership, client.FieldOwner("aiops-operator"),
	}
	if dryRun {
		opts = append(opts, client.DryRunAll)
	}
	return r.Patch(ctx, u, patch, opts...)
}

func (r *RemediationExecutorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Remediation{}).
		Complete(r)
}

func hasCond(rem *v1alpha1.Remediation, t string, st meta.ConditionStatus) bool {
	for _, c := range rem.Status.Conditions {
		if c.Type == t && c.Status == st {
			return true
		}
	}
	return false
}

// func setCond(list *[]meta.Condition, t string, st meta.ConditionStatus, reason, msg string) {
// 	meta.SetStatusCondition(list, meta.Condition{
// 		Type: t, Status: st, Reason: reason, Message: msg, LastTransitionTime: meta.Now(),
// 	})
// }
