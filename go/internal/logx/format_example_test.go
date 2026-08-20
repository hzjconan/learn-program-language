package logx

import (
	"os"
	"strings"
)

func ExampleTextFormatter_Format() {
	r, _ := Analyze(strings.NewReader(strings.Join([]string{
		"2026-08-18T10:00:00Z INFO api-gateway 15 GET /users/42 -> 200",
		"2026-08-18T10:00:01Z INFO api-gateway 23 GET /orders -> 200",
		"2026-08-18T10:00:02Z WARN api-gateway 150 slow query",
		"2026-08-18T10:00:03Z ERROR db 200 connection refused",
		"2026-08-18T10:00:04Z INFO db 5 reconnected",
		"坏行",
	}, "\n")))
	TextFormatter{Top: 2}.Format(os.Stdout, r)
	// Output:
	// 总条数   5
	// 失败条数  1
	//
	// 级别     计数
	// INFO   3
	// WARN   1
	// ERROR  1
	//
	// 服务           计数
	// api-gateway  3
	// db           2
	//
	// 分位数  耗时
	// p50  23ms
	// p95  200ms
	// p99  200ms
	//
	// 最慢的 2 条
	// 时间                    级别     服务           耗时     消息
	// 2026-08-18T10:00:03Z  ERROR  db           200ms  connection refused
	// 2026-08-18T10:00:02Z  WARN   api-gateway  150ms  slow query
}
