package logx

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// entryAt 造一条测试用日志：第 i 秒、耗时 ms 毫秒。
func entryAt(t *testing.T, sec int, ms int, level Level, service string) Entry {
	t.Helper()
	return Entry{
		Time:    mustTime(t, "2026-08-18T10:00:00Z").Add(time.Duration(sec) * time.Second),
		Level:   level,
		Service: service,
		Latency: time.Duration(ms) * time.Millisecond,
		Message: "msg",
	}
}

// newReportWith 造一个装了指定耗时（毫秒）的 Report。
func newReportWith(t *testing.T, msList ...int) *Report {
	t.Helper()
	r := NewReport()
	if r == nil {
		t.Fatal("NewReport() 返回了 nil")
	}
	for i, ms := range msList {
		r.Add(entryAt(t, i, ms, LevelInfo, "svc"))
	}
	return r
}

// ---------- 基本聚合 ----------

func TestReport_Add(t *testing.T) {
	r := NewReport()
	if r == nil {
		t.Fatal("NewReport() 返回了 nil")
	}

	r.Add(entryAt(t, 0, 10, LevelInfo, "api"))
	r.Add(entryAt(t, 1, 20, LevelInfo, "api"))
	r.Add(entryAt(t, 2, 30, LevelError, "db"))

	if r.Total != 3 {
		t.Errorf("Total = %d, want 3", r.Total)
	}
	if got := r.ByLevel[LevelInfo]; got != 2 {
		t.Errorf("ByLevel[INFO] = %d, want 2", got)
	}
	if got := r.ByLevel[LevelError]; got != 1 {
		t.Errorf("ByLevel[ERROR] = %d, want 1", got)
	}
	if got := r.ByLevel[LevelWarn]; got != 0 {
		t.Errorf("ByLevel[WARN] = %d, want 0（没出现过的级别不该有计数）", got)
	}
	if got := r.ByService["api"]; got != 2 {
		t.Errorf("ByService[api] = %d, want 2", got)
	}
	if got := r.ByService["db"]; got != 1 {
		t.Errorf("ByService[db] = %d, want 1", got)
	}
}

func TestReport_AddFailure(t *testing.T) {
	r := NewReport()
	r.AddFailure(&ParseError{Line: 3, Field: "level", Err: errors.New("x")})
	r.AddFailure(&ParseError{Line: 9, Field: "time", Err: errors.New("y")})

	if len(r.Failed) != 2 {
		t.Fatalf("len(Failed) = %d, want 2", len(r.Failed))
	}
	if r.Total != 0 {
		t.Errorf("Total = %d, want 0（失败的行不该计入 Total）", r.Total)
	}

	// Failed 里的元素要能被 errors.As 还原成 *ParseError
	var perr *ParseError
	if !errors.As(r.Failed[0], &perr) || perr.Line != 3 {
		t.Errorf("Failed[0] 拿不到行号，得到 %v", r.Failed[0])
	}
}

// ---------- 分位数 ----------

func TestReport_Percentile(t *testing.T) {
	// 7 个样本，升序是 1,2,5,15,23,150,200
	r := newReportWith(t, 15, 23, 150, 200, 5, 1, 2)

	tests := []struct {
		name string
		p    float64
		want time.Duration
	}{
		{name: "p50", p: 0.50, want: 15 * time.Millisecond},
		{name: "p95", p: 0.95, want: 200 * time.Millisecond},
		{name: "p99", p: 0.99, want: 200 * time.Millisecond},
		{name: "p0 取最小", p: 0, want: 1 * time.Millisecond},
		{name: "p1 取最大", p: 1, want: 200 * time.Millisecond},
		{name: "p 小于 0 也取最小", p: -0.5, want: 1 * time.Millisecond},
		{name: "p 大于 1 也取最大", p: 1.5, want: 200 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Percentile(tt.p); got != tt.want {
				t.Errorf("Percentile(%v) = %v, want %v", tt.p, got, tt.want)
			}
		})
	}
}

func TestReport_PercentileEmpty(t *testing.T) {
	r := NewReport()
	if got := r.Percentile(0.95); got != 0 {
		t.Errorf("空 Report 的 Percentile(0.95) = %v, want 0", got)
	}
}

