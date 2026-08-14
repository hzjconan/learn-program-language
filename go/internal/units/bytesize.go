package units

import "fmt"

// ByteSize 是以字节为单位的数据量。
//
// 底层用 int64 而不是 uint64：Go 的惯例是「除非真的需要表示 2^63 以上的数，
// 或者在做位运算/二进制协议，否则一律用有符号类型」。
// 无符号类型的减法下溢（1 - 2 变成一个天文数字）比它省下的那一位值钱得多。
type ByteSize int64

// 数据量单位，每档 1024 倍。
//
// 【已完成 · 小题 A-1】原先这组常量是手写死值，要求用 iota 重写，
// 让每一行的值都由上一行推导出来，且 TestUnitConstants 仍然通过。
//
// 手写死值的问题不是「打字多」，是「PB 那一行你敢保证数对了吗」——
// 让编译器算，人不算，这是 iota 存在的理由。
//
// 两个易错点（都踩过）：省略右侧时重复的是上一行的**完整表达式，连类型一起**，
// 所以 ByteSize 只写一次；而中途换一个不同的表达式（如 B = 1 + iota）
// 会让该行退化成无类型常量，白白丢掉类型安全。
const (
	// B 是 1 字节。
	B ByteSize = 1 << (10 * iota) // iota=0 → 1<<0 = 1
	// KB 是 1024 字节。
	KB // iota=1 → 1<<10
	// MB 是 1024 KB。
	MB
	// GB 是 1024 MB。
	GB
	// TB 是 1024 GB。
	TB
	// PB 是 1024 TB。
	PB
)

// 数据量单位表，从大到小排列 —— FormatBytes 靠这个顺序选出「不超过该值的最大单位」。
var units = []struct {
	size ByteSize
	name string
}{
	{PB, "PB"},
	{TB, "TB"},
	{GB, "GB"},
	{MB, "MB"},
	{KB, "KB"},
}

// FormatBytes 把字节数格式化成人类可读的字符串。
//
// 规则：
//   - 绝对值小于 1 KB 时，按整数输出并以 " B" 结尾，例如 "512 B"、"0 B"。
//   - 否则选用「不超过该数值的最大单位」，保留一位小数，例如 "1.5 KB"、"2.0 GB"。
//   - 负数在前面加 "-"，例如 "-1.5 KB"。
//
// 【已完成 · 小题 A-2】要求不写六个 if 分支硬编码，而是把单位表数据化成一个
// struct 切片，再循环选择。「用数据驱动代替分支」是 Go 里非常常见的写法，
// 和表驱动测试是同一个思路。
//
// 踩过的坑：float64(n/u.size) —— 转换写在括号外面，整数除法早就把小数截掉了。
// 正确写法是 float64(n) / float64(u.size)，**转换要贴着操作数，别贴着表达式**。
func FormatBytes(n ByteSize) string {
	sign := ""
	if n < 0 {
		n = -n
		sign = "-"
	}

	for _, u := range units {
		if n >= u.size {
			return fmt.Sprintf("%s%.1f %s", sign, float64(n)/float64(u.size), u.name)
		}
	}

	return fmt.Sprintf("%s%d B", sign, n)
}
