// Package escapex 是 D17 练习 ① 的素材：逃逸分析。
//
// ⚠️ 先别跑编译器。对每个函数，在 notes/D17.md 的小题区写下预测：
// 函数里的哪个变量会被分配到堆上（「逃逸」），为什么。然后再跑：
//
//	go build -gcflags='-m' ./internal/escapex
//	go build -gcflags='-m=2' ./internal/escapex    # 想知道「为什么」时看这个
//
// 对照讲义 §2。每个函数故意只考一个点。
package escapex

import "fmt"

// Point 是练习用的小结构体。
type Point struct{ X, Y int }

var global *Point

// NewPoint ① 返回局部变量的地址。
func NewPoint(x, y int) *Point {
	p := Point{X: x, Y: y}
	return &p
}

// SumLocal ② 取了地址，但指针没离开函数。
func SumLocal() int {
	p := &Point{X: 1, Y: 2}
	return p.X + p.Y
}

// Describe ③ 把结构体交给 fmt。
func Describe(p Point) string {
	return fmt.Sprint(p)
}

// SumFixed ④ make 一个长度是常量的切片。
func SumFixed() int {
	s := make([]int, 8)
	for i := range s {
		s[i] = i
	}
	return s[3]
}

// SumN ⑤ make 一个长度由参数决定的切片。
func SumN(n int) int {
	s := make([]int, n)
	for i := range s {
		s[i] = i
	}
	return len(s)
}

// Counter ⑥ 闭包捕获局部变量，并且闭包本身被返回。
func Counter() func() int {
	c := 0
	return func() int {
		c++
		return c
	}
}

// Keep ⑦ 把参数存进包级变量。
func Keep(p *Point) {
	global = p
}

// Kept 返回 Keep 存下的那个点（只是为了让 global 有读者，不是练习题）。
func Kept() *Point { return global }

// Norm1 ⑧ 只读参数指向的数据。
func Norm1(p *Point) int {
	return abs(p.X) + abs(p.Y)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Big ⑨ 声明一个 1MB 的局部数组。
func Big() byte {
	var a [1 << 20]byte
	a[0] = 1
	return a[0] + a[len(a)-1]
}
