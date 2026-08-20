package logx

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// 测试文件是我写的，别改。你的实现要让它们全绿。

func mustTime(t *testing.T, s string) time.Time {
	t.Helper() // 失败时行号指向调用处，不是这里（D6 §4）
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("测试数据里的时间写错了 %q: %v", s, err)
	}
	return ts
}

// ---------- Level ----------

func TestLogx_LevelString(t *testing.T) {
	tests := []struct {
		in   Level
		want string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{Level(7), "Level(7)"}, // 未知值走标准库惯例，别返回空串
		{Level(-1), "Level(-1)"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("Level(%d).String() = %q, want %q", int(tt.in), got, tt.want)
		}
	}
}

// TestLogx_LevelOrdered 验证级别可以比大小 —— 这是用具名 int 而不是 string 的理由之一。
func TestLogx_LevelOrdered(t *testing.T) {
	if LevelDebug >= LevelInfo || LevelInfo >= LevelWarn || LevelWarn >= LevelError {
		t.Error("四个级别应该按严重程度递增，检查 iota 的顺序")
	}
}

func TestLogx_ParseLevel(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Level
		wantErr bool
	}{
		{name: "大写", in: "INFO", want: LevelInfo},
		{name: "小写", in: "info", want: LevelInfo},
		{name: "混合大小写", in: "WaRn", want: LevelWarn},
		{name: "DEBUG", in: "debug", want: LevelDebug},
		{name: "ERROR", in: "error", want: LevelError},
		{name: "不认识的", in: "TRACE", wantErr: true},
		{name: "空字符串", in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLevel(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseLevel(%q) 没报错，want 错误", tt.in)
				}
				// 错误消息里要带上非法输入本身，否则线上没法查（可观测性）
				if tt.in != "" && !strings.Contains(err.Error(), tt.in) {
					t.Errorf("错误消息里没带上非法输入 %q：%v", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLevel(%q) 报错: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// ---------- ParseError ----------

func TestLogx_ParseErrorMessage(t *testing.T) {
	e := &ParseError{Line: 12, Field: "latency", Err: errors.New(`无效的耗时 "abc"`)}
	want := `第 12 行 latency 字段: 无效的耗时 "abc"`
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q\nwant %q", got, want)
	}
}

func TestLogx_ParseErrorUnwrap(t *testing.T) {
	base := errors.New("底层原因")
	e := &ParseError{Line: 1, Field: "time", Err: base}

	if !errors.Is(e, base) {
		t.Error("errors.Is 穿不透 ParseError —— Unwrap 实现了吗？")
	}
}

// ---------- ParseLine ----------

func TestLogx_ParseLineOK(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Entry
	}{
		{
			name: "普通一行",
			in:   "2026-08-18T10:00:00Z INFO api-gateway 15 GET /users/42 -> 200",
			want: Entry{
				Time:    mustTime(t, "2026-08-18T10:00:00Z"),
				Level:   LevelInfo,
				Service: "api-gateway",
				Latency: 15 * time.Millisecond,
				Message: "GET /users/42 -> 200",
			},
		},
		{
			name: "消息里有多个空格，不能被切碎",
			in:   "2026-08-18T10:00:00Z WARN db 7 slow  query   here",
			want: Entry{
				Time:    mustTime(t, "2026-08-18T10:00:00Z"),
				Level:   LevelWarn,
				Service: "db",
				Latency: 7 * time.Millisecond,
				Message: "slow  query   here",
			},
		},
		{
			name: "只有四段 —— 消息为空，合法",
			in:   "2026-08-18T10:00:08Z INFO cache 2",
			want: Entry{
				Time:    mustTime(t, "2026-08-18T10:00:08Z"),
				Level:   LevelInfo,
				Service: "cache",
				Latency: 2 * time.Millisecond,
				Message: "",
			},
		},
		{
			name: "耗时为 0",
			in:   "2026-08-18T10:00:00Z DEBUG cache 0 noop",
			want: Entry{
				Time:    mustTime(t, "2026-08-18T10:00:00Z"),
				Level:   LevelDebug,
				Service: "cache",
				Latency: 0,
				Message: "noop",
			},
		},
		{
			name: "级别小写也认",
			in:   "2026-08-18T10:00:00Z error db 3 boom",
			want: Entry{
				Time:    mustTime(t, "2026-08-18T10:00:00Z"),
				Level:   LevelError,
				Service: "db",
				Latency: 3 * time.Millisecond,
				Message: "boom",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLine(1, tt.in)
			if err != nil {
				t.Fatalf("ParseLine 失败: %v", err)
			}
			if !got.Time.Equal(tt.want.Time) { // time.Time 用 Equal 不用 ==（D6 review）
				t.Errorf("Time = %v, want %v", got.Time, tt.want.Time)
			}
			if got.Level != tt.want.Level {
				t.Errorf("Level = %v, want %v", got.Level, tt.want.Level)
			}
			if got.Service != tt.want.Service {
				t.Errorf("Service = %q, want %q", got.Service, tt.want.Service)
			}
			if got.Latency != tt.want.Latency {
				t.Errorf("Latency = %v, want %v", got.Latency, tt.want.Latency)
			}
			if got.Message != tt.want.Message {
				t.Errorf("Message = %q, want %q", got.Message, tt.want.Message)
			}
		})
	}
}

func TestLogx_ParseLineErrors(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantField string
		// wantIsMalformed 为 true 时还要求 errors.Is(err, ErrMalformed)
		wantIsMalformed bool
	}{
		{name: "空行", in: "", wantField: "format", wantIsMalformed: true},
		{name: "只有一段", in: "这行整个是坏的", wantField: "format", wantIsMalformed: true},
		{name: "只有三段", in: "2026-08-18T10:00:00Z INFO db", wantField: "format", wantIsMalformed: true},
		{name: "时间不合法", in: "昨天 INFO db 1 x", wantField: "time"},
		{name: "时间格式不是 RFC3339", in: "2026/08/18 INFO db 1 x", wantField: "time"},
		{name: "级别不认识", in: "2026-08-18T10:00:00Z NOPE db 1 x", wantField: "level"},
		{name: "耗时不是数字", in: "2026-08-18T10:00:00Z INFO db abc x", wantField: "latency"},
		{name: "耗时是负数", in: "2026-08-18T10:00:00Z INFO db -5 x", wantField: "latency"},
		{name: "耗时是小数", in: "2026-08-18T10:00:00Z INFO db 1.5 x", wantField: "latency"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseLine(42, tt.in)
			if err == nil {
				t.Fatalf("ParseLine(%q) 没报错", tt.in)
			}

			var perr *ParseError
			if !errors.As(err, &perr) {
				t.Fatalf("返回的错误不是 *ParseError：%v (%T)", err, err)
			}
			if perr.Line != 42 {
				t.Errorf("perr.Line = %d, want 42（行号要如实带上，线上靠它定位）", perr.Line)
			}
			if perr.Field != tt.wantField {
				t.Errorf("perr.Field = %q, want %q", perr.Field, tt.wantField)
			}
			if perr.Err == nil {
				t.Error("perr.Err 是 nil —— 底层原因丢了，errors.Is 就没法穿透")
			}
			if tt.wantIsMalformed && !errors.Is(err, ErrMalformed) {
				t.Errorf("errors.Is(err, ErrMalformed) = false，err = %v", err)
			}

			// 错误消息里必须能看出是第几行
			if !strings.Contains(err.Error(), "42") {
				t.Errorf("错误消息里没有行号：%q", err.Error())
			}
		})
	}
}

// TestLogx_ParseLineDoesNotReturnPartialEntry 是一个设计约束：
// 失败时返回的 Entry 应该是零值，别返回填了一半的数据 —— 调用方拿到 err 之后
// 如果不小心用了那个 Entry，半截数据比零值更难查。
func TestLogx_ParseLineDoesNotReturnPartialEntry(t *testing.T) {
	got, err := ParseLine(1, "2026-08-18T10:00:00Z INFO db abc x")
	if err == nil {
		t.Fatal("这行应该解析失败")
	}
	if got != (Entry{}) {
		t.Errorf("失败时应返回零值 Entry，实际 = %+v", got)
	}
}
