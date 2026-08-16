package mathx

import "testing"

func TestSafeDivide(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want int
	}{
		{name: "整除", a: 10, b: 2, want: 5},
		{name: "向零截断", a: 7, b: 2, want: 3},
		{name: "负数也向零截断", a: -7, b: 2, want: -3},
		{name: "被除数为零", a: 0, b: 5, want: 0},
		{name: "除以负数", a: 10, b: -2, want: -5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeDivide(tt.a, tt.b)
			if err != nil {
				t.Fatalf("SafeDivide(%d, %d) 返回错误 %v, want nil", tt.a, tt.b, err)
			}
			if got != tt.want {
				t.Errorf("SafeDivide(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestSafeDivideByZero 是这道题的全部意义所在。
//
// 如果 recover 没写对，这个测试不会「失败」—— 它会让整个测试进程 panic 崩掉。
// 那种输出（一大堆 goroutine 栈）和普通的 FAIL 长得完全不一样，注意分辨。
func TestSafeDivideByZero(t *testing.T) {
	tests := []struct {
		name    string
		a, b    int
		wantMsg string
	}{
		{name: "正数除以零", a: 5, b: 0, wantMsg: "除以零: 5 / 0"},
		{name: "负数除以零", a: -3, b: 0, wantMsg: "除以零: -3 / 0"},
		{name: "零除以零", a: 0, b: 0, wantMsg: "除以零: 0 / 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SafeDivide(tt.a, tt.b)
			if err == nil {
				t.Fatalf("SafeDivide(%d, %d) = %d, nil，want 错误", tt.a, tt.b, got)
			}
			// 出错时另一个返回值应该是零值，调用方不该去读它。
			if got != 0 {
				t.Errorf("出错时 result = %d, want 0", got)
			}
			if err.Error() != tt.wantMsg {
				t.Errorf("err.Error() = %q, want %q", err.Error(), tt.wantMsg)
			}
		})
	}
}
