package logx

import (
	"bufio"
	"cmp"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"time"
)

// Report 是一批日志的聚合结果。
//
// # 字段为什么导出
//
// 这四个字段要被 JSONFormatter 直接序列化，所以导出。但**耗时数据怎么存是你的决定**
// —— 用未导出字段，随便你存成 []time.Duration、堆、还是别的。
// 这就是「导出的是契约，未导出的是实现」。
// 注意： Report不是并发安全的，应该在单goroutine中使用
type Report struct {
	// Total 是成功解析的条数。
	Total int
	// Failed 是解析失败的行，每个元素都是 *ParseError。
	//
	// 用 []error 而不是 []*ParseError：调用方通常只想打印它们，
	// 需要细节时再用 errors.As 取（D5 §5）。
	Failed []error
	// ByLevel 是按级别的计数。
	ByLevel map[Level]int
	// ByService 是按服务名的计数。
	ByService map[string]int

	// 下面存耗时和最慢条目的部分由你设计，用未导出字段。
	entries []Entry
}

// NewReport 返回一个可用的空 Report。
//
// TODO(D7)：实现我。
//
// 想一想：能不能做到零值可用（D4 §1），从而不需要这个构造函数？
// 如果能，为什么我还是给了你一个 New？review 时我会问。
func NewReport() *Report {
	return &Report{
		ByLevel:   make(map[Level]int),
		ByService: make(map[string]int),
	}
}

// Add 把一条日志计入统计。
//
// TODO(D7)：实现我。
func (r *Report) Add(e Entry) {
	// 提问，这里要不要考虑并发Add的场景？你的要求里似乎没有提到。
	r.Total++
	r.ByLevel[e.Level]++
	r.ByService[e.Service]++
	r.entries = append(r.entries, e)
}

// AddFailure 记录一行解析失败。
//
// TODO(D7)：实现我。
func (r *Report) AddFailure(err error) {
	r.Failed = append(r.Failed, err)
}

// Percentile 返回耗时的 p 分位数，p 取值 [0, 1]。
//
// TODO(D7)：实现我。
//
// 用**最近秩法（nearest-rank）**，定义如下（照着写就行，重点不是算法）：
//
//	升序排好的 n 个样本，idx = ceil(p*n) - 1，然后把 idx 夹到 [0, n-1]
//
// 边界约定：
//   - 没有任何样本 → 返回 0
//   - p <= 0 → 最小值；p >= 1 → 最大值
//
// ⚠️ 性能提示：如果你每次调用都重新排序，跑 p50/p95/p99 就排了三次。
// review 时我会看你怎么处理 —— 但**别过早优化**，先写对，用 benchmark 说话（D6 §6）。
func (r *Report) Percentile(p float64) time.Duration {
	n := len(r.entries)
	if n == 0 {
		return 0
	}

	// 避免直接改到原始的latencies切片数据
	l := make([]time.Duration, n)
	for i, v := range r.entries {
		l[i] = v.Latency
	}
	// 排序（升序）
	slices.Sort(l)

	idx := max(int(math.Ceil(p*float64(n)))-1, 0)
	if idx >= n {
		idx = n - 1
	}
	return l[idx]
}

// TopSlowest 返回最慢的 n 条，按耗时**降序**。
//
// TODO(D7)：实现我。
//
// 耗时相同时按 Time **升序**排 —— 这是为了让结果确定，否则测试会随机失败（D6）。
// n <= 0 时返回 nil；样本不足 n 条时有多少返回多少。
//
// ⚠️ 返回的切片不能让调用方改到你的内部数据（D3 别名、D6 review 里 store.Tags 那条）。
func (r *Report) TopSlowest(n int) []Entry {
	if n <= 0 {
		return nil
	}

	elen := len(r.entries)
	e := make([]Entry, elen)
	copy(e, r.entries)
	slices.SortFunc(e, func(a, b Entry) int {
		if c := cmp.Compare(b.Latency, a.Latency); c != 0 { // 降序
			return c
		}
		return a.Time.Compare(b.Time) // 升序
	})
	return e[:min(n, elen)]
}

// TimeRange 返回最早和最晚的日志时间。没有样本时返回两个零值。
//
// TODO(D7)：实现我。
func (r *Report) TimeRange() (first, last time.Time) {
	earliest := time.Time{}
	latest := time.Time{}
	for _, e := range r.entries {
		if earliest.IsZero() || e.Time.Before(earliest) {
			earliest = e.Time
		}
		if latest.IsZero() || e.Time.After(latest) {
			latest = e.Time
		}
	}
	return earliest, latest
}

// Analyze 从 r 逐行读取日志并聚合。
//
// TODO(D7)：实现我。
//
// # 为什么参数是 io.Reader 而不是文件名
//
// 小接口哲学（D5 §2）：这样测试可以传 strings.Reader，生产可以传 *os.File，
// 将来还能传 gzip.Reader、网络连接。函数体里不该出现任何 os 包的东西。
//
// 行为要求：
//   - **单行解析失败不中断**，记进 Report.Failed 继续下一行
//   - 空行和只含空白的行**跳过**，既不计入 Total 也不算失败
//   - 只有读取本身出错（scanner.Err()）才返回 error
//   - 行号从 1 开始，**空行也占行号**（否则错误消息里的行号对不上文件）
//
// 提示：bufio.Scanner。注意它默认的单行上限是 64KB，超长行会报错 ——
// 这个错误要不要处理、怎么处理，你决定，review 时说说理由。
func Analyze(r io.Reader) (*Report, error) {
	rpt := NewReport()

	scanner := bufio.NewScanner(r)
	scanner.Split(bufio.ScanLines)
	lineNo := 0
	for scanner.Scan() {
		lineNo++

		if strings.TrimSpace(scanner.Text()) == "" {
			continue // 空行
		}

		entry, err := ParseLine(lineNo, scanner.Text())
		if err != nil {
			rpt.AddFailure(err)
			continue
		}
		rpt.Add(entry)
	}

	if se := scanner.Err(); se != nil {
		return nil, fmt.Errorf("读取日志到第 %d 行时失败: %w", lineNo, se)
	}

	return rpt, nil
}

// 编译期断言：确保 Report 能被 Formatter 用（format.go 里定义）。
var _ = func(f Formatter, w io.Writer, r *Report) error { return f.Format(w, r) }
