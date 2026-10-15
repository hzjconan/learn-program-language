package controller

// D16 练习 ②③：operator 的自定义指标。讲义见 lessons/D16.md §4。
//
// controller-runtime 已经替你暴露了一批指标（reconcile 次数/耗时/错误、workqueue 深度……，§4.1）。
// 它们回答的是「controller 忙不忙、快不快、错没错」——但回答不了
// 「reconcile 每一趟到底【写】了什么」。D20 的幂等纪律，要靠下面这几个指标在线上验证。

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// TODO(D16) ②：定义并注册两个 counter。
//
//	golearn_webapp_child_ops_total{kind, op}
//	    kind = "Deployment" | "Service"
//	    op   = controllerutil.OperationResult 的字符串："created" | "updated" | "unchanged"
//	    在 Reconcile 里两次 CreateOrUpdate 之后各 Inc 一次。
//
//	golearn_webapp_status_updates_total
//	    每调用一次 r.Status().Update 就 Inc。
//
//	⭐ 注册到 controller-runtime 的 registry，不是 prometheus.DefaultRegisterer：
//	    metrics.Registry.MustRegister(...)     // "sigs.k8s.io/controller-runtime/pkg/metrics"
//	   注册到 Default 的话 /metrics 上看不到 —— Manager 的 metrics server 只 serve 它自己的 registry。
//	⭐ 在 init() 里注册（kubebuilder 官方文档的做法）。MustRegister 重复注册会 panic —— 所以只能一次。
//	⚠️ 名字要带 _total 后缀、带自己的前缀（golearn_）—— §3.3 的命名约定。

var (
	// childOpsTotal 子资源的 CreateOrUpdate 结果计数，只记成功完成的（失败由 reconcile_total{result="error"} 记）。
	// label kind: "Deployment" | "Service"
	// label op: string(op)，controllerutil.OperationResult 是 string 类型 → "created" | "updated" | "unchanged"
	childOpsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "golearn_webapp_child_ops_total",
			Help: "Successful CreateOrUpdate calls on WebApp child resources, by kind and result. " +
				"op=\"updated\" means the operator sent an Update; the API server may still treat it as a no-op.",
		},
		[]string{"kind", "op"},
	)

	// statusUpdatesTotal 成功的 Status().Update 调用次数，包括 API server 当作 no-op 的。
	statusUpdatesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "golearn_webapp_status_updates_total",
			Help: "Successful Status().Update calls on WebApp, including ones the API server treats as no-op.",
		},
	)

	readyReplicas = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "golearn_webapp_ready_replicas",
			Help: "Ready replicas of the Deployment managed by a WebApp, as observed at the last reconcile.",
		},
		[]string{"namespace", "name"},
	)
)

func init() {
	metrics.Registry.MustRegister(childOpsTotal)
	metrics.Registry.MustRegister(statusUpdatesTotal)
	metrics.Registry.MustRegister(readyReplicas)
}

// TODO(D16) ③（做完 ② 再做）：
//
//	golearn_webapp_ready_replicas{namespace, name}   gauge
//	    reconcile 末尾 Set(ready)。
//	    ⚠️ WebApp 被删之后这条时间序列怎么办？（§3.4「僵尸序列」）—— 在 NotFound 那条路径上处理。
//	    ⚠️ 为什么这里用 name 做 label 可以，而 golearn-api 的 HTTP 指标用 path 做 label 要小心？（§3.4 基数）