// TestReport_PercentileStable 验证多次调用结果一致 —— 如果实现里就地排序了内部切片，
// 又恰好在别处依赖原顺序，就会出问题。
func TestReport_PercentileStable(t *testing.T) {
	r := newReportWith(t, 15, 23, 150, 200, 5, 1, 2)
	first := r.Percentile(0.95)
	_ = r.Percentile(0.5)
	if second := r.Percentile(0.95); second != first {
		t.Errorf("重复调用结果不一致：%v → %v", first, second)
	}
}

// ---------- 最慢的 N 条 ----------

func TestReport_TopSlowest(t *testing.T) {
	r := NewReport()
	r.Add(entryAt(t, 0, 10, LevelInfo, "a"))
	r.Add(entryAt(t, 1, 200, LevelError, "b"))
	r.Add(entryAt(t, 2, 150, LevelWarn, "c"))
	r.Add(entryAt(t, 3, 5, LevelDebug, "d"))

	got := r.TopSlowest(2)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Latency != 200*time.Millisecond {
		t.Errorf("最慢的 Latency = %v, want 200ms", got[0].Latency)
	}
	if got[1].Latency != 150*time.Millisecond {
		t.Errorf("第二慢的 Latency = %v, want 150ms", got[1].Latency)
	}
}

// TestReport_TopSlowestTieBreak 验证耗时相同时按时间升序 —— 否则结果不确定，测试会随机失败。
func TestReport_TopSlowestTieBreak(t *testing.T) {
	r := NewReport()
	r.Add(entryAt(t, 5, 100, LevelInfo, "晚的"))
	r.Add(entryAt(t, 1, 100, LevelInfo, "早的"))
	r.Add(entryAt(t, 3, 100, LevelInfo, "中间的"))

	got := r.TopSlowest(3)
	want := []string{"早的", "中间的", "晚的"}
	for i, w := range want {
		if got[i].Service != w {
			t.Errorf("第 %d 名是 %q, want %q（耗时相同要按时间升序）", i, got[i].Service, w)
		}
	}
}

func TestReport_TopSlowestEdges(t *testing.T) {
	r := newReportWith(t, 10, 20, 30)

	if got := r.TopSlowest(0); got != nil {
		t.Errorf("TopSlowest(0) = %v, want nil", got)
	}
	if got := r.TopSlowest(-1); got != nil {
		t.Errorf("TopSlowest(-1) = %v, want nil", got)
	}
	if got := r.TopSlowest(99); len(got) != 3 {
		t.Errorf("TopSlowest(99) 返回 %d 条, want 3（不够就有多少给多少）", len(got))
	}
	if got := NewReport().TopSlowest(5); len(got) != 0 {
		t.Errorf("空 Report 的 TopSlowest(5) 返回了 %d 条", len(got))
	}
}

// TestReport_TopSlowestNoAliasing 验证调用方改返回值不会污染 Report 内部数据。
// 这是 D6 review 里 store.Tags 那条的同类问题（D3 别名 + D4 浅拷贝）。
func TestReport_TopSlowestNoAliasing(t *testing.T) {
	r := newReportWith(t, 10, 20, 30)

	got := r.TopSlowest(3)
	got[0].Service = "被外部改掉了"
	got[0].Latency = 999 * time.Second

	again := r.TopSlowest(3)
	if again[0].Service == "被外部改掉了" || again[0].Latency == 999*time.Second {
		t.Error("调用方改了返回的切片，影响到了 Report 内部数据")
	}
}

// ---------- 时间范围 ----------

func TestReport_TimeRange(t *testing.T) {
	r := NewReport()
	r.Add(entryAt(t, 5, 1, LevelInfo, "a"))
	r.Add(entryAt(t, 1, 1, LevelInfo, "b")) // 故意乱序加入
	r.Add(entryAt(t, 9, 1, LevelInfo, "c"))

	first, last := r.TimeRange()
	if !first.Equal(mustTime(t, "2026-08-18T10:00:01Z")) {
		t.Errorf("first = %v, want 10:00:01Z", first)
	}
	if !last.Equal(mustTime(t, "2026-08-18T10:00:09Z")) {
		t.Errorf("last = %v, want 10:00:09Z", last)
	}
}

func TestReport_TimeRangeEmpty(t *testing.T) {
	first, last := NewReport().TimeRange()
	if !first.IsZero() || !last.IsZero() {
		t.Errorf("空 Report 的 TimeRange = (%v, %v), want 两个零值", first, last)
	}
}

// ---------- Analyze ----------

