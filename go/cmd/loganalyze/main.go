// Command loganalyze 读取日志文件（或 stdin），输出聚合统计。
//
//	go run ./cmd/loganalyze -format=text internal/logx/testdata/sample.log
//	go run ./cmd/loganalyze -format=json -top=5 internal/logx/testdata/sample.log
//	cat access.log | go run ./cmd/loganalyze
//
// # 这个文件要多薄
//
// D6 §11：main 包只做三件事 —— 解析参数、装配依赖、把错误报给用户。
// **一行业务逻辑都不该写在这里**，全部在 internal/logx 里，那样才测得了。
//
// 判断标准：如果你想给 main 里的某段代码写测试，那段代码就该搬进 logx。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hzjconan/learn-program-language/go/internal/logx"
)

// run 是真正的入口。
//
// TODO(D7)：实现我。
//
// 把逻辑放在 run 而不是 main，是为了能 return error ——
// main 里只能 os.Exit，没法用 defer，也没法被测试调用。这是 Go 里很常见的一个小模式。
//
// 要做的事：
//  1. 解析 flag：-format（text|json，默认 text）、-top（默认 5）、-indent（json 用）
//  2. 参数里有文件名就打开它，**没有就读 os.Stdin**
//  3. defer 关文件（读 stdin 时别关！关掉标准输入是个经典 bug）
//  4. 调 logx.Analyze
//  5. 按 -format 选一个 logx.Formatter，写到 os.Stdout
//  6. 有解析失败的行时，把它们写到 **os.Stderr**（不是 stdout —— 否则会污染 JSON 输出，
//     下游 `| jq` 就废了）
//
// 想一想：-format 不认识的值该怎么办？静默用默认值，还是报错退出？review 时我会问。
func run() error {
	format := flag.String("format", "text", "输出格式：text 或 json")
	top := flag.Int("top", 5, "最慢的 N 条记录")
	indent := flag.Bool("indent", false, "JSON 缩进输出")
	flag.Parse()

	// 1. 选输入源：有文件名就打开，没有就读 stdin
	var input *os.File
	if args := flag.Args(); len(args) > 0 {
		f, err := os.Open(args[0])
		if err != nil {
			return fmt.Errorf("打开文件: %w", err)
		}
		defer func() { _ = f.Close() }() //nolint:errcheck // 只读文件，关闭错误无需处理
		input = f
	} else {
		input = os.Stdin // 读 stdin 时不能关！
	}

	// 2. 解析日志
	report, err := logx.Analyze(input)
	if err != nil {
		return fmt.Errorf("分析日志: %w", err)
	}

	// 3. 解析失败的行写到 stderr（不污染 stdout）
	for _, e := range report.Failed {
		fmt.Fprintln(os.Stderr, e)
	}

	// 4. 按 -format 选 formatter
	var formatter logx.Formatter
	switch *format {
	case "text":
		formatter = logx.TextFormatter{Top: *top}
	case "json":
		formatter = logx.JSONFormatter{Top: *top, Indent: *indent}
	default:
		return fmt.Errorf("不认识的 -format %q（可选：text、json）", *format)
	}

	// 5. 输出到 stdout
	return formatter.Format(os.Stdout, report)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "loganalyze:", err)
		os.Exit(1)
	}
}
