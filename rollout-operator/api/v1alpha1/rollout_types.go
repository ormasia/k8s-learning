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

// package v1alpha1

// import (
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// )

// // EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// // NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// // RolloutSpec defines the desired state of Rollout
// type RolloutSpec struct {
// 	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
// 	// Important: Run "make" to regenerate code after modifying this file

// 	// Foo is an example field of Rollout. Edit rollout_types.go to remove/update
// 	Foo string `json:"foo,omitempty"`
// }

// // RolloutStatus defines the observed state of Rollout
// type RolloutStatus struct {
// 	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
// 	// Important: Run "make" to regenerate code after modifying this file
// }

// //+kubebuilder:object:root=true
// //+kubebuilder:subresource:status

// // Rollout is the Schema for the rollouts API
// type Rollout struct {
// 	metav1.TypeMeta   `json:",inline"`
// 	metav1.ObjectMeta `json:"metadata,omitempty"`

// 	Spec   RolloutSpec   `json:"spec,omitempty"`
// 	Status RolloutStatus `json:"status,omitempty"`
// }

// //+kubebuilder:object:root=true

// // RolloutList contains a list of Rollout
// type RolloutList struct {
// 	metav1.TypeMeta `json:",inline"`
// 	metav1.ListMeta `json:"metadata,omitempty"`
// 	Items           []Rollout `json:"items"`
// }

//	func init() {
//		SchemeBuilder.Register(&Rollout{}, &RolloutList{})
//	}
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type StrategyType string

const (
	Canary    StrategyType = "Canary"
	BlueGreen StrategyType = "BlueGreen"
)

type RolloutStep struct {
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Weight int32 `json:"weight"`
	// +kubebuilder:default=180
	// +kubebuilder:validation:Minimum=0
	HoldSeconds int32 `json:"holdSeconds,omitempty"`
}

type RolloutStrategy struct {
	// +kubebuilder:default=Canary
	Type StrategyType `json:"type,omitempty"`
	// Canary 模式使用；BlueGreen 留空
	// +optional
	Steps []RolloutStep `json:"steps,omitempty"`
}

type MetricCheck struct {
	Name      string `json:"name"`
	PromQL    string `json:"promQL"`
	Threshold string `json:"threshold"`
	// +kubebuilder:validation:Enum=LT;GT
	Compare string `json:"compare"`
}

type AnalysisSpec struct {
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	IntervalSeconds int32 `json:"intervalSeconds,omitempty"`
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=1
	SuccessThreshold int32 `json:"successThreshold,omitempty"`
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=1
	FailureThreshold int32 `json:"failureThreshold,omitempty"`
	// 最少 1 个；先可用“就绪率”代替
	Metrics []MetricCheck `json:"metrics"`
}

type TrafficSpec struct {
	// +kubebuilder:validation:Enum=NginxIngress
	Provider      string `json:"provider"`
	Host          string `json:"host"`
	StableService string `json:"stableService"`
	CanaryService string `json:"canaryService"`
}

type TargetRef struct {
	// 先固定 Deployment；后续可扩展
	// +kubebuilder:validation:Enum=Deployment
	Kind string `json:"kind"`
	Name string `json:"name"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
}

type RolloutSpec struct {
	TargetRef TargetRef       `json:"targetRef"`
	Strategy  RolloutStrategy `json:"strategy"`
	Analysis  AnalysisSpec    `json:"analysis"`
	Traffic   TrafficSpec     `json:"traffic"`
	// +kubebuilder:default=true
	RollbackOnFailure bool `json:"rollbackOnFailure,omitempty"`
}

type RolloutPhase string

const (
	PhaseIdle        RolloutPhase = "Idle"
	PhaseProgressing RolloutPhase = "Progressing"
	PhaseAnalyzing   RolloutPhase = "Analyzing"
	PhaseSucceeded   RolloutPhase = "Succeeded"
	PhaseFailed      RolloutPhase = "Failed"
	PhaseRolledBack  RolloutPhase = "RolledBack"
)

type RolloutStatus struct {
	Phase          RolloutPhase `json:"phase,omitempty"`
	StepIndex      int32        `json:"stepIndex,omitempty"`
	StableRevision string       `json:"stableRevision,omitempty"`
	CanaryRevision string       `json:"canaryRevision,omitempty"`
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Step",type=integer,JSONPath=`.status.stepIndex`
// +kubebuilder:printcolumn:name="Strategy",type=string,JSONPath=`.spec.strategy.type`
// +kubebuilder:webhook:path=/mutate-delivery-example-com-v1alpha1-rollout,mutating=true,failurePolicy=fail,sideEffects=None,groups=delivery.example.com,resources=rollouts,verbs=create;update,versions=v1alpha1,name=mrollout.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-delivery-example-com-v1alpha1-rollout,mutating=false,failurePolicy=fail,sideEffects=None,groups=delivery.example.com,resources=rollouts,verbs=create;update,versions=v1alpha1,name=vrollout.kb.io,admissionReviewVersions=v1
type Rollout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RolloutSpec   `json:"spec"`
	Status            RolloutStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type RolloutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Rollout `json:"items"`
}

func init() { SchemeBuilder.Register(&Rollout{}, &RolloutList{}) }
