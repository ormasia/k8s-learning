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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NginxSpec 定义 Nginx 实例的期望配置
type NginxSpec struct {
	// 副本数（默认 1）
	Replicas *int32 `json:"replicas,omitempty"`
	// Nginx 镜像版本（默认 nginx:1.23）
	Image string `json:"image,omitempty"`
	// 服务暴露端口（默认 80）
	ServicePort int32 `json:"servicePort,omitempty"`
}

// NginxStatus 定义 Nginx 实例的实际状态
type NginxStatus struct {
	// 当前运行的副本数
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	// 服务访问地址（格式：<service-name>.<namespace>:<port>）
	ServiceAddress string `json:"serviceAddress,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Nginx is the Schema for the nginxes API
type Nginx struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NginxSpec   `json:"spec,omitempty"`
	Status NginxStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NginxList contains a list of Nginx
type NginxList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Nginx `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Nginx{}, &NginxList{})
}
