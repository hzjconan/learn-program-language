package crawl

import (
	"testing"
	"time"
)

var sinkResult []Result

func BenchmarkCrawl_FetchAll(b *testing.B) {
	/*
						执行结果

						goos: darwin
						goarch: arm64
						pkg: github.com/hzjconan/learn-program-language/go/internal/crawl
						cpu: Apple M1 Pro
						BenchmarkCrawl_FetchAll/1个worker,fetch耗时10ms完成-10                                 8         132383755 ns/op         2398 B/op         22 allocs/op
						BenchmarkCrawl_FetchAll/2个worker,fetch耗时10ms完成-10                                18          66050062 ns/op         2492 B/op         24 allocs/op
						BenchmarkCrawl_FetchAll/5个worker,fetch耗时10ms完成-10                                37          32777607 ns/op         2889 B/op         33 allocs/op
						BenchmarkCrawl_FetchAll/10个worker,fetch耗时10ms完成-10                               55          21944698 ns/op         4101 B/op         49 allocs/op
						BenchmarkCrawl_FetchAll/1个worker,fetch耗时0ms完成-10                             195704              6080 ns/op         1728 B/op         19 allocs/op
						BenchmarkCrawl_FetchAll/2个worker,fetch耗时0ms完成-10                             173182              6634 ns/op         1801 B/op         21 allocs/op
						BenchmarkCrawl_FetchAll/5个worker,fetch耗时0ms完成-10                             147580              8137 ns/op         2018 B/op         27 allocs/op
						BenchmarkCrawl_FetchAll/10个worker,fetch耗时0ms完成-10                            118476              9961 ns/op         2379 B/op         37 allocs/op
						PASS
						ok      github.com/hzjconan/learn-program-language/go/internal/crawl    10.254s


		benchmark 里的 fetch 0ms 不是「CPU 密集」，是「零工作量」。 三种情况要分开：

		┌─────────────────────┬──────────────────────┬────────────────────────┐
		│        类型         │   每个任务在干什么   │    加 worker 的效果    │
		├─────────────────────┼──────────────────────┼────────────────────────┤
		│ 零工作量（delay=0） │ 什么都不干，立刻返回 │ 纯亏——只剩协调开销     │
		├─────────────────────┼──────────────────────┼────────────────────────┤
		│ CPU 密集            │ 占着 CPU 算          │ 有用，到核数为止       │
		├─────────────────────┼──────────────────────┼────────────────────────┤
		│ IO 密集             │ 等外部，不占 CPU     │ 有用，天花板远高于核数 │
		└─────────────────────┴──────────────────────┴────────────────────────┘

		 三种情况要分开：
		   零工作量（本例 fetch 0ms）：只剩协调开销，worker 越多越慢
		   CPU 密集：并发有用，但天花板 = 核数，超过只是徒增切换开销
		   IO 密集（本例 fetch 10ms）：并发收益最大，天花板远高于核数
		 经验公式：最优 worker 数 ≈ 核数 × (1 + 等待时间/计算时间)
		 真实爬虫是 IO 密集的，workers 开到几十上百都合理 ——
		 瓶颈会先落在对方服务器或连接数上，而不是 CPU。

	*/
	tests := []struct {
		name     string
		workers  int
		duration time.Duration
	}{
		{name: "1个worker,fetch耗时10ms完成", workers: 1, duration: 10 * time.Millisecond},
		{name: "2个worker,fetch耗时10ms完成", workers: 2, duration: 10 * time.Millisecond},
		{name: "5个worker,fetch耗时10ms完成", workers: 5, duration: 10 * time.Millisecond},
		{name: "10个worker,fetch耗时10ms完成", workers: 10, duration: 10 * time.Millisecond},

		{name: "1个worker,fetch耗时0ms完成", workers: 1, duration: 0 * time.Millisecond},
		{name: "2个worker,fetch耗时0ms完成", workers: 2, duration: 0 * time.Millisecond},
		{name: "5个worker,fetch耗时0ms完成", workers: 5, duration: 0 * time.Millisecond},
		{name: "10个worker,fetch耗时0ms完成", workers: 10, duration: 0 * time.Millisecond},
	}
	for _, v := range tests {
		b.Run(v.name, func(b *testing.B) {
			f := newFetcher(v.duration)
			for b.Loop() {
				sinkResult = FetchAll(f, []string{"/", "/a", "/b", "/a1", "/a2", "/b1", "/", "/a", "/b", "/a1", "/a2", "/b1"}, v.workers)
			}
		})
	}
}
