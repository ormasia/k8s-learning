/*
Copyright 2025.

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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var rolloutlog = logf.Log.WithName("rollout-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *Rollout) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-delivery-example-com-v1alpha1-rollout,mutating=true,failurePolicy=fail,sideEffects=None,groups=delivery.example.com,resources=rollouts,verbs=create;update,versions=v1alpha1,name=mrollout.kb.io,admissionReviewVersions=v1

var _ admission.Defaulter = &Rollout{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Rollout) Default() {
	rolloutlog.Info("default", "name", r.Name)

	// 1. 默认策略类型为 Canary
	if r.Spec.Strategy.Type == "" {
		r.Spec.Strategy.Type = Canary
		rolloutlog.Info("set default strategy type to Canary", "name", r.Name)
	}

	// 2. Canary 策略默认步骤：10% → 30% → 100%
	if r.Spec.Strategy.Type == Canary && len(r.Spec.Strategy.Steps) == 0 {
		r.Spec.Strategy.Steps = []RolloutStep{
			{Weight: 10, HoldSeconds: 60},
			{Weight: 30, HoldSeconds: 60},
			{Weight: 100},
		}
		rolloutlog.Info("set default canary steps", "name", r.Name, "steps", r.Spec.Strategy.Steps)
	}

	// 3. 分析配置默认值
	if r.Spec.Analysis.IntervalSeconds == 0 {
		r.Spec.Analysis.IntervalSeconds = 30
	}
	if r.Spec.Analysis.SuccessThreshold == 0 {
		r.Spec.Analysis.SuccessThreshold = 2
	}
	if r.Spec.Analysis.FailureThreshold == 0 {
		r.Spec.Analysis.FailureThreshold = 2
	}

	// 4. 默认开启失败回滚
	if !r.Spec.RollbackOnFailure {
		r.Spec.RollbackOnFailure = true
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-delivery-example-com-v1alpha1-rollout,mutating=false,failurePolicy=fail,sideEffects=None,groups=delivery.example.com,resources=rollouts,verbs=create;update,versions=v1alpha1,name=vrollout.kb.io,admissionReviewVersions=v1

var _ admission.Validator = &Rollout{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Rollout) ValidateCreate() (admission.Warnings, error) {
	rolloutlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil, r.validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Rollout) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	rolloutlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil, r.validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Rollout) ValidateDelete() (admission.Warnings, error) {
	rolloutlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

// validate 方法逻辑不变
func (r *Rollout) validate() error {
	var allErrs field.ErrorList
	fp := field.NewPath("spec")
	if r.Spec.Strategy.Type == BlueGreen && len(r.Spec.Strategy.Steps) > 0 {
		allErrs = append(allErrs, field.Invalid(fp.Child("strategy", "steps"), r.Spec.Strategy.Steps, "BlueGreen must not define steps"))
	}
	if r.Spec.Strategy.Type == Canary && len(r.Spec.Strategy.Steps) == 0 {
		allErrs = append(allErrs, field.Required(fp.Child("strategy", "steps"), "steps required for canary"))
	} else {
		prev := int32(-1)
		for i, s := range r.Spec.Strategy.Steps {
			if s.Weight < 0 || s.Weight > 100 {
				allErrs = append(allErrs, field.Invalid(fp.Child("strategy", "steps").Index(i).Child("weight"), s.Weight, "0..100"))
			}
			if s.Weight < prev {
				allErrs = append(allErrs, field.Invalid(fp.Child("strategy", "steps"), r.Spec.Strategy.Steps, "weights must be non-decreasing"))
			}
			prev = s.Weight
		}
	}
	if len(r.Spec.Analysis.Metrics) == 0 {
		allErrs = append(allErrs, field.Required(fp.Child("analysis", "metrics"), "at least 1 metric"))
	}
	if r.Spec.Traffic.Host == "" || r.Spec.Traffic.StableService == "" || r.Spec.Traffic.CanaryService == "" {
		allErrs = append(allErrs, field.Required(fp.Child("traffic"), "host/stableService/canaryService required"))
	}
	if len(allErrs) == 0 {
		return nil
	}
	return allErrs.ToAggregate()
}
