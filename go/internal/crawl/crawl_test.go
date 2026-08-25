package crawl

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

// 测试文件是我写的，别改。你的实现要让它们全绿。
//
// 你自己想补的用例写在 crawl_extra_test.go 里，函数名用 TestCrawl_Extra_Xxx。

// ---------- 测试替身 ----------

// fakeFetcher 是一个可控的假抓取器。
//
// 它能做三件事：按图返回链接、模拟延迟、记录「同时在跑的最大数量」。
type fakeFetcher struct {
	graph map[string][]string // url → 它页面里的链接
	fails map[string]error    // url → 要返回的错误
	delay time.Duration       // 每次抓取的模拟耗时

	inFlight atomic.Int64 // 当前正在跑的数量
	maxSeen  atomic.Int64 // 历史最大值
	calls    atomic.Int64 // 总调用次数
}

func (f *fakeFetcher) Fetch(url string) ([]string, error) {
	n := f.inFlight.Add(1)
	for { // 无锁地把 maxSeen 抬到至少 n
		old := f.maxSeen.Load()
		if n <= old || f.maxSeen.CompareAndSwap(old, n) {
			break
		}
	}
	defer f.inFlight.Add(-1)

	f.calls.Add(1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if err, ok := f.fails[url]; ok {
		return nil, err
	}
	return f.graph[url], nil
}

var errBoom = errors.New("抓取失败")

func newFetcher(delay time.Duration) *fakeFetcher {
	return &fakeFetcher{
		graph: map[string][]string{
			"/":     {"/a", "/b"},
			"/a":    {"/a1", "/a2", "/b"}, // /b 重复出现，考去重
			"/b":    {"/b1"},
			"/a1":   {"/deep"},
			"/a2":   {},
			"/b1":   {"/deep"}, // /deep 从两条路都能到
			"/deep": {},
		},
		fails: map[string]error{},
		delay: delay,
	}
}

// assertNoLeak 检查一段代码跑完后 goroutine 数量回到原点。
func assertNoLeak(t *testing.T, fn func()) {
	t.Helper()
	before := runtime.NumGoroutine()
	fn()

	// goroutine 退出不是瞬时的，给它一点时间；最多等 2 秒
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("goroutine 泄漏了：调用前 %d 条，等了 2 秒后仍有 %d 条\n"+
		"（超时之后那条 goroutine 还在往 channel 里发结果，而已经没人接收了 —— D8 §7）",
		before, runtime.NumGoroutine())
}

// ---------- FetchWithTimeout ----------

func TestCrawl_FetchWithTimeoutOK(t *testing.T) {
	f := newFetcher(0)
	links, err := FetchWithTimeout(f, "/", time.Second)
	if err != nil {
		t.Fatalf("没超时却报错: %v", err)
	}
	if !slices.Equal(links, []string{"/a", "/b"}) {
		t.Errorf("links = %v, want [/a /b]", links)
	}
}

func TestCrawl_FetchWithTimeoutFires(t *testing.T) {
	f := newFetcher(200 * time.Millisecond)

	start := time.Now()
	_, err := FetchWithTimeout(f, "/", 20*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("应该超时")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("errors.Is(err, ErrTimeout) = false，err = %v", err)
	}
	if elapsed > 150*time.Millisecond {
		t.Errorf("等了 %v 才返回 —— 超时应该立刻生效，不该等 Fetch 跑完", elapsed)
	}
}

// TestCrawl_FetchWithTimeoutPropagatesError 验证 Fetch 自己的错误要原样传出来。
func TestCrawl_FetchWithTimeoutPropagatesError(t *testing.T) {
	f := newFetcher(0)
	f.fails["/"] = errBoom

	_, err := FetchWithTimeout(f, "/", time.Second)
	if !errors.Is(err, errBoom) {
		t.Errorf("errors.Is(err, errBoom) = false，err = %v", err)
	}
	if errors.Is(err, ErrTimeout) {
		t.Error("这不是超时，不该报 ErrTimeout")
	}
}

// TestCrawl_FetchWithTimeoutNoLeak 是今天的题眼。
func TestCrawl_FetchWithTimeoutNoLeak(t *testing.T) {
	f := newFetcher(80 * time.Millisecond)

	assertNoLeak(t, func() {
		for range 30 {
			_, _ = FetchWithTimeout(f, "/", 5*time.Millisecond) // 全部超时
		}
		time.Sleep(200 * time.Millisecond) // 等那些 Fetch 都跑完
	})
}

// ---------- FetchAll ----------

func TestCrawl_FetchAllOrder(t *testing.T) {
	f := newFetcher(10 * time.Millisecond)
	urls := []string{"/", "/a", "/b", "/a1", "/a2", "/b1"}

	got := FetchAll(f, urls, 3)

	if len(got) != len(urls) {
		t.Fatalf("len = %d, want %d", len(got), len(urls))
	}
	for i, u := range urls {
		if got[i].URL != u {
			t.Errorf("got[%d].URL = %q, want %q（返回顺序必须和传入顺序一致，"+
				"不是完成顺序）", i, got[i].URL, u)
		}
		if got[i].Depth != 0 {
			t.Errorf("got[%d].Depth = %d, want 0", i, got[i].Depth)
		}
	}
	if !slices.Equal(got[0].Links, []string{"/a", "/b"}) {
		t.Errorf("got[0].Links = %v, want [/a /b]", got[0].Links)
	}
}

// TestCrawl_FetchAllRespectsLimit 验证并发上限真的生效。
func TestCrawl_FetchAllRespectsLimit(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) {
			f := newFetcher(20 * time.Millisecond)
			urls := make([]string, 12)
			for i := range urls {
				urls[i] = "/"
			}

			FetchAll(f, urls, workers)

			if got := int(f.maxSeen.Load()); got > workers {
				t.Errorf("同时在跑的最大数量 = %d，超过了上限 %d", got, workers)
			}
			if got := int(f.maxSeen.Load()); workers > 1 && got < 2 {
				t.Errorf("最大并发只有 %d —— 是不是根本没并发起来？", got)
			}
		})
	}
}

