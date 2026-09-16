package kubeaudit

import (
	"context"
	"strings"
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

		// 端口 :5000 在 / 之前，不是 tag 分隔符 → 镜像名里没 tag
		{"registry 带端口无 tag", "registry.mycorp.com:5000/nginx", true},
		// :1.27 在 / 之后，是 tag
		{"registry 带端口有 tag", "registry.mycorp.com:5000/nginx:1.27", false},
		// digest 是权威引用，不需要 tag
		{"digest 引用", "nginx@sha256:abcdef1234", false},
		// digest 优先，即使 tag 是 latest 也不报
		{"latest + digest", "nginx:latest@sha256:abcdef1234", false},
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

func TestAudit_NotReady(t *testing.T) {
	tests := []struct {
		name      string
		pod       *corev1.Pod
		wantMsg   string // Message 里必须包含的子串（空=不检查）
		wantCount int    // finding的个数
	}{
		{
			name: "Pending + ImagePullBackOff → 报",
			pod: podWith("default", "bad", "nginx:1.27", func(p *corev1.Pod) {
				p.Status.Phase = corev1.PodPending
				p.Status.ContainerStatuses = []corev1.ContainerStatus{{
					Name: "app", Ready: false,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
					},
				}}
			}),
			wantMsg:   "ImagePullBackOff",
			wantCount: 1,
		},
		{
			name: "Pending + Unschedulable + 空 ContainerStatuses → 报 (pod 级)",
			pod: podWith("default", "unsched", "nginx:1.27", func(p *corev1.Pod) {
				p.Status.Phase = corev1.PodPending
				p.Status.Reason = "Unschedulable"
				// ⚠️ 关键：调度不上的 Pod 根本没有 ContainerStatuses
				p.Status.ContainerStatuses = nil
			}),
			wantMsg:   "Unschedulable",
			wantCount: 1,
		},
		{
			name: "Failed + Evicted + 空 ContainerStatuses → 报 (pod 级)",
			pod: podWith("default", "evict", "nginx:1.27", func(p *corev1.Pod) {
				p.Status.Phase = corev1.PodFailed
				p.Status.Reason = "Evicted"
				p.Status.ContainerStatuses = nil
			}),
			wantMsg:   "Evicted",
			wantCount: 1,
		},
		{
			name: "Succeeded → 不报（Job 跑完不算问题）",
			pod: podWith("default", "done", "busybox:1.36", func(p *corev1.Pod) {
				p.Status.Phase = corev1.PodSucceeded
				p.Status.ContainerStatuses = []corev1.ContainerStatus{{
					Name: "app", Ready: false,
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{Reason: "Completed", ExitCode: 0},
					},
				}}
			}),
			wantCount: 0,
		},
		{
			name: "Running + 容器 Ready=false + CrashLoopBackOff → 报",
			pod: podWith("default", "flaky", "nginx:1.27", func(p *corev1.Pod) {
				p.Status.ContainerStatuses = []corev1.ContainerStatus{{
					Name: "app", Ready: false,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
					},
				}}
			}),
			wantMsg:   "CrashLoopBackOff",
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientset(tt.pod)
			got, err := Audit(context.Background(), client, Options{MaxRestarts: 3})
			if err != nil {
				t.Fatalf("Audit: %v", err)
			}
			var notReady []Finding
			for _, f := range got {
				if f.Rule == "not-ready" {
					notReady = append(notReady, f)
				}
			}
			has := len(notReady) > 0
			if len(notReady) != tt.wantCount {
				t.Errorf("not-ready finding count=%v, want %v", len(notReady), tt.wantCount)
			}
			if has && tt.wantMsg != "" && !strings.Contains(notReady[0].Message, tt.wantMsg) {
				t.Errorf("Message should contain %q, got: %s", tt.wantMsg, notReady[0].Message)
			}
		})
	}
}

