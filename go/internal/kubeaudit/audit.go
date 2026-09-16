// Package kubeaudit 巡检集群里的 Pod，报告常见的健康/规范问题（D19 练习 ①）。
//
// # 设计
//
//	Audit(ctx, client, opts)  拉一次 Pod 列表，对每个 Pod 跑所有规则，汇总 Finding
//	rule                      一条规则 = 一个函数：拿一个 Pod，返回零到多个 Finding
//
// ⭐ client 的类型是 kubernetes.Interface，不是 *kubernetes.Clientset ——
// 这样测试能传 fake.NewClientset()（D15 的 fake 又来了，只不过这次是官方给的）。
// 和 D14 §2 的依赖倒置是同一件事：依赖接口，真实现在 main 里注入。
//
// # 规则（TODO(D19)：实现下面四条）
//
//	not-ready    Pod 不在 Running，或 Running 但有容器 Ready=false
//	restarts     任一容器 RestartCount > opts.MaxRestarts
//	no-limits    任一容器没有设置 resources.limits.memory
//	latest-tag   任一容器镜像没有 tag，或 tag 是 latest
//
// 每条规则的 Message 要让【没看代码的人】知道下一步该看什么 ——
// 「容器 api 重启了 17 次」比「restarts」有用，「上次退出原因: OOMKilled」更有用。
package kubeaudit

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Finding 是一条巡检发现。
type Finding struct {
	Namespace string
	Pod       string
	Container string // 规则针对整个 Pod 时为空
	Rule      string // "not-ready" | "restarts" | "no-limits" | "latest-tag"
	Message   string
}

// Options 控制巡检范围和阈值。
type Options struct {
	// Namespace 为空表示所有 namespace（metav1.NamespaceAll 就是 ""）。
	Namespace string
	// MaxRestarts 超过这个值才报。
	MaxRestarts int32
	PageSize    int32
}

const defaultPageSize int32 = 500

// rule 是一条规则。⭐ 用函数类型而不是接口 —— 和 D11 §1.7 http.HandlerFunc 同一个理由：
// 规则没有状态，一个函数就够；要参数（比如阈值）就用闭包捕获。
type rule func(pod *corev1.Pod) []Finding

// Audit 拉取 Pod 列表并对每个 Pod 应用全部规则。
//
// ⚠️ 这是一次性 List，不是 watch —— 巡检 CLI 跑一次就退出，不需要 informer（讲义 §4.5
// 讲了什么时候需要）。List 一次拿全量，大集群可能有几万个 Pod，
// 所以 ListOptions 里用了 Limit 分页 —— TODO(D19)：处理 Continue，把所有页拉完。
func Audit(ctx context.Context, client kubernetes.Interface, opts Options) ([]Finding, error) {
	if opts.PageSize == 0 {
		opts.PageSize = defaultPageSize
	}

	rules := []rule{
		checkReady,
		checkRestarts(opts.MaxRestarts),
		checkLimits,
		checkLatestTag,
	}

	var findings []Finding

	// TODO(D19)：
	//   pods, err := client.CoreV1().Pods(opts.Namespace).List(ctx, metav1.ListOptions{Limit: 500})
	//   处理 err；遍历 pods.Items（⚠️ 注意 range 拷贝 —— Pod 是大 struct，用 &pods.Items[i]）
	//   对每个 pod 跑每条 rule，append 到 findings
	//   pods.Continue != "" 时带上 Continue 再 List，直到为空

	continueList := ""
	for {
		listopts := metav1.ListOptions{Limit: int64(opts.PageSize), Continue: continueList}
		pods, err := client.CoreV1().Pods(opts.Namespace).List(ctx, listopts)
		if err != nil {
			return nil, fmt.Errorf("list pods in namespace %q (continue=%q): %w", opts.Namespace, continueList, err)
		}

		for i := range pods.Items {
			for _, rule := range rules {
				findings = append(findings, rule(&pods.Items[i])...)
			}
		}

		if pods.Continue == "" {
			break
		}
		continueList = pods.Continue
	}

	return findings, nil
}

