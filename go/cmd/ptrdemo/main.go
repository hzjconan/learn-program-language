// Command ptrdemo 是 lessons/D2.md §2「值、指针与参数传递」的可运行验证。
//
// 所有结论都不用背 —— 跑一遍，改一改，看输出怎么变：
//
//	go run ./cmd/ptrdemo
package main

import "fmt"

// User 是演示用的小 struct。
type User struct{ Name string }

// ---------- Go 永远传值：拷贝的是指针，不是指针指向的东西 ----------

// modifyPointee 改的是指针【指向的对象】，调用方可见。
func modifyPointee(u *User) { u.Name = "改了指向的对象" }

// reassignPointer 改的是指针【本身】，只影响函数内那份拷贝，调用方看不见。
//
// 注意函数内部打印出来的地址和名字【确实变了】—— 变的是这份拷贝，
// 函数一返回它就没了。这正是「Go 永远传值」最直观的证据。
//
// 顺带一提：写这个函数时被 lint 拦了两次 —— 只赋值不用，ineffassign 报
// 「ineffectual assignment」；赋值前不读参数，staticcheck 报 SA4009
// 「argument is overwritten before first use」。工具比人先看出这行没意义。
func reassignPointer(u *User) {
	fmt.Printf("  （函数内 · 赋值前: %-16s 地址 %p）\n", u.Name, u)
	u = &User{Name: "换了个对象"}
	fmt.Printf("  （函数内 · 赋值后: %-16s 地址 %p ← 确实变了，但只在这里）\n", u.Name, u)
}

// ---------- slice / map / array 的传参差异 ----------

// mutateSliceElem 改元素 —— 走的是同一个底层数组，调用方可见。
func mutateSliceElem(s []int) { s[0] = 999 }

// appendToSlice 里的 append 改的是【函数内那份 slice header 的 len】，调用方看不见。
func appendToSlice(s []int) { _ = append(s, 4) }

// appendViaPointer 能生效，但不地道 —— Go 的惯用做法是返回新 slice 让调用方接住。
func appendViaPointer(s *[]int) { *s = append(*s, 4) }

// appendIdiomatic 才是标准形状，对照 append / strconv.AppendInt / slices.Delete。
func appendIdiomatic(s []int, v int) []int { return append(s, v) }

// mutateMap 能改到调用方的 map —— map 变量本身就是个指针（*hmap）。
func mutateMap(m map[string]int) { m["new"] = 1 }

// mutateArray 改不到 —— 数组是值类型，传参时整个被拷贝。
func mutateArray(a [3]int) { a[0] = 999 }

func main() {
	fmt.Println("=== 指针：拷贝的是指针，不是指向的对象 ===")
	u := &User{Name: "原始"}
	fmt.Printf("传入前:              %-16s (地址 %p)\n", u.Name, u)
	modifyPointee(u)
	fmt.Printf("函数里改 u.Name:     %-16s (地址 %p)  ← 可见\n", u.Name, u)
	reassignPointer(u)
	fmt.Printf("函数里改 u 本身:     %-16s (地址 %p)  ← 没变\n", u.Name, u)

	fmt.Println("\n=== slice：元素共享，长度不共享 ===")
	s := []int{1, 2, 3}
	fmt.Printf("初始:                %v  len=%d cap=%d\n", s, len(s), cap(s))
	mutateSliceElem(s)
	fmt.Printf("函数里 s[0]=999:     %v  ← 可见（同一个底层数组）\n", s)
	appendToSlice(s)
	fmt.Printf("函数里 append:       %v  ← 不可见（len 只改了函数内那份拷贝）\n", s)
	appendViaPointer(&s)
	fmt.Printf("传 *[]int 后 append: %v  ← 可见，但不地道\n", s)
	s = appendIdiomatic(s, 5)
	fmt.Printf("返回值 + 调用方接住: %v  ← ⭐ 这才是 Go 的写法\n", s)

	fmt.Println("\n=== map：变量本身就是指针 ===")
	m := map[string]int{"a": 1}
	mutateMap(m)
	fmt.Printf("函数里 m[\"new\"]=1:   %v  ← 可见\n", m)

	fmt.Println("\n=== 对照组：数组是彻底的值 ===")
	arr := [3]int{1, 2, 3}
	mutateArray(arr)
	fmt.Printf("函数里 a[0]=999:     %v  ← 不可见（整个数组被拷贝）\n", arr)

	fmt.Println("\n💡 试试改这里：把 s 换成 make([]int, 3, 10)（cap 留富余）再跑一遍，")
	fmt.Println("   看 appendToSlice 的行为有什么变化 —— 那是 D3 的主题。")
}
