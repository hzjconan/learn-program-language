// kubeaudit 巡检集群里的 Pod（D19 练习 ①）。
//
//	go run ./cmd/kubeaudit                       # 当前 kubeconfig 上下文，所有 namespace
//	go run ./cmd/kubeaudit -n kube-system        # 只看一个 namespace
//	go run ./cmd/kubeaudit -max-restarts 0       # 重启过就报
//
// 退出码：0 没问题；1 有发现；2 出错。⭐ 让它能直接接进 CI 或 cron。
//
// 本文件是【接线】：读 kubeconfig → 建 clientset → 调 kubeaudit.Audit → 打印。
// 已经写好，读懂即可。规则逻辑在 internal/kubeaudit，那才是练习。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/hzjconan/learn-program-language/go/internal/kubeaudit"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		namespace   = flag.String("n", "", "只看这个 namespace（默认全部）")
		maxRestarts = flag.Int("max-restarts", 3, "容器重启超过这个次数才报")
		timeout     = flag.Duration("timeout", 30*time.Second, "整个巡检的超时")
		ps          = flag.Int("page-size", 500, "分页大小，默认500")
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载集群配置: %v\n", err)
		return 2
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建 clientset: %v\n", err)
		return 2
	}

	findings, err := kubeaudit.Audit(ctx, client, kubeaudit.Options{
		Namespace:   *namespace,
		MaxRestarts: int32(*maxRestarts), //nolint:gosec // 命令行参数，范围可控
		PageSize:    int32(*ps),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "巡检失败: %v\n", err)
		return 2
	}

	if len(findings) == 0 {
		fmt.Println("没有发现问题")
		return 0
	}

	// tabwriter：对齐的表格输出，标准库自带
	// 写进 tabwriter 的缓冲不会失败，真正写 stdout 是在 Flush —— 只检查那一次
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tPOD\tCONTAINER\tRULE\tMESSAGE") //nolint:errcheck
	for _, f := range findings {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", f.Namespace, f.Pod, f.Container, f.Rule, f.Message) //nolint:errcheck
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "输出失败: %v\n", err)
		return 2
	}
	fmt.Fprintf(os.Stderr, "\n共 %d 条\n", len(findings))
	return 1
}

// loadConfig 的查找顺序（这是 kubectl 和所有 client-go 工具的惯例，D19 §4.1）：
//
//  1. 在 Pod 里跑 → in-cluster（ServiceAccount token 自动挂在 /var/run/secrets/...）
//  2. 否则 → kubeconfig（$KUBECONFIG 或 ~/.kube/config，用它的 current-context）
//
// ⭐ 同一个二进制，本机开发和部署进集群都能跑，不用改代码。
func loadConfig() (*rest.Config, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, nil
	}
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
}
