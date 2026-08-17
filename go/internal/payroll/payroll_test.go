package payroll

import "testing"

// ---------- 编译期断言 ----------
//
// 这几行不产生任何运行时开销，纯粹让编译器检查设计约束。
// 标准库里到处是这种写法，看到 `var _ Interface = (*Type)(nil)` 就是它。

// Manager、Engineer 是值类型满足 Payer（方法都是值接收者）。
var (
	_ Payer = Manager{}
	_ Payer = Engineer{}
	_ Payer = Contractor{}
)

// 指针也能满足 —— *T 的方法集包含 T 的所有方法（D4 §3）。
var (
	_ Payer = (*Manager)(nil)
	_ Payer = (*Engineer)(nil)
)

// Manager 和 Engineer 必须【嵌入】Employee，而不是各自重复定义字段。
// 只有嵌入了，m.Employee 这个选择器才存在。
var (
	_ = func(m Manager) Employee { return m.Employee }
	_ = func(e Engineer) Employee { return e.Employee }
)

// ---------- 薪酬计算 ----------

func TestMonthlyPay(t *testing.T) {
	tests := []struct {
		name string
		in   Payer
		want int
	}{
		{
			name: "管理岗：10000 + 10000/5 + 5*100",
			in:   Manager{Employee: Employee{Name: "王经理", BaseSalary: 10000}, TeamSize: 5},
			want: 12500,
		},
		{
			name: "管理岗：没有下属",
			in:   Manager{Employee: Employee{Name: "光杆司令", BaseSalary: 10000}, TeamSize: 0},
			want: 12000,
		},
		{
			name: "管理岗：底薪除不尽时向零截断",
			in:   Manager{Employee: Employee{Name: "老李", BaseSalary: 10001}, TeamSize: 1},
			want: 10001 + 2000 + 100, // 10001/5 = 2000
		},
		{
			name: "技术岗：8000 + 3*500",
			in:   Engineer{Employee: Employee{Name: "小张", BaseSalary: 8000}, Level: 3},
			want: 9500,
		},
		{
			name: "技术岗：职级 1",
			in:   Engineer{Employee: Employee{Name: "小新", BaseSalary: 8000}, Level: 1},
			want: 8500,
		},
		{
			name: "外包：160 小时 × 200",
			in:   Contractor{Name: "外包甲", Hours: 160, HourlyRate: 200},
			want: 32000,
		},
		{
			name: "外包：没干活",
			in:   Contractor{Name: "外包乙", Hours: 0, HourlyRate: 200},
			want: 0,
		},
		{
			name: "零值 Manager 不该 panic",
			in:   Manager{},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.MonthlyPay(); got != tt.want {
				t.Errorf("MonthlyPay() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestLabelPromotion 验证 Label 是从 Employee 提升上来的，不是各写一遍。
func TestLabel(t *testing.T) {
	tests := []struct {
		name string
		in   Payer
		want string
	}{
		{name: "管理岗", in: Manager{Employee: Employee{Name: "王经理"}}, want: "王经理"},
		{name: "技术岗", in: Engineer{Employee: Employee{Name: "小张"}}, want: "小张"},
		{name: "外包要带后缀", in: Contractor{Name: "外包甲"}, want: "外包甲（外包）"},
		{name: "零值", in: Engineer{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Label(); got != tt.want {
				t.Errorf("Label() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------- 指针接收者 ----------

func TestRaise(t *testing.T) {
	e := Employee{Name: "小张", BaseSalary: 8000}
	e.Raise(500)
	if e.BaseSalary != 8500 {
		t.Errorf("Raise 之后 BaseSalary = %d, want 8500\n"+
			"（接收者是不是写成值类型了？值接收者拿到的是拷贝，改了等于没改）",
			e.BaseSalary)
	}
}

// TestRaiseThroughEmbedding 验证提升对指针接收者方法一样生效。
func TestRaiseThroughEmbedding(t *testing.T) {
	m := Manager{Employee: Employee{Name: "王经理", BaseSalary: 10000}, TeamSize: 5}
	before := m.MonthlyPay()

	// m 是可寻址的局部变量，所以编译器把这行改写成 (&m.Employee).Raise(1000)。
	m.Raise(1000)

	if m.BaseSalary != 11000 {
		t.Fatalf("提升的 Raise 没生效：BaseSalary = %d, want 11000", m.BaseSalary)
	}
	if after := m.MonthlyPay(); after <= before {
		t.Errorf("加薪后 MonthlyPay 没涨：before=%d after=%d", before, after)
	}
}

// TestValueCopyDoesNotShare 是「值语义」的对照实验。
//
// Manager 拷贝一份之后给拷贝加薪，原件必须不受影响 —— 这和 Java 完全相反，
// Java 里 Manager m2 = m1 之后两者指向同一个对象。
func TestValueCopyDoesNotShare(t *testing.T) {
	m1 := Manager{Employee: Employee{Name: "王经理", BaseSalary: 10000}, TeamSize: 5}
	m2 := m1 // 整个 struct 被拷贝

	m2.Raise(5000)

	if m1.BaseSalary != 10000 {
		t.Errorf("给拷贝加薪影响到了原件：m1.BaseSalary = %d, want 10000", m1.BaseSalary)
	}
	if m2.BaseSalary != 15000 {
		t.Errorf("拷贝没加上薪：m2.BaseSalary = %d, want 15000", m2.BaseSalary)
	}
}

// ---------- 面向接口 ----------

func TestTotalPayroll(t *testing.T) {
	// 关键点：这个切片里三种类型【毫无共同基类】。
	// Contractor 连 Employee 都没嵌入，它只是碰巧有那两个方法。
	ps := []Payer{
		Manager{Employee: Employee{Name: "王经理", BaseSalary: 10000}, TeamSize: 5},  // 12500
		Engineer{Employee: Employee{Name: "小张", BaseSalary: 8000}, Level: 3},      // 9500
		Contractor{Name: "外包甲", Hours: 160, HourlyRate: 200},                      // 32000
		&Manager{Employee: Employee{Name: "李总", BaseSalary: 20000}, TeamSize: 20}, // 26000
	}

	want := 12500 + 9500 + 32000 + 26000
	if got := TotalPayroll(ps); got != want {
		t.Errorf("TotalPayroll = %d, want %d", got, want)
	}
}

func TestTotalPayrollEmpty(t *testing.T) {
	if got := TotalPayroll(nil); got != 0 {
		t.Errorf("TotalPayroll(nil) = %d, want 0", got)
	}
	if got := TotalPayroll([]Payer{}); got != 0 {
		t.Errorf("TotalPayroll(空切片) = %d, want 0", got)
	}
}
