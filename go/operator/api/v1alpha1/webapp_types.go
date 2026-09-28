/*
Copyright 2026.

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
	"k8s.io/apimachinery/pkg/runtime"
)

// ⭐ 这个文件是【唯一】需要人写的 API 定义（D20 §2）。
// 改完必须跑 `make generate manifests`：
//   generate   → zz_generated.deepcopy.go（D19 §1.2 说的生成代码，这次轮到你自己生成）
//   manifests  → config/crd/bases/*.yaml（CRD 的 OpenAPI schema 从下面的注释 marker 来）
// ⚠️ 改了字段忘了 generate：编译能过，DeepCopy 里少字段，运行时诡异。

// WebAppSpec 是用户想要的样子（D19 §2.3 ④：spec 是别人的）。
//
// 设计上它是 D19 那个 Helm chart 的 Deployment + Service 的最小子集：
// 给一个镜像、几个副本、一个端口、一组环境变量，controller 负责把它们变成真实资源。
type WebAppSpec struct {
	// image 是容器镜像，含 tag。⚠️ 别用 latest（D19 §7 讨论过为什么）。
	// +kubebuilder:validation:MinLength=1
	// +required
	Image string `json:"image"`

	// replicas 是期望副本数。指针是为了区分「没写」和「写了 0」（D12 §1.2 那条）。
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// port 是容器监听的端口，Service 也用它。
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=8080
	// +optional
	Port int32 `json:"port,omitempty"`

	// healthPath 是 readiness 探针的路径。
	// +kubebuilder:default="/healthz"
	// +optional
	HealthPath string `json:"healthPath,omitempty"`

	// TODO(D20) ①：加一个 Env map[string]string 字段（json:"env,omitempty"，+optional），
	// 注入成容器的环境变量 —— golearn-api 需要 DATABASE_URL 才能起来。
	// 加完跑 `make generate manifests`，然后看 config/crd/bases/ 里的 yaml 多了什么。
	// +optional
	Env map[string]string `json:"env,omitempty"`
}

// WebAppStatus 是 controller 观察到的现实（D19 §2.3 ④：status 是你的）。
//
// ⚠️ 只有 controller 写这里，而且是通过 status 子资源（r.Status().Update）——
// 和写 spec 走的是不同的 API 端点，RBAC 也分开（见 controller 里的 rbac marker）。
type WebAppStatus struct {
	// observedGeneration 是 controller 最后处理过的 metadata.generation。
	// ⭐ 用户每改一次 spec，generation +1；status 里记下你看到的那个，
	// 读 status 的人就能判断「这个 status 是不是针对当前 spec 的」。
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// readyReplicas 从 Deployment.Status.ReadyReplicas 抄过来。
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// conditions 是标准的状态条件列表。本课只用一个：
	//   Available  True  = readyReplicas == spec.replicas
	//              False = 还没到 / 出错了（Reason 里说为什么）
	// 用 meta.SetStatusCondition 维护，别手写 append（它会处理去重和 LastTransitionTime）。
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// 下面几行 marker 决定 kubectl get webapp 显示哪些列（§2.2）。
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=wa
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=`.spec.replicas`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Available",type=string,JSONPath=`.status.conditions[?(@.type=="Available")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// WebApp is the Schema for the webapps API
type WebApp struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of WebApp
	// +required
	Spec WebAppSpec `json:"spec"`

	// status defines the observed state of WebApp
	// +optional
	Status WebAppStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// WebAppList contains a list of WebApp
type WebAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []WebApp `json:"items"`
}

// ConditionAvailable 是本课唯一用到的 condition 类型。
const ConditionAvailable = "Available"

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &WebApp{}, &WebAppList{})
		return nil
	})
}