func TestCrawl_FetchAllErrors(t *testing.T) {
	f := newFetcher(0)
	f.fails["/b"] = errBoom

	got := FetchAll(f, []string{"/a", "/b", "/b1"}, 2)

	if got[0].Err != nil {
		t.Errorf("/a 不该出错: %v", got[0].Err)
	}
	if !errors.Is(got[1].Err, errBoom) {
		t.Errorf("/b 的 Err = %v, want errBoom", got[1].Err)
	}
	if got[1].Links != nil {
		t.Errorf("出错时 Links 应该是 nil，实际 %v", got[1].Links)
	}
	if got[2].Err != nil {
		t.Errorf("/b1 不该受 /b 失败的影响: %v", got[2].Err)
	}
}

func TestCrawl_FetchAllEdges(t *testing.T) {
	f := newFetcher(0)

	if got := FetchAll(f, nil, 3); got != nil {
		t.Errorf("FetchAll(nil) = %v, want nil", got)
	}
	if got := FetchAll(f, []string{}, 3); got != nil {
		t.Errorf("FetchAll(空切片) = %v, want nil", got)
	}

	// workers <= 0 当作 1，不能 panic 也不能死锁
	// ⚠️ 不能只检查长度 —— 0 个 worker 时返回的是一片零值 Result，长度照样对
	for _, w := range []int{0, -5} {
		got := FetchAll(f, []string{"/a", "/b"}, w)
		if len(got) != 2 {
			t.Fatalf("workers=%d 时返回 %d 条, want 2", w, len(got))
		}
		if got[0].URL != "/a" || got[1].URL != "/b" {
			t.Errorf("workers=%d 时结果没被填充：%+v\n（workers <= 0 要当作 1，不是 0 个 worker）",
				w, got)
		}
	}
}

