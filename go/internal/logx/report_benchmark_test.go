package logx

import (
	"bytes"
	"os"
	"testing"
)

var sinkReport = &Report{}
var sinkReportErr error

func BenchmarkReport_Analyze(b *testing.B) {
	data, err := os.ReadFile("testdata/sample.log")
	if err != nil {
		b.Fatalf("读测试数据: %v", err)
	}

	for b.Loop() {
		sinkReport, sinkReportErr = Analyze(bytes.NewReader(data))
	}

	/**
	执行结果
	go test -bench=BenchmarkReport -benchmem -run='^$' ./internal/logx
	goos: darwin
	goarch: arm64
	pkg: github.com/hzjconan/learn-program-language/go/internal/logx
	cpu: Apple M1 Pro
	BenchmarkReport_Analyze-10        406885              2943 ns/op            8244 B/op         55 allocs/op
	PASS
	ok      github.com/hzjconan/learn-program-language/go/internal/logx     2.106s
	*/
}
