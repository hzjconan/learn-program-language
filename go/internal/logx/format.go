package logx

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"text/tabwriter"
	"time"
)

// Formatter 把 Report 写成某种格式。
//
// 一个方法的小接口（D5 §2）。注意它写到 io.Writer 而不是返回 string ——
// 这样调用方可以直接写文件、写 HTTP 响应、写 gzip 流，不用先在内存里拼出全文。
//
// **logx 包里不允许出现任何 fmt.Print / os.Stdout**：输出去哪儿由调用方决定。
type Formatter interface {
	// Format 把 r 写进 w。
	Format(w io.Writer, r *Report) error
}

// TextFormatter 输出给人看的文本报告。
//
// TODO(D7)：设计这个 struct 的字段。至少要能配置「最慢的几条」。
type TextFormatter struct {
	// Top 是「最慢的 N 条」里的 N。<= 0 表示不输出这一段。
	Top int
}

// Format 实现 Formatter。
//
// TODO(D7)：实现我。
//
// 具体排版**由你决定**，但必须包含这些事实（测试会检查关键字，不检查排版）：
//   - 总条数、失败条数
//   - 每个级别的计数
//   - 每个服务的计数
//   - p50 / p95 / p99 耗时
//   - Top 条最慢的记录（Top > 0 时）
//
// 提示：对齐用 text/tabwriter，比手写 %-20s 省心。
// map 遍历顺序是随机的（D3），输出前**必须排序**，否则每次跑结果都不一样。
func (f TextFormatter) Format(w io.Writer, r *Report) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintf(tw, "总条数\t%d\n", r.Total)        //nolint:errcheck // 写的是内存缓冲，不会失败
	fmt.Fprintf(tw, "失败条数\t%d\n", len(r.Failed)) //nolint:errcheck // 写的是内存缓冲，不会失败
	fmt.Fprintln(tw)                             //nolint:errcheck // 写的是内存缓冲，不会失败

	// 按级别（排序后输出）
	fmt.Fprintln(tw, "级别\t计数") //nolint:errcheck // 写的是内存缓冲，不会失败

	levels := make([]Level, 0, len(r.ByLevel))
	for lv := range r.ByLevel {
		levels = append(levels, lv)
	}
	slices.Sort(levels)
	for _, lv := range levels {
		fmt.Fprintf(tw, "%s\t%d\n", lv.String(), r.ByLevel[lv]) //nolint:errcheck // 写的是内存缓冲，不会失败
	}
	fmt.Fprintln(tw) //nolint:errcheck // 写的是内存缓冲，不会失败

	// 按服务（排序后输出）
	fmt.Fprintln(tw, "服务\t计数") //nolint:errcheck // 写的是内存缓冲，不会失败
	services := make([]string, 0, len(r.ByService))
	for s := range r.ByService {
		services = append(services, s)
	}
	sort.Strings(services)
	for _, s := range services {
		fmt.Fprintf(tw, "%s\t%d\n", s, r.ByService[s]) //nolint:errcheck // 写的是内存缓冲，不会失败
	}
	fmt.Fprintln(tw) //nolint:errcheck // 写的是内存缓冲，不会失败

	// 分位数
	fmt.Fprintln(tw, "分位数\t耗时")                      //nolint:errcheck // 写的是内存缓冲，不会失败
	fmt.Fprintf(tw, "p50\t%v\n", r.Percentile(0.5))  //nolint:errcheck // 写的是内存缓冲，不会失败
	fmt.Fprintf(tw, "p95\t%v\n", r.Percentile(0.95)) //nolint:errcheck // 写的是内存缓冲，不会失败
	fmt.Fprintf(tw, "p99\t%v\n", r.Percentile(0.99)) //nolint:errcheck // 写的是内存缓冲，不会失败
	fmt.Fprintln(tw)                                 //nolint:errcheck // 写的是内存缓冲，不会失败

	// 最慢条目
	if f.Top > 0 {
		slowest := r.TopSlowest(f.Top)
		if len(slowest) > 0 {
			fmt.Fprintf(tw, "最慢的 %d 条\n", f.Top)   //nolint:errcheck // 写的是内存缓冲，不会失败
			fmt.Fprintln(tw, "时间\t级别\t服务\t耗时\t消息") //nolint:errcheck // 写的是内存缓冲，不会失败
			for _, e := range slowest {
				//nolint:errcheck // 写的是内存缓冲，不会失败
				fmt.Fprintf(tw, "%s\t%s\t%s\t%v\t%s\n",
					e.Time.Format(time.RFC3339),
					e.Level,
					e.Service,
					e.Latency,
					e.Message,
				)
			}
		}
	}

	return tw.Flush()
}

