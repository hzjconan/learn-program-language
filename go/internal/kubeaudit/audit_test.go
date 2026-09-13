package kubeaudit

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ⭐ fake.NewClientset：client-go 自带的内存 fake，和 D15 讲的 fakeRepo 是一回事，
// 只是官方替你写好了。它是【真的能用的简化实现】—— List 会返回你 Add 进去的对象。
//
// ⚠️ 但也别忘了 D15 §2 的教训：fake 看不见「真实现和你以为的不一致」。
// 比如真 API server 的 List 会分页（Continue），fake 不会 ——
// 你的分页代码在这里【测不出 bug】。这就是为什么练习最后要真连 kind 集群跑一遍。

// podWith 造一个只有一个容器的 Pod，测试用。
func podWith(ns, name, image string, mutate func(*corev1.Pod)) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name:  "app",
				Image: image,
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
				},
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "app", Ready: true, RestartCount: 0,
			}},
		},
	}
	if mutate != nil {
		mutate(p)
	}
	return p
}

// 一个健康的 Pod 什么都不该报 —— 先锁住「没有误报」。
func TestAudit_HealthyPodHasNoFindings(t *testing.T) {
	client := fake.NewClientset(podWith("default", "ok", "nginx:1.27", nil))

	got, err := Audit(context.Background(), client, Options{MaxRestarts: 3})
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("健康的 Pod 报了 %d 条: %+v", len(got), got)
	}
}

func TestAudit_LatestTag(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  bool // 该不该报
	}{
		{"显式 latest", "nginx:latest", true},
		{"没写 tag", "nginx", true},
		{"正常 tag", "nginx:1.27", false},
		// TODO(D19)：补上这几种，想清楚每一种为什么是那个答案：
		//   "registry.mycorp.com:5000/nginx"           registry 带端口、镜像没 tag
		//   "registry.mycorp.com:5000/nginx:1.27"
		//   "nginx@sha256:abc..."                       digest 引用，没有 tag 但【确定】
		//   "nginx:latest@sha256:abc..."                tag + digest —— digest 说了算
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientset(podWith("default", "p", tt.image, nil))
			got, err := Audit(context.Background(), client, Options{MaxRestarts: 3})
			if err != nil {
				t.Fatalf("Audit: %v", err)
			}
			has := false
			for _, f := range got {
				if f.Rule == "latest-tag" {
					has = true
				}
			}
			if has != tt.want {
				t.Errorf("image=%q 报了 latest-tag=%v, want %v\nfindings: %+v", tt.image, has, tt.want, got)
			}
		})
	}
}

// TODO(D19)：下面三条自己写。每条至少一个「该报」一个「不该报」。
//
// TestAudit_NotReady
//   - Phase=Pending + Waiting.Reason=ImagePullBackOff → 报，Message 里要有 ImagePullBackOff
//   - Phase=Succeeded（跑完的 Job）→ 不报
//   - Running 但某容器 Ready=false → 报
//
// TestAudit_Restarts
//   - RestartCount=5, MaxRestarts=3 → 报；LastTerminationState.Terminated.Reason=OOMKilled 要出现在 Message
//   - RestartCount=3, MaxRestarts=3 → 不报（边界：是 > 不是 >=）
//
// TestAudit_NoLimits
//   - Limits 为 nil → 报
//   - 只有 cpu limit 没有 memory → 报
//
// TestAudit_NamespaceFilter
//   - 两个 namespace 各一个坏 Pod，Options.Namespace="a" 只报 a 的
