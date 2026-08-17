// Package payroll 计算不同用工形式的月度薪酬。
//
// 这个包是一道重构题：把下面这段「Java 味」的继承层次，改写成 Go 的组合 + 接口。
//
// # 原始的 Java 设计
//
//	abstract class Employee {
//	    protected String name;
//	    protected int baseSalary;
//
//	    // 模板方法：定义骨架，把可变部分交给子类
//	    public int calculatePay() {
//	        return baseSalary + bonus();
//	    }
//	    protected abstract int bonus();      // 子类必须实现
//
//	    public String label() { return name; }
//	}
//
//	class Manager extends Employee {
//	    private int teamSize;
//	    protected int bonus() { return baseSalary / 5 + teamSize * 100; }
//	}
//
//	class Engineer extends Employee {
//	    private int level;
//	    protected int bonus() { return level * 500; }
//	}
//
// # 为什么不能直译成 Go
//
// 直译写法是「Employee 定义 CalculatePay 和 Bonus，Manager 嵌入 Employee 并
// 定义自己的 Bonus」。它编译得过，但 CalculatePay 里调到的永远是
// Employee.Bonus，不是 Manager.Bonus —— **嵌入没有动态派发**。
// 亲眼看一遍：lessons/D4.md §4，以及 `go run ./cmd/embeddemo` 的第 ② 段。
//
// # Go 的解法
//
//  1. 「能算薪酬」这个能力用【接口】表达 —— 多态靠它，不靠嵌入。
//  2. 共有字段用【嵌入】复用，纯粹为了少写代码，不指望它提供多态。
//  3. Contractor 不嵌入 Employee（它没有底薪），用它说明组合比继承灵活：
//     继承要求「是一个 Employee」，接口只要求「能算出薪酬」。
package payroll

import "fmt"

// Payer 是「能算出本月应付薪酬」这个能力。
//
// 接口由使用方定义：TotalPayroll 只关心这两个方法，不关心你是不是员工。
// 各实现类型里【不需要】写任何 "implements Payer" —— 隐式实现（D5 细讲）。
type Payer interface {
	// MonthlyPay 返回本月应付金额，单位是元。
	MonthlyPay() int
	// Label 返回用于账单和日志的可读名称。
	Label() string
}

// Employee 承载正式员工共有的数据。
//
// 它【不是】抽象基类：自己不知道谁嵌入了它，也不定义任何模板方法。
// 存在的唯一目的是让 Manager 和 Engineer 少写两个字段。
type Employee struct {
	// Name 是员工姓名。
	Name string
	// BaseSalary 是月底薪，单位元。
	BaseSalary int
}

// Label 返回员工姓名。
//
// 这个方法会被提升到所有嵌入了 Employee 的类型上 —— Manager 和 Engineer
// 因此自动满足 Payer 的 Label 部分，不需要各写一遍。
//
// TODO(D4 主练习)：实现我。
func (e Employee) Label() string {
	return e.Name
}

// Raise 给员工加薪 amount 元。
//
// TODO(D4 主练习)：实现我。
//
// 【必须用指针接收者】—— 值接收者拿到的是拷贝，改了等于没改。
// 顺带一提，真写成值接收者的话 golangci-lint 会直接报
// SA4005: ineffective assignment to field，这个经典 bug 有工具兜底。
//
// 它同样会被提升：mgr.Raise(500) 实际调用 (&mgr.Employee).Raise(500)，
// 前提是 mgr 可寻址（D2 §2.1.3）。
func (e *Employee) Raise(amount int) {
	e.BaseSalary += amount
}

// Manager 是管理岗。
//
// 薪酬 = 底薪 + 底薪/5 + 团队人数 × 100（整数除法，向零截断）。
type Manager struct {
	Employee
	// TeamSize 是直接下属人数。
	TeamSize int
}

// MonthlyPay 实现 Payer。
//
// TODO(D4 主练习)：实现我。
func (m Manager) MonthlyPay() int {
	return m.BaseSalary + m.BaseSalary/5 + m.TeamSize*100
}

// Engineer 是技术岗。
//
// 薪酬 = 底薪 + 职级 × 500。
type Engineer struct {
	Employee
	// Level 是职级，从 1 开始。
	Level int
}

// MonthlyPay 实现 Payer。
//
// TODO(D4 主练习)：实现我。
func (e Engineer) MonthlyPay() int {
	return e.BaseSalary + e.Level*500
}

// Contractor 是外包，按小时计费，没有底薪，**不嵌入 Employee**。
//
// 薪酬 = 工时 × 时薪。
//
// 这是这道题的重点。在 Java 里想让它进同一个薪酬列表，你得要么让它继承
// Employee（然后被迫背上一个用不上的 baseSalary，还得想办法让 bonus() 返回
// 些什么），要么在 Employee 之上再抽一层接口、改动整个层次。
//
// Go 里它什么都不用继承 —— 只要碰巧有 MonthlyPay 和 Label 两个方法，
// 它就已经是 Payer 了。
type Contractor struct {
	// Name 是外包人员或供应商名称。
	Name string
	// Hours 是本月工时。
	Hours int
	// HourlyRate 是时薪，单位元。
	HourlyRate int
}

// MonthlyPay 实现 Payer。
//
// TODO(D4 主练习)：实现我。
func (c Contractor) MonthlyPay() int {
	return c.Hours * c.HourlyRate
}

// Label 实现 Payer。
//
// 格式固定为：`张三（外包）`
//
// TODO(D4 主练习)：实现我。
func (c Contractor) Label() string {
	return fmt.Sprintf("%s（外包）", c.Name)
}

// TotalPayroll 返回一组 Payer 的薪酬总额。
//
// TODO(D4 主练习)：实现我。
//
// 注意参数类型是 []Payer 而不是 []Employee —— 这就是「面向接口」：
// 函数只依赖它真正需要的能力，不依赖具体类型，更不依赖继承层次。
func TotalPayroll(ps []Payer) int {
	total := 0
	for _, p := range ps {
		total += p.MonthlyPay()
	}
	return total
}