// checkReady：Pod 不在 Running 阶段，或有容器没 Ready。
//
// 提示：pod.Status.Phase / pod.Status.ContainerStatuses[i].Ready
// Message 里带上 Phase 和（如果有）容器的 State.Waiting.Reason —— 那是排查的第一线索
// （CrashLoopBackOff / ImagePullBackOff / ...）。
//
// ⚠️ Succeeded 的 Pod（跑完的 Job）不算问题。
func checkReady(pod *corev1.Pod) []Finding {
	// TODO(D19)
	var findings []Finding

	// Succeeded 的 Pod（跑完的 Job）不算问题。
	if pod.Status.Phase == corev1.PodSucceeded {
		return findings
	}

	// Pod 不在 Running 阶段 —— 先报一条 Pod 级 Finding（Container 留空）。
	// ⚠️ 必须在容器循环之外：调度不上（Unschedulable）、被驱逐（Evicted）的 Pod
	// 根本没有 ContainerStatuses，挂在循环里就漏报了。
	if pod.Status.Phase != corev1.PodRunning {
		msg := fmt.Sprintf("Pod phase is %s", pod.Status.Phase)
		if pod.Status.Reason != "" {
			msg = fmt.Sprintf("Pod phase is %s, reason: %s", pod.Status.Phase, pod.Status.Reason)
		}
		f := Finding{
			Namespace: pod.Namespace,
			Pod:       pod.Name,
			Rule:      "not-ready",
			Message:   msg,
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
				f.Container = cs.Name
				f.Message = fmt.Sprintf("%s, container %s waiting: %s",
					msg, cs.Name, cs.State.Waiting.Reason)
				break
			}
		}
		findings = append(findings, f)
		return findings
	}

	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			continue
		}
		f := Finding{
			Namespace: pod.Namespace,
			Pod:       pod.Name,
			Container: cs.Name,
			Rule:      "not-ready",
			Message:   fmt.Sprintf("container %s is not ready", cs.Name),
		}
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			f.Message = fmt.Sprintf("container %s not ready, waiting: %s",
				cs.Name, cs.State.Waiting.Reason)
		}
		findings = append(findings, f)
	}
	return findings
}

// checkRestarts 返回一条规则：任一容器重启超过 maxRestarts 次就报。
//
// 提示：pod.Status.ContainerStatuses[i].RestartCount；
// LastTerminationState.Terminated 不为 nil 时把 Reason 放进 Message（OOMKilled / Error）。
func checkRestarts(maxRestarts int32) rule {
	return func(pod *corev1.Pod) []Finding {
		// TODO(D19)
		var findings []Finding
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > maxRestarts {
				f := Finding{
					Namespace: pod.Namespace,
					Pod:       pod.Name,
					Container: cs.Name,
					Rule:      "restarts",
					Message:   fmt.Sprintf("container %s restarts %d times", cs.Name, cs.RestartCount),
				}
				if cs.LastTerminationState.Terminated != nil {
					f.Message = fmt.Sprintf("container %s restarts %d times, terminated reason: %s",
						cs.Name, cs.RestartCount, cs.LastTerminationState.Terminated.Reason)
				}
				findings = append(findings, f)
			}
		}
		return findings
	}
}

// checkLimits：没有 memory limit 的容器。
//
// 提示：pod.Spec.Containers[i].Resources.Limits 是 map，
// 用 .Memory() 或 [corev1.ResourceMemory]；零值 Quantity 的 IsZero() 为 true。
func checkLimits(pod *corev1.Pod) []Finding {
	// TODO(D19)
	var findings []Finding
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		mem := c.Resources.Limits[corev1.ResourceMemory]
		if mem.IsZero() {
			findings = append(findings, Finding{
				Namespace: pod.Namespace,
				Pod:       pod.Name,
				Container: c.Name,
				Rule:      "no-limits",
				Message:   fmt.Sprintf("container %s has no memory limit", c.Name),
			})
		}
	}
	return findings
}

// checkLatestTag：镜像没写 tag 或 tag 是 latest。
//
// ⚠️ 镜像引用不只是 name:tag —— 还有 registry:port/name@sha256:... 这种。
// 别用 strings.Split(image, ":")。想一想哪些情况会误判，写进测试。
func checkLatestTag(pod *corev1.Pod) []Finding {
	// TODO(D19)
	var findings []Finding
	for i := range pod.Spec.Containers {
		c := &pod.Spec.Containers[i]
		if needsTag(c.Image) {
			findings = append(findings, Finding{
				Namespace: pod.Namespace,
				Pod:       pod.Name,
				Container: c.Name,
				Rule:      "latest-tag",
				Message:   fmt.Sprintf("container %s uses image %s without a pinned tag", c.Name, c.Image),
			})
		}
	}
	return findings
}

func needsTag(image string) bool {
	// ① digest 引用权威，不需要 tag
	if strings.Contains(image, "@") {
		return false
	}
	// ② tag 只可能在「最后一个 / 之后」
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if lastColon > lastSlash {
		// 存在 tag 分隔符 —— 看 tag 本身
		tag := image[lastColon+1:]
		return tag == "latest"
	}
	// ③ 没有 tag 分隔符 → 无 tag
	return true
}
