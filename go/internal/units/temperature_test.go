package units

import (
	"math"
	"testing"
)

// eps 是浮点比较容差。
//
// 浮点数永远不要用 == 比较：0.1+0.2 != 0.3。
// 单位换算涉及 9/5 这类除法，误差必然存在，标准做法就是容差比较。
const eps = 1e-9

// closeEnough 报告两个 float64 是否在容差内相等。
//
// 参数写成 float64 而不是 Celsius，是为了让三种温度类型都能复用它 ——
// 调用方需要显式转换，比如 closeEnough(float64(got), float64(want))。
// 这个「烦人」正是类型系统在起作用：你被迫承认自己在跨类型比较。
func closeEnough(got, want float64) bool {
	return math.Abs(got-want) <= eps
}

func TestCToF(t *testing.T) {
	tests := []struct {
		name string
		in   Celsius
		want Fahrenheit
	}{
		{name: "冰点", in: 0, want: 32},
		{name: "沸点", in: 100, want: 212},
		{name: "体温", in: 37, want: 98.6},
		{name: "负温度", in: -40, want: -40}, // 两个温标唯一的交点
		{name: "绝对零度", in: AbsoluteZeroC, want: -459.67},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CToF(tt.in)
			if !closeEnough(float64(got), float64(tt.want)) {
				t.Errorf("CToF(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFToC(t *testing.T) {
	tests := []struct {
		name string
		in   Fahrenheit
		want Celsius
	}{
		{name: "冰点", in: 32, want: 0},
		{name: "沸点", in: 212, want: 100},
		{name: "交点", in: -40, want: -40},
		{name: "体温", in: 98.6, want: 37},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FToC(tt.in)
			if !closeEnough(float64(got), float64(tt.want)) {
				t.Errorf("FToC(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestCToK(t *testing.T) {
	tests := []struct {
		name string
		in   Celsius
		want Kelvin
	}{
		{name: "绝对零度", in: AbsoluteZeroC, want: 0},
		{name: "冰点", in: FreezingC, want: 273.15},
		{name: "沸点", in: BoilingC, want: 373.15},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CToK(tt.in)
			if !closeEnough(float64(got), float64(tt.want)) {
				t.Errorf("CToK(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestKToC(t *testing.T) {
	tests := []struct {
		name string
		in   Kelvin
		want Celsius
	}{
		{name: "绝对零度", in: 0, want: AbsoluteZeroC},
		{name: "冰点", in: 273.15, want: 0},
		{name: "沸点", in: 373.15, want: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := KToC(tt.in)
			if !closeEnough(float64(got), float64(tt.want)) {
				t.Errorf("KToC(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFToK(t *testing.T) {
	tests := []struct {
		name string
		in   Fahrenheit
		want Kelvin
	}{
		{name: "冰点", in: 32, want: 273.15},
		{name: "沸点", in: 212, want: 373.15},
		{name: "绝对零度", in: -459.67, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FToK(tt.in)
			if !closeEnough(float64(got), float64(tt.want)) {
				t.Errorf("FToK(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestRoundTrip 验证来回转换能回到原值。
//
// 这类「性质测试（property-based）」比逐点断言更能抓住实现错误 ——
// 比如你把 9/5 和 5/9 写反了，单点测试可能凑巧通过，往返测试一定挂。
func TestRoundTrip(t *testing.T) {
	for _, c := range []Celsius{-273.15, -40, 0, 37, 100, 1234.5} {
		if got := FToC(CToF(c)); !closeEnough(float64(got), float64(c)) {
			t.Errorf("FToC(CToF(%v)) = %v, want %v", c, got, c)
		}
		if got := KToC(CToK(c)); !closeEnough(float64(got), float64(c)) {
			t.Errorf("KToC(CToK(%v)) = %v, want %v", c, got, c)
		}
	}
}

func TestIsValidC(t *testing.T) {
	tests := []struct {
		name string
		in   Celsius
		want bool
	}{
		{name: "常温", in: 25, want: true},
		{name: "绝对零度本身有效", in: AbsoluteZeroC, want: true},
		{name: "低于绝对零度", in: -300, want: false},
		{name: "刚好低一点点", in: AbsoluteZeroC - 0.01, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidC(tt.in); got != tt.want {
				t.Errorf("IsValidC(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
