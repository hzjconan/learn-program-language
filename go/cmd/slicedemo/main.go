// Command slicedemo 是 lessons/D3.md 的可运行验证。
//
// slice 的坑几乎全部来自「底层数组共享」+「append 有时扩容有时不扩容」这两件事的组合。
// 光看文字很难信，跑一遍就懂了：
//
//	go run ./cmd/slicedemo
package main

import "fmt"

func main() {
	demoHeader()
	demoAliasNoGrow()
	demoAliasGrow()
	demoThreeIndex()
	demoTruncateTrap()
	demoLeak()
	demoMapOrder()
	demoString()
}

func title(s string) { fmt.Printf("\n=== %s ===\n", s) }

func demoHeader() {
	title("① slice header：一个切片就是「指针 + len + cap」")
	arr := [6]int{0, 1, 2, 3, 4, 5}
	s := arr[1:4]
	fmt.Printf("arr = %v\n", arr)
	fmt.Printf("s := arr[1:4] → %v   len=%d cap=%d\n", s, len(s), cap(s))
	fmt.Println("  cap 是 5 不是 3：从起点 1 数到底层数组末尾还有 5 个位置")
	fmt.Println("  ⚠️ 这意味着 s 能通过 append「看到」并覆盖 arr[4]")
}

func demoAliasNoGrow() {
	title("② append 不扩容时：改的是【同一块】内存")
	arr := [6]int{0, 1, 2, 3, 4, 5}
	s := arr[1:4] // len=3 cap=5，还有富余
	fmt.Printf("append 前: arr=%v  s=%v (len=%d cap=%d)\n", arr, s, len(s), cap(s))
	s = append(s, 999)
	fmt.Printf("append 后: arr=%v  s=%v\n", arr, s)
	fmt.Println("  ⚠️ arr[4] 被 999 覆盖了 —— 你以为只是往 s 里加了个元素")
}

func demoAliasGrow() {
	title("③ append 扩容时：偷偷换了一块新内存，联系断开")
	arr := [4]int{0, 1, 2, 3}
	s := arr[1:4] // len=3 cap=3，没有富余
	fmt.Printf("append 前: arr=%v  s=%v (len=%d cap=%d)  地址 %p\n", arr, s, len(s), cap(s), s)
	s = append(s, 999)
	fmt.Printf("append 后: arr=%v  s=%v (len=%d cap=%d)  地址 %p\n", arr, s, len(s), cap(s), s)
	fmt.Println("  arr 没被改动，因为 s 已经搬到别处了")
	fmt.Println("  ⚠️ 真正可怕的是：②③ 的代码写法完全一样，行为却相反 ——")
	fmt.Println("     取决于运行时 cap 够不够。这就是 slice 事故难复现的根源。")
}

func demoThreeIndex() {
	title("④ 三索引切片 s[a:b:c]：把 cap 焊死，杜绝②")
	arr := [6]int{0, 1, 2, 3, 4, 5}
	// s[a:b:c] → len = b-a，cap = c-a。三个数都是【下标】，减起点才得到长度。
	s := arr[1:4:4] // len = 4-1 = 3，cap = 4-1 = 3（第三个数 4 减起点 1）
	fmt.Printf("s := arr[1:4:4] → %v  len=%d cap=%d\n", s, len(s), cap(s))
	fmt.Println("  约束：0 ≤ a ≤ b ≤ c ≤ cap(原) —— 三索引只能【缩小】cap，不能扩大")
	s = append(s, 999)
	fmt.Printf("append 后: arr=%v  s=%v\n", arr, s)
	fmt.Println("  ✅ arr 毫发无伤：cap 已满 → append 必然分配新数组")
	fmt.Println("  给别人返回子切片时，这是最省事的自保手段")
}

func demoTruncateTrap() {
	title("⑤ s[:0] 复用陷阱：最常见的「高效」写法，会毁掉入参")
	src := []int{1, 2, 3, 4, 5, 6}
	fmt.Printf("原始 src = %v\n", src)

	// 网上到处能搜到的「零分配过滤」写法
	out := src[:0]
	for _, v := range src {
		if v%2 == 0 {
			out = append(out, v)
		}
	}
	fmt.Printf("过滤结果 = %v   ← 看着对\n", out)
	fmt.Printf("但 src  = %v   ← 调用方的数据被就地改写了！\n", src)
}

func demoLeak() {
	title("⑥ 子切片会拖住整个底层数组（内存泄漏）")
	big := make([]byte, 10<<20) // 10MB
	small := big[:3]
	fmt.Printf("small = %v  len=%d cap=%d\n", small, len(small), cap(small))
	fmt.Printf("  ⚠️ cap 是 %d —— 只要 small 活着，那 10MB 就回收不了\n", cap(small))

	safe := make([]byte, 3)
	copy(safe, big)
	fmt.Printf("copy 之后 safe len=%d cap=%d —— 10MB 可以被 GC 了\n", len(safe), cap(safe))
}

func demoMapOrder() {
	title("⑦ map 遍历顺序是【故意】随机的")
	m := map[string]int{}
	for i := range 10 {
		m[fmt.Sprintf("k%d", i)] = i
	}
	for i := range 4 {
		fmt.Printf("  第 %d 次遍历: ", i+1)
		for k := range m {
			fmt.Print(k, " ")
		}
		fmt.Println()
	}
	fmt.Println("  不是「碰巧无序」，是运行时每次都随机一个起点 —— 防止你写出依赖顺序的代码")
}

func demoString() {
	title("⑧ string：索引给你 byte，range 给你 rune")
	s := "Go语言"
	fmt.Printf("s = %q\n", s)
	fmt.Printf("len(s) = %d   ← 字节数，不是字符数！\n", len(s))

	fmt.Print("  s[i] 逐字节: ")
	for i := 0; i < len(s); i++ {
		fmt.Printf("%d ", s[i])
	}
	fmt.Println("  ← 中文被拆成了 3 个字节")

	fmt.Print("  range 逐字符: ")
	for i, r := range s {
		fmt.Printf("[%d]%c ", i, r)
	}
	fmt.Println("  ← 下标是【字节偏移】，会跳跃")

	fmt.Printf("  []rune(s) 长度 = %d   ← 这才是字符数\n", len([]rune(s)))
	fmt.Printf("  s[0:2] = %q            ← 切在字符边界上，正常\n", s[0:2])
	fmt.Printf("  s[0:3] = %q     ← 切进「语」的中间，切出半个字符\n", s[0:3])
	fmt.Printf("  s[0:5] = %q      ← 「语」占 3 字节，切到 5 才完整\n", s[0:5])
}
