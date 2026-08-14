// Command hello 是环境验证程序：确认工具链版本、架构与新语法特性可用。
package main

import (
	"fmt"
	"runtime"
)

func main() {
	fmt.Println(greet("Go"))
	fmt.Printf("toolchain: %s %s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func greet(name string) string {
	return fmt.Sprintf("Hello, %s!", name)
}
