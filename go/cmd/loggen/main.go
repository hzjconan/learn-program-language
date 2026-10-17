// Command loggen 生成 D17 练习 ③（GC 实验）用的大日志，格式和 internal/logx 一致。
//
//	go run ./cmd/loggen -n 1000000 > profile/d17/big.log    # 约 61MB
//
// 同样的 -n 永远生成同样的内容，前后两次实验的输入完全一致。
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	n := flag.Int("n", 1_000_000, "生成多少行")
	flag.Parse()

	w := bufio.NewWriter(os.Stdout)
	svcs := []string{"api-gateway", "orders", "payments", "users", "search"}
	lvs := []string{"INFO", "INFO", "INFO", "WARN", "ERROR"}
	t0 := time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC)
	for i := range *n {
		if _, err := fmt.Fprintf(w, "%s %s %s %d GET /items/%d -> 200\n",
			t0.Add(time.Duration(i)*time.Second).Format(time.RFC3339),
			lvs[i%len(lvs)], svcs[(i/3)%len(svcs)], (i*7919)%2000, i); err != nil {
			fmt.Fprintln(os.Stderr, "loggen: write stdout:", err)
			os.Exit(1)
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, "loggen: flush stdout:", err)
		os.Exit(1)
	}
}
