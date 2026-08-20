package logx

import "testing"

var sinkEntry = Entry{}
var sinkErr error

func BenchmarkLogx_ParseLine(b *testing.B) {
	for b.Loop() {
		sinkEntry, sinkErr = ParseLine(1, "2026-08-18T10:00:00Z INFO api-gateway 15 GET /users/42 -> 200")
	}

	/**
	执行结果
	go test -bench=BenchmarkLogx -benchmem -run='^$' ./internal/logx
	goos: darwin
	goarch: arm64
	pkg: github.com/hzjconan/learn-program-language/go/internal/logx
	cpu: Apple M1 Pro
	BenchmarkLogx_ParseLine-10      11367345                88.87 ns/op           80 B/op          1 allocs/op
	PASS
	ok      github.com/hzjconan/learn-program-language/go/internal/logx     2.710s
	*/
}
