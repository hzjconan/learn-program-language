package logx

// D17 练习 ② 的基础设施：大输入的 benchmark + 锁住输出的 golden 测试。
//
// ⭐ 优化的铁律：先有能证明「行为没变」的测试，再动代码。
// TestBigReport_Golden 把 2 万行输入的完整报告（文本 + JSON）存成 golden 文件。
// 你做的任何优化，这两个文件都必须一个字节都不变。
//
// 当前实现产出的 golden 已经生成好了。只有在你【确认】输出就应该变的时候才更新：
//
//	go test ./internal/logx -run TestBigReport_Golden -update

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "更新 golden 文件")

// bigLog 生成 n 行确定性的日志：5 个服务、带 WARN/ERROR、耗时 0–1999ms 分布打散，
// 外加少量空行和坏行，让失败路径也被走到。同样的 n 永远生成同样的字节。
func bigLog(n int) []byte {
	var b bytes.Buffer
	b.Grow(n * 64)
	svcs := []string{"api-gateway", "orders", "payments", "users", "search"}
	lvs := []string{"INFO", "INFO", "INFO", "WARN", "ERROR"}
	t0 := time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC)
	for i := range n {
		switch {
		case i%997 == 0:
			b.WriteString("\n") // 空行
		case i%1009 == 0:
			b.WriteString("this line is broken\n") // 坏行
		default:
			fmt.Fprintf(&b, "%s %s %s %d GET /items/%d -> 200\n",
				t0.Add(time.Duration(i)*time.Second).Format(time.RFC3339),
				lvs[i%len(lvs)], svcs[(i/3)%len(svcs)], (i*7919)%2000, i)
		}
	}
	return b.Bytes()
}

func TestBigReport_Golden(t *testing.T) {
	rpt, err := Analyze(bytes.NewReader(bigLog(20_000)))
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	for _, tc := range []struct {
		name string
		f    Formatter
	}{
		{"big_report.txt", TextFormatter{Top: 10}},
		{"big_report.json", JSONFormatter{Top: 10}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got bytes.Buffer
			if err := tc.f.Format(&got, rpt); err != nil {
				t.Fatalf("Format: %v", err)
			}
			path := filepath.Join("testdata", tc.name)
			if *update {
				if err := os.WriteFile(path, got.Bytes(), 0o644); err != nil {
					t.Fatalf("写 golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("读 golden %s: %v（第一次跑要加 -update）", path, err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Errorf("%s 和 golden 不一致——优化改变了输出。\n--- got ---\n%s\n--- want ---\n%s",
					tc.name, got.String(), want)
			}
		})
	}
}

// 两个 benchmark 分开测「解析」和「出报告」，这样你能看出时间花在哪一半。
// 20 万行 ≈ 12MB 输入。

var bigInput = bigLog(200_000)

func BenchmarkBig_Analyze(b *testing.B) {
	for b.Loop() {
		sinkReport, sinkReportErr = Analyze(bytes.NewReader(bigInput))
	}
}

var sinkFormatErr error

// BenchmarkBig_EndToEnd 是 cmd/loganalyze 真实的用法：解析一遍，出一份报告。
// ⭐ 优化效果以它为准。下面两个拆开的 benchmark 只用来判断时间花在哪一半。
func BenchmarkBig_EndToEnd(b *testing.B) {
	var out bytes.Buffer
	for b.Loop() {
		rpt, err := Analyze(bytes.NewReader(bigInput))
		if err != nil {
			b.Fatalf("Analyze: %v", err)
		}
		out.Reset()
		sinkFormatErr = TextFormatter{Top: 10}.Format(&out, rpt)
	}
}

// BenchmarkBig_Format 只测出报告。
//
// ⚠️ 每一轮用的是【同一个】Report。如果你在 Report 里加了缓存（比如排好序的耗时），
// 只有第一轮会真的算，后面全是命中缓存 —— 这个 benchmark 会显示「快了很多」，
// 但 cmd/loganalyze 每次只 Format 一次，根本享受不到。判断优化效果看 EndToEnd（讲义 §5.3）。
func BenchmarkBig_Format(b *testing.B) {
	rpt, err := Analyze(bytes.NewReader(bigInput))
	if err != nil {
		b.Fatalf("Analyze: %v", err)
	}
	var out bytes.Buffer
	for b.Loop() {
		out.Reset()
		sinkFormatErr = TextFormatter{Top: 10}.Format(&out, rpt)
	}
}
