// Package mathx 提供带错误处理的算术工具。
//
// 本包只有一个函数，存在的唯一目的是练习
// 「命名返回值 + defer + recover」这三件事怎么配合。
package mathx

import "fmt"

// SafeDivide 返回 a / b，除零时不 panic 而是返回错误。
//
// 【实现要求】必须用 defer + recover 捕获除零 panic，不许写 if b == 0。
//
// 先说清楚：**这不是推荐写法**。生产代码里就该老老实实
// `if b == 0 { return 0, ErrDivideByZero }`，简单一万倍，也不会掩盖真正的 bug。
// 这道题的目的是让你亲手把下面三件事焊在一起，以后读别人的 recover 代码才不懵：
//
//  1. recover() 只在 defer 调用的函数里才有效，直接写在函数体里返回 nil。
//  2. 只有【命名返回值】才能被 defer 里的闭包改到 —— 匿名返回值没有名字，碰不着。
//  3. `return a / b, nil` 不是一步：先给命名返回值赋值，再跑 defer，最后才真正返回。
//     所以 defer 里改 err 是来得及的。
//
// 错误消息格式固定为：`除以零: 5 / 0`
//
// TODO(D2 小题 B)：实现我。
func SafeDivide(a, b int) (result int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("除以零: %d / %d", a, b)
		}
	}()
	result = a / b
	return result, nil
}