func TestReport_AnalyzeSampleFile(t *testing.T) {
	f, err := os.Open("testdata/sample.log")
	if err != nil {
		t.Fatalf("打不开测试数据: %v", err)
	}
	defer f.Close() //nolint:errcheck // 测试里读文件，Close 出错无所谓

	r, err := Analyze(f)
	if err != nil {
		t.Fatalf("Analyze 失败: %v", err)
	}

	if r.Total != 7 {
		t.Errorf("Total = %d, want 7（11 行里：3 行解析失败、1 行空行）", r.Total)
	}
	if len(r.Failed) != 3 {
		t.Errorf("len(Failed) = %d, want 3", len(r.Failed))
	}

	wantLevel := map[Level]int{LevelDebug: 1, LevelInfo: 4, LevelWarn: 1, LevelError: 1}
	for lv, want := range wantLevel {
		if got := r.ByLevel[lv]; got != want {
			t.Errorf("ByLevel[%v] = %d, want %d", lv, got, want)
		}
	}

	wantSvc := map[string]int{"api-gateway": 3, "db": 2, "cache": 2}
	for svc, want := range wantSvc {
		if got := r.ByService[svc]; got != want {
			t.Errorf("ByService[%q] = %d, want %d", svc, got, want)
		}
	}

	if got := r.Percentile(0.5); got != 15*time.Millisecond {
		t.Errorf("p50 = %v, want 15ms", got)
	}
	if got := r.Percentile(0.95); got != 200*time.Millisecond {
		t.Errorf("p95 = %v, want 200ms", got)
	}
}

// TestReport_AnalyzeLineNumbers 是可观测性的硬要求：
// 空行也要占行号，否则错误消息里的行号和文件对不上，线上没法定位。
func TestReport_AnalyzeLineNumbers(t *testing.T) {
	f, err := os.Open("testdata/sample.log")
	if err != nil {
		t.Fatalf("打不开测试数据: %v", err)
	}
	defer f.Close() //nolint:errcheck // 同上

	r, err := Analyze(f)
	if err != nil {
		t.Fatalf("Analyze 失败: %v", err)
	}

	wantLines := []int{8, 9, 10} // sample.log 里坏掉的是第 8、9、10 行
	if len(r.Failed) != len(wantLines) {
		t.Fatalf("失败行数 = %d, want %d", len(r.Failed), len(wantLines))
	}
	for i, want := range wantLines {
		var perr *ParseError
		if !errors.As(r.Failed[i], &perr) {
			t.Fatalf("Failed[%d] 不是 *ParseError: %v", i, r.Failed[i])
		}
		if perr.Line != want {
			t.Errorf("第 %d 个失败的行号 = %d, want %d（空行也要占行号）", i, perr.Line, want)
		}
	}
}

func TestReport_AnalyzeSkipsBlankLines(t *testing.T) {
	in := "\n\n   \n\t\n"
	r, err := Analyze(strings.NewReader(in))
	if err != nil {
		t.Fatalf("Analyze 失败: %v", err)
	}
	if r.Total != 0 || len(r.Failed) != 0 {
		t.Errorf("全是空行时 Total=%d Failed=%d, want 0 和 0（空行既不算成功也不算失败）",
			r.Total, len(r.Failed))
	}
}

func TestReport_AnalyzeEmptyInput(t *testing.T) {
	r, err := Analyze(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Analyze 失败: %v", err)
	}
	if r == nil {
		t.Fatal("空输入应该返回可用的空 Report，不是 nil")
	}
	if r.Total != 0 {
		t.Errorf("Total = %d, want 0", r.Total)
	}
}

// failingReader 在读了一点之后开始报错，用来验证读取错误会被返回。
type failingReader struct{ done bool }

var errIO = errors.New("磁盘炸了")

func (f *failingReader) Read(p []byte) (int, error) {
	if f.done {
		return 0, errIO
	}
	f.done = true
	n := copy(p, "2026-08-18T10:00:00Z INFO db 1 ok\n")
	return n, nil
}

// TestReport_AnalyzePropagatesReadError 验证「单行解析失败」和「读取本身失败」是两回事：
// 前者记进 Failed 继续，后者要返回 error。
func TestReport_AnalyzePropagatesReadError(t *testing.T) {
	_, err := Analyze(&failingReader{})
	if err == nil {
		t.Fatal("底层读取出错时 Analyze 必须返回错误")
	}
	if !errors.Is(err, errIO) {
		t.Errorf("errors.Is(err, errIO) = false，err = %v（包装要用 %%w）", err)
	}
}