func TestCrawl_FetchAllNoLeak(t *testing.T) {
	f := newFetcher(time.Millisecond)
	assertNoLeak(t, func() {
		for range 20 {
			FetchAll(f, []string{"/", "/a", "/b", "/a1"}, 3)
		}
	})
}

// ---------- Crawl ----------

func urlsOf(rs []Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.URL
	}
	return out
}

func TestCrawl_Depth(t *testing.T) {
	tests := []struct {
		name     string
		maxDepth int
		want     []string // 已按 (Depth, URL) 排好序
	}{
		{name: "只抓种子", maxDepth: 0, want: []string{"/"}},
		{name: "一层", maxDepth: 1, want: []string{"/", "/a", "/b"}},
		{name: "两层", maxDepth: 2, want: []string{"/", "/a", "/b", "/a1", "/a2", "/b1"}},
		{name: "三层", maxDepth: 3, want: []string{"/", "/a", "/b", "/a1", "/a2", "/b1", "/deep"}},
		{name: "深度超过图的规模", maxDepth: 9, want: []string{"/", "/a", "/b", "/a1", "/a2", "/b1", "/deep"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFetcher(0)
			got := urlsOf(Crawl(f, "/", tt.maxDepth, 3))
			if !slices.Equal(got, tt.want) {
				t.Errorf("Crawl 结果 = %v\nwant %v\n（要按 Depth 升序、同层 URL 字典序排好）",
					got, tt.want)
			}
		})
	}
}

// TestCrawl_Dedup 验证每个 URL 只抓一次。
//
// 图里 /b 既是 / 的链接，也是 /a 的链接；/deep 从 /a1 和 /b1 两条路都能到。
func TestCrawl_Dedup(t *testing.T) {
	f := newFetcher(0)
	got := Crawl(f, "/", 3, 3)

	seen := map[string]int{}
	for _, r := range got {
		seen[r.URL]++
	}
	for u, n := range seen {
		if n != 1 {
			t.Errorf("%q 出现了 %d 次，应该只有 1 次", u, n)
		}
	}
	if n := int(f.calls.Load()); n != 7 {
		t.Errorf("Fetch 被调用了 %d 次，want 7（图里一共 7 个页面，每个只该抓一次）", n)
	}
}

func TestCrawl_Depths(t *testing.T) {
	f := newFetcher(0)
	want := map[string]int{"/": 0, "/a": 1, "/b": 1, "/a1": 2, "/a2": 2, "/b1": 2, "/deep": 3}

	for _, r := range Crawl(f, "/", 5, 3) {
		if w, ok := want[r.URL]; ok && r.Depth != w {
			t.Errorf("%q 的 Depth = %d, want %d（同一个 URL 从多条路可达时，"+
				"应该记最先到达的那一层）", r.URL, r.Depth, w)
		}
	}
}

// TestCrawl_FailedPageNotExpanded 验证抓取失败的页面出现在结果里，但不展开它的链接。
func TestCrawl_FailedPageNotExpanded(t *testing.T) {
	f := newFetcher(0)
	f.fails["/a"] = errBoom

	got := Crawl(f, "/", 3, 3)
	byURL := map[string]Result{}
	for _, r := range got {
		byURL[r.URL] = r
	}

	if _, ok := byURL["/a"]; !ok {
		t.Fatal("抓取失败的 /a 也应该出现在结果里（带 Err）")
	}
	if !errors.Is(byURL["/a"].Err, errBoom) {
		t.Errorf("/a 的 Err = %v, want errBoom", byURL["/a"].Err)
	}
	if _, ok := byURL["/a1"]; ok {
		t.Error("/a 抓失败了，它的链接 /a1 不该被展开")
	}
	if _, ok := byURL["/b1"]; !ok {
		t.Error("/b 没失败，它的链接 /b1 应该照常展开")
	}
}

