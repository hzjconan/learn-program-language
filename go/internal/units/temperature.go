// Package units 提供温度与数据量的单位换算。
//
// 设计要点：每一种单位都是独立的具名类型（defined type），
// 这样「把华氏度当摄氏度传进去」这类错误在编译期就会被拦住，
// 而运行时它们仍然只是一个 float64，零额外开销。
package units

// Celsius 是摄氏温度。
type Celsius float64

// Fahrenheit 是华氏温度。
type Fahrenheit float64

// Kelvin 是开尔文温度（绝对温标）。
type Kelvin float64

// 常用温度参考点。
//
// 注意这里能直接写 -273.15 而不需要 Celsius(-273.15)：
// 字面量是「无类型常量」，赋值时才确定类型，所以不违反「无隐式转换」。
const (
	// AbsoluteZeroC 是绝对零度对应的摄氏度。
	AbsoluteZeroC Celsius = -273.15
	// FreezingC 是水的冰点。
	FreezingC Celsius = 0
	// BoilingC 是标准大气压下水的沸点。
	BoilingC Celsius = 100
)

// CToF 把摄氏度转换为华氏度。
//
// 公式：F = C*9/5 + 32
func CToF(c Celsius) Fahrenheit {
	return Fahrenheit(c*9/5 + 32)
}

// FToC 把华氏度转换为摄氏度。
//
// 公式：C = (F-32)*5/9
func FToC(f Fahrenheit) Celsius {
	return Celsius((f - 32) * 5 / 9)
}

// CToK 把摄氏度转换为开尔文。
//
// 公式：K = C + 273.15
func CToK(c Celsius) Kelvin {
	return Kelvin(c - AbsoluteZeroC)
}

// KToC 把开尔文转换为摄氏度。
//
// 公式：C = K - 273.15
func KToC(k Kelvin) Celsius {
	return Celsius(k) + AbsoluteZeroC
}

// FToK 把华氏度转换为开尔文。
//
// 提示：别重新推导公式，复用上面已有的两个函数。
// 「不重复自己」在这里还有一层意义：以后要改精度只需改一处。
func FToK(f Fahrenheit) Kelvin {
	return CToK(FToC(f))
}

// IsValidC 报告 c 是否是物理上有效的温度（不低于绝对零度）。
//
// 这是 Go 里布尔返回函数的命名惯例：Is/Has/Can 开头，
// 文档注释用 "reports whether"（中文写成「报告……是否……」）。
func IsValidC(c Celsius) bool {
	return c >= AbsoluteZeroC
}
