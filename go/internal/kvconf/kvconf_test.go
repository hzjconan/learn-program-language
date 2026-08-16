package kvconf

import (
	"errors"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string]string
	}{
		{
			name: "基本",
			in:   "host=localhost\nport=8080\n",
			want: map[string]string{"host": "localhost", "port": "8080"},
		},
		{
			name: "跳过空行和注释",
			in:   "# 数据库配置\n\nhost=localhost\n   \n# 端口\nport=8080\n",
			want: map[string]string{"host": "localhost", "port": "8080"},
		},
		{
			name: "缩进的注释也算注释",
			in:   "   # 这是注释\nhost=localhost\n",
			want: map[string]string{"host": "localhost"},
		},
		{
			name: "key 和 value 两侧空白被裁掉",
			in:   "  host  =  localhost  \n",
			want: map[string]string{"host": "localhost"},
		},
		{
			name: "value 允许为空",
			in:   "password=\n",
			want: map[string]string{"password": ""},
		},
		{
			name: "只按第一个等号分割",
			in:   "url=http://x/?a=1&b=2\n",
			want: map[string]string{"url": "http://x/?a=1&b=2"},
		},
		{
			name: "value 里的 # 不是注释",
			in:   "color=#ff0000\n",
			want: map[string]string{"color": "#ff0000"},
		},
		{
			name: "末尾没有换行",
			in:   "host=localhost",
			want: map[string]string{"host": "localhost"},
		},
		{
			name: "空输入返回空 map 而不是 nil 报错",
			in:   "",
			want: map[string]string{},
		},
		{
			name: "全是注释",
			in:   "# a\n# b\n",
			want: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("Parse(%q) 返回错误 %v, want nil", tt.in, err)
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("Parse(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantErr  error  // 期望能被 errors.Is 匹配到的哨兵错误
		wantLine int    // 期望的行号（从 1 开始，含被跳过的行）
		wantText string // 期望的原始行内容
	}{
		{
			name:     "没有等号",
			in:       "host=localhost\n\n# 注释\nbad line\n",
			wantErr:  ErrMissingSeparator,
			wantLine: 4,
			wantText: "bad line",
		},
		{
			name:     "键为空",
			in:       "host=localhost\n=8080\n",
			wantErr:  ErrEmptyKey,
			wantLine: 2,
			wantText: "=8080",
		},
		{
			name:     "键只有空白也算空",
			in:       "   =8080\n",
			wantErr:  ErrEmptyKey,
			wantLine: 1,
			wantText: "   =8080",
		},
		{
			name:     "键重复",
			in:       "host=a\nport=1\nhost=b\n",
			wantErr:  ErrDuplicateKey,
			wantLine: 3,
			wantText: "host=b",
		},
		{
			name:     "trim 后重复也算重复",
			in:       "host=a\n  host  =b\n",
			wantErr:  ErrDuplicateKey,
			wantLine: 2,
			wantText: "  host  =b",
		},
		{
			name:     "第一个错误立即返回",
			in:       "bad1\nbad2\n",
			wantErr:  ErrMissingSeparator,
			wantLine: 1,
			wantText: "bad1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(strings.NewReader(tt.in))
			if err == nil {
				t.Fatalf("Parse(%q) = %v, nil，want 错误", tt.in, got)
			}
			if got != nil {
				t.Errorf("出错时第一个返回值 = %v, want nil", got)
			}

			// errors.Is 要能穿透 *ParseError 找到哨兵错误 —— 靠的是 Unwrap 方法。
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("errors.Is(err, %v) = false, err = %v（Unwrap 实现了吗？）", tt.wantErr, err)
			}

			// errors.As 要能取出结构化数据。注意第二个参数是 **ParseError。
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("errors.As 取不到 *ParseError, err = %v (%T)", err, err)
			}
			if pe.Line != tt.wantLine {
				t.Errorf("ParseError.Line = %d, want %d", pe.Line, tt.wantLine)
			}
			if pe.Text != tt.wantText {
				t.Errorf("ParseError.Text = %q, want %q", pe.Text, tt.wantText)
			}
		})
	}
}