func TestCrawl_RespectsLimit(t *testing.T) {
	f := newFetcher(15 * time.Millisecond)
	Crawl(f, "/", 3, 2)

	if got := int(f.maxSeen.Load()); got > 2 {
		t.Errorf("同时在跑的最大数量 = %d，超过了上限 2", got)
	}
}

func TestCrawl_NoLeak(t *testing.T) {
	assertNoLeak(t, func() {
		for range 10 {
			f := newFetcher(time.Millisecond)
			Crawl(f, "/", 3, 3)
		}
	})
}

// TestCrawl_ResultLinksNoAliasing 验证返回的 Links 不和 Fetcher 内部数据共享底层数组。
//
// 这是 D3 别名 + D4 §1 浅拷贝 + D7 review 里 store.Tags 那条的同类问题。
// ⚠️ 早先的版本用「第二次调用换个新 fetcher」来验，那是测不出来的 —— 污染不到新对象。
// 必须直接对着【同一个】fetcher 的内部数据查。
func TestCrawl_ResultLinksNoAliasing(t *testing.T) {
	f := newFetcher(0)
	before := slices.Clone(f.graph["/"])

	got := FetchAll(f, []string{"/"}, 1)
	got[0].Links[0] = "被调用方改掉了"

	if !slices.Equal(f.graph["/"], before) {
		t.Errorf("调用方改了返回的 Links，污染了 Fetcher 内部数据：\n"+
			"  改动前 %v\n  改动后 %v\n（f.Fetch 返回的切片要 slices.Clone 之后再放进 Result）",
			before, f.graph["/"])
	}
}

// TestCrawl_WholeLevelFails 验证「某一层全部失败」时不会重复抓取。
//
// 这个场景抓的是：更新下一层队列的那行赋值，必须在遍历完整层【之后】执行。
// 如果它挂在「这条成功了」的分支里，整层全挂时队列就不更新 —— 上一层的 URL 会被反复抓。
func TestCrawl_WholeLevelFails(t *testing.T) {
	f := newFetcher(0)
	f.fails["/a"] = errBoom
	f.fails["/b"] = errBoom // 第 1 层的两个全挂

	done := make(chan []Result, 1)
	go func() { done <- Crawl(f, "/", 3, 3) }()

	var got []Result
	select {
	case got = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Crawl 没有返回 —— 队列没更新导致死循环了？")
	}

	seen := map[string]int{}
	for _, r := range got {
		seen[r.URL]++
	}
	for u, n := range seen {
		if n != 1 {
			t.Errorf("%q 在结果里出现了 %d 次，应该只有 1 次（整层失败后队列没更新，被反复抓了）", u, n)
		}
	}
	if n := int(f.calls.Load()); n != 3 {
		t.Errorf("Fetch 调用了 %d 次, want 3（/ 一次，/a 和 /b 各一次，然后就该停）", n)
	}
}

// TestCrawl_SortMatters 用一个「字典序和发现顺序相反」的图，确保排序真的在起作用。
//
// 早先的测试图里两者恰好一致，排不排结果都一样 —— 那条测试对「去掉排序」是免疫的。
func TestCrawl_SortMatters(t *testing.T) {
	f := &fakeFetcher{
		graph: map[string][]string{
			"/":  {"/z", "/m", "/a"}, // 发现顺序 z,m,a；字典序 a,m,z
			"/z": {},
			"/m": {},
			"/a": {},
		},
		fails: map[string]error{},
	}

	got := urlsOf(Crawl(f, "/", 1, 3))
	want := []string{"/", "/a", "/m", "/z"}
	if !slices.Equal(got, want) {
		t.Errorf("Crawl 结果 = %v\nwant %v\n（同层要按 URL 字典序排，不是发现顺序）", got, want)
	}
}
