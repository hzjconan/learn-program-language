// Package buildinfo 回答「线上跑的这个二进制是谁、从哪来」（D18 §2）。
//
// 两个来源：
//
//  1. Version：构建时用 -ldflags "-X ...buildinfo.Version=v1.2.3" 注入。不注入就是 "dev"
//  2. Revision / Time / Modified：Go 1.18+ 的 go build 会自动从 .git 读出来，存进二进制，
//     运行时用 runtime/debug.ReadBuildInfo() 取。⚠️ 但 Docker 构建时没有 .git（§2.3），这几个字段是空的
package buildinfo

import "runtime/debug"

// Version 由 -ldflags -X 注入。⚠️ 必须是包级的 string 变量（不能是 const，也不能是函数调用的结果），-X 才改得了它。
var Version = "dev"

// Revision 由 -ldflags -X 注入；为空时回退到 Go 自动记录的提交号。
var Revision = ""

// Info 是一次构建的身份信息。
type Info struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`  // git 提交号
	Time      string `json:"time"`      // 提交时间
	Modified  bool   `json:"modified"`  // 构建时工作区是否有未提交的改动
	GoVersion string `json:"goVersion"` // 编译用的 Go 版本
}

// Get 返回当前二进制的构建信息。
//
//   - Version 直接用包级变量
//   - 用 debug.ReadBuildInfo() 拿到 *debug.BuildInfo（ok 为 false 时就是拿不到，比如 go run 的某些情况）
//   - bi.GoVersion 是 Go 版本
//   - bi.Settings 是 []debug.BuildSetting，Key 是 "vcs.revision"、"vcs.time"、"vcs.modified"
//     （本机 go version -m 二进制 能看到同样的内容）
//
// ⚠️ 拿不到的字段留空字符串，别编一个值出来：「不知道」要显示成不知道（D16 review 关注点：不撒谎）。
func Get() Info {
	info := Info{Version: Version, Revision: Revision}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}

	info.GoVersion = bi.GoVersion

	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if Revision == "" {
				info.Revision = s.Value
			}
		case "vcs.time":
			info.Time = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}

	return info
}