func TestParseErrorMessage(t *testing.T) {
	e := &ParseError{Line: 3, Text: "abc", Err: ErrMissingSeparator}
	want := `第 3 行: 缺少 '=' 分隔符 (原文: "abc")`
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q,\nwant %q", got, want)
	}
}

// failingReader 是一个永远读失败的 io.Reader。
//
// 这是 Go 测试里最常见的打桩手法：需要什么行为，就地定义一个满足接口的小类型。
// 不需要 mock 框架 —— 接口是隐式实现的，D5 会讲透。
type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestParseReadError(t *testing.T) {
	sentinel := errors.New("磁盘炸了")

	got, err := Parse(failingReader{err: sentinel})
	if err == nil {
		t.Fatalf("Parse = %v, nil，want 错误", got)
	}
	if got != nil {
		t.Errorf("出错时第一个返回值 = %v, want nil", got)
	}

	// 读取错误要用 %w 包装传上来，调用方才能识别底层原因。
	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false, err = %v（用 %%w 包装了吗？）", err)
	}

	// 读取失败不是语法错误，不该伪装成 *ParseError。
	var pe *ParseError
	if errors.As(err, &pe) {
		t.Errorf("读取错误不应该是 *ParseError, got %v", pe)
	}
}

func TestParseFile(t *testing.T) {
	// t.TempDir 会在测试结束后自动清理，不需要自己写 defer os.RemoveAll。
	dir := t.TempDir()
	path := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(path, []byte("# 配置\nhost=localhost\nport=8080\n"), 0o600); err != nil {
		t.Fatalf("准备测试文件失败: %v", err)
	}

	got, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile(%q) 返回错误 %v, want nil", path, err)
	}
	want := map[string]string{"host": "localhost", "port": "8080"}
	if !maps.Equal(got, want) {
		t.Errorf("ParseFile = %v, want %v", got, want)
	}
}

func TestParseFileNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "不存在.conf")

	got, err := ParseFile(path)
	if err == nil {
		t.Fatalf("ParseFile = %v, nil，want 错误", got)
	}

	// ⭐ 今天最关键的一条断言：
	// 错误链要一路 %w 到 os.Open 返回的 *fs.PathError，
	// 中间任何一层用了 %v，这里就会挂。
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("errors.Is(err, fs.ErrNotExist) = false, err = %v\n"+
			"（包装用的是 %%w 还是 %%v？）", err)
	}

	// 错误消息要带上路径，否则调用方不知道是哪个文件。
	if !strings.Contains(err.Error(), "不存在.conf") {
		t.Errorf("错误消息里没有文件名: %v", err)
	}
}

func TestParseFileParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.conf")
	if err := os.WriteFile(path, []byte("host=localhost\n\nbad line\n"), 0o600); err != nil {
		t.Fatalf("准备测试文件失败: %v", err)
	}

	_, err := ParseFile(path)
	if err == nil {
		t.Fatal("ParseFile 返回 nil，want 错误")
	}

	// 包装了一层文件上下文之后，errors.As 仍然要能取到里面的 *ParseError。
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("errors.As 取不到 *ParseError, err = %v", err)
	}
	if pe.Line != 3 {
		t.Errorf("ParseError.Line = %d, want 3", pe.Line)
	}

	// 哨兵错误也要能穿透两层包装。
	if !errors.Is(err, ErrMissingSeparator) {
		t.Errorf("errors.Is(err, ErrMissingSeparator) = false, err = %v", err)
	}

	if !strings.Contains(err.Error(), "broken.conf") {
		t.Errorf("错误消息里没有文件名: %v", err)
	}
}

// 编译期断言：*ParseError 必须实现 error 接口。
//
// 这行代码不产生任何运行时开销，纯粹让编译器帮你检查。
// 标准库里到处是这种写法，看到 `var _ Interface = (*Type)(nil)` 就是它。
var _ error = (*ParseError)(nil)

// 确保 io 被使用（failingReader 实现的是 io.Reader）。
var _ io.Reader = failingReader{}