func TestAudit_Restarts(t *testing.T) {
	tests := []struct {
		name        string
		maxRestarts int32
		pod         *corev1.Pod
		want        bool   // 该不该报 restarts
		wantMsg     string // Message 里必须包含的子串
	}{
		{
			name:        "RestartCount=5, MaxRestarts=3, OOMKilled → 报",
			maxRestarts: 3,
			pod: podWith("default", "boom", "app:v1", func(p *corev1.Pod) {
				p.Status.ContainerStatuses = []corev1.ContainerStatus{{
					Name: "app", Ready: true, RestartCount: 5,
					LastTerminationState: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137},
					},
				}}
			}),
			want:    true,
			wantMsg: "OOMKilled",
		},
		{
			name:        "RestartCount=3, MaxRestarts=3 → 不报（边界 > 不是 >=）",
			maxRestarts: 3,
			pod: podWith("default", "ok", "app:v1", func(p *corev1.Pod) {
				p.Status.ContainerStatuses = []corev1.ContainerStatus{{
					Name: "app", Ready: true, RestartCount: 3,
				}}
			}),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientset(tt.pod)
			got, err := Audit(context.Background(), client, Options{MaxRestarts: tt.maxRestarts})
			if err != nil {
				t.Fatalf("Audit: %v", err)
			}
			var restarts []Finding
			for _, f := range got {
				if f.Rule == "restarts" {
					restarts = append(restarts, f)
				}
			}
			has := len(restarts) > 0
			if has != tt.want {
				t.Errorf("has restarts finding=%v, want %v", has, tt.want)
			}
			if has && tt.wantMsg != "" && !strings.Contains(restarts[0].Message, tt.wantMsg) {
				t.Errorf("Message should contain %q, got: %s", tt.wantMsg, restarts[0].Message)
			}
		})
	}
}

func TestAudit_NoLimits(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool // 该不该报 no-limits
	}{
		{
			name: "Limits 为 nil → 报",
			pod: podWith("default", "bare", "app:v1", func(p *corev1.Pod) {
				p.Spec.Containers[0].Resources.Limits = nil
			}),
			want: true,
		},
		{
			name: "只有 cpu limit 没有 memory → 报",
			pod: podWith("default", "partial", "app:v1", func(p *corev1.Pod) {
				p.Spec.Containers[0].Resources.Limits = corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("500m"),
				}
			}),
			want: true,
		},
		{
			name: "有 memory limit → 不报",
			pod:  podWith("default", "ok", "app:v1", nil),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientset(tt.pod)
			got, err := Audit(context.Background(), client, Options{MaxRestarts: 3})
			if err != nil {
				t.Fatalf("Audit: %v", err)
			}
			var noLimits []Finding
			for _, f := range got {
				if f.Rule == "no-limits" {
					noLimits = append(noLimits, f)
				}
			}
			if (len(noLimits) > 0) != tt.want {
				t.Errorf("has no-limits finding=%v, want %v", len(noLimits) > 0, tt.want)
			}
		})
	}
}

func TestAudit_NamespaceFilter(t *testing.T) {
	// namespace "a"：容器没有 memory limit → 应该被找到
	badInA := podWith("a", "bad-a", "app:v1", func(p *corev1.Pod) {
		p.Spec.Containers[0].Resources.Limits = nil
	})
	// namespace "b"：同样没有 memory limit → 被过滤掉
	badInB := podWith("b", "bad-b", "app:v1", func(p *corev1.Pod) {
		p.Spec.Containers[0].Resources.Limits = nil
	})

	client := fake.NewClientset(badInA, badInB)

	t.Run("只查 namespace a", func(t *testing.T) {
		got, err := Audit(context.Background(), client, Options{Namespace: "a", MaxRestarts: 3})
		if err != nil {
			t.Fatalf("Audit: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("want 1 finding, got %d: %+v", len(got), got)
		}
		if got[0].Namespace != "a" {
			t.Errorf("finding namespace = %q, want %q", got[0].Namespace, "a")
		}
	})

	t.Run("查所有 namespace", func(t *testing.T) {
		got, err := Audit(context.Background(), client, Options{Namespace: "", MaxRestarts: 3})
		if err != nil {
			t.Fatalf("Audit: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("want 2 findings, got %d: %+v", len(got), got)
		}
	})
}