// JSONFormatter 输出给机器读的 JSON。
//
// TODO(D7)：设计字段。
type JSONFormatter struct {
	// Top 同 TextFormatter。
	Top int
	// Indent 为 true 时输出缩进过的 JSON。
	Indent bool
}

// Format 实现 Formatter。
//
// TODO(D7)：实现我。
//
// JSON 结构固定为下面这样（测试会按字段名检查）：
//
//	{
//	  "total": 6,
//	  "failed": 1,
//	  "by_level": {"INFO": 3, "ERROR": 2},
//	  "by_service": {"api-gateway": 4, "db": 2},
//	  "latency_ms": {"p50": 15, "p95": 200, "p99": 200},
//	  "slowest": [
//	    {"time": "2026-08-18T10:23:45Z", "level": "ERROR", "service": "db",
//	     "latency_ms": 200, "message": "connection refused"}
//	  ]
//	}
//
// 几个要点：
//   - 级别和服务名做 map 的 key，值是计数
//   - 耗时统一用**毫秒整数**，不要输出 "15ms" 这种字符串 —— 机器读的格式不该带单位
//   - 时间用 RFC3339
//   - Top <= 0 时 "slowest" 输出空数组 []，**不是 null**（想想为什么，review 会问）
//
// 提示：别直接给 Report 加 json tag —— 那会把内部表示焊死在输出格式上。
// 定义一个专门用于序列化的未导出 struct，这是 Go 里处理「领域模型 vs 传输格式」的常规做法。
func (f JSONFormatter) Format(w io.Writer, r *Report) error {
	// 用专门的 struct 做序列化，把领域模型和传输格式解耦
	type slowestEntry struct {
		Time      string `json:"time"`
		Level     string `json:"level"`
		Service   string `json:"service"`
		LatencyMs int    `json:"latency_ms"`
		Message   string `json:"message"`
	}

	byLevel := make(map[string]int, len(r.ByLevel))
	for lv, n := range r.ByLevel {
		byLevel[lv.String()] = n
	}

	byService := make(map[string]int, len(r.ByService))
	for s, n := range r.ByService {
		byService[s] = n
	}

	var slowest []slowestEntry
	if f.Top > 0 {
		entries := r.TopSlowest(f.Top)
		slowest = make([]slowestEntry, 0, len(entries))
		for _, e := range entries {
			slowest = append(slowest, slowestEntry{
				Time:      e.Time.Format(time.RFC3339),
				Level:     e.Level.String(),
				Service:   e.Service,
				LatencyMs: int(e.Latency / time.Millisecond),
				Message:   e.Message,
			})
		}
	}
	// Top <= 0 时输出空数组 [] 而不是 null
	if slowest == nil {
		slowest = []slowestEntry{}
	}

	out := struct {
		Total     int            `json:"total"`
		Failed    int            `json:"failed"`
		ByLevel   map[string]int `json:"by_level"`
		ByService map[string]int `json:"by_service"`
		LatencyMs struct {
			P50 int `json:"p50"`
			P95 int `json:"p95"`
			P99 int `json:"p99"`
		} `json:"latency_ms"`
		Slowest []slowestEntry `json:"slowest"`
	}{
		Total:     r.Total,
		Failed:    len(r.Failed),
		ByLevel:   byLevel,
		ByService: byService,
		Slowest:   slowest,
	}
	out.LatencyMs.P50 = int(r.Percentile(0.5) / time.Millisecond)
	out.LatencyMs.P95 = int(r.Percentile(0.95) / time.Millisecond)
	out.LatencyMs.P99 = int(r.Percentile(0.99) / time.Millisecond)

	enc := json.NewEncoder(w)
	if f.Indent {
		enc.SetIndent("", "  ")
	}
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

// 编译期断言：两个 formatter 都必须满足接口。
//
// 注意用的是值而不是指针 —— 说明它们的方法都该是值接收者（D4 §3）。
var (
	_ Formatter = TextFormatter{}
	_ Formatter = JSONFormatter{}
)
