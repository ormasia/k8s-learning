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
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// RemediationSpec defines the desired state of Remediation
type RemediationSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// 指向异常对象（通常是 Deployment；也可以是 Pod）
	// +kubebuilder:validation:Required
	TargetRef corev1.ObjectReference `json:"targetRef"`
	// 监控器采集的证据（Pod/Events/previous logs 的打包 JSON）
	// 允许承载任意 JSON 证据，不被裁剪
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Optional
	Evidence apiextensionsv1.JSON `json:"evidence,omitempty"`
	// 审批开关（默认 false；改为 true 后，执行器才会落补丁）
	Approved bool `json:"approved,omitempty"`
}

// RemediationStatus defines the observed state of Remediation
type RemediationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// LLM 生成的“最小补丁”（以 SSA 可应用的片段为准）
	ProposedPatch *runtime.RawExtension `json:"proposedPatch,omitempty"`
	// 标准 Conditions：Diagnosing/Proposed/ReadyForReview/Applied/Failed
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// 最近更新时间（方便观测）
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
}

// Remediation is the Schema for the remediations API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=rem
// +kubebuilder:printcolumn:name="Proposed",type=string,JSONPath=`.status.conditions[?(@.type=="Proposed")].status`
// +kubebuilder:printcolumn:name="ReadyForReview",type=string,JSONPath=`.status.conditions[?(@.type=="ReadyForReview")].status`
// +kubebuilder:printcolumn:name="Applied",type=string,JSONPath=`.status.conditions[?(@.type=="Applied")].status`
type Remediation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemediationSpec   `json:"spec,omitempty"`
	Status RemediationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// RemediationList contains a list of Remediation
type RemediationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Remediation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Remediation{}, &RemediationList{})
}
