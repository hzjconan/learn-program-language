package config_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/hzjconan/learn-program-language/go/internal/config"
)

// tokenValue 是脱敏测试的探针 —— 它绝不能出现在任何输出里。
const tokenValue = "sk-live-51H8xQ2eZvKYlo"

// env 把 map 变成 Lookup，测试里就不用碰真实环境变量了。
func env(m map[string]string) config.Lookup {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

// ---------- 默认值 ----------

// TestConfig_DefaultsAreUsable 锁住「什么都不设也能跑」。
func TestConfig_DefaultsAreUsable(t *testing.T) {
	t.Parallel()

	c, err := config.Load(env(nil))
	if err != nil {
		t.Fatalf("没有任何环境变量时 Load 就失败了: %v\n（默认值必须自成一份可用配置）", err)
	}
	if c.Addr != config.DefaultAddr {
		t.Errorf("Addr = %q, want %q", c.Addr, config.DefaultAddr)
	}
	if c.ReadTimeout != config.DefaultReadTimeout {
		t.Errorf("ReadTimeout = %v, want %v", c.ReadTimeout, config.DefaultReadTimeout)
	}
	if c.WriteTimeout != config.DefaultWriteTimeout {
		t.Errorf("WriteTimeout = %v, want %v", c.WriteTimeout, config.DefaultWriteTimeout)
	}
	if c.MaxBodyBytes != config.DefaultMaxBodyBytes {
		t.Errorf("MaxBodyBytes = %d, want %d", c.MaxBodyBytes, config.DefaultMaxBodyBytes)
	}
	if c.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want Info", c.LogLevel)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("默认配置没通过自己的校验: %v", err)
	}
}

// ---------- 覆盖 ----------

func TestConfig_EnvOverrides(t *testing.T) {
	t.Parallel()

	c, err := config.Load(env(map[string]string{
		"ADDR":             ":9999",
		"READ_TIMEOUT":     "3s",
		"WRITE_TIMEOUT":    "30s",
		"SHUTDOWN_TIMEOUT": "1m",
		"MAX_BODY_BYTES":   "2048",
		"LOG_LEVEL":        "debug",
		"LOG_FORMAT":       "json",
		"API_TOKEN":        tokenValue,
	}))
	if err != nil {
		t.Fatalf("Load 失败: %v", err)
	}

	// ⚠️ 每个字段都单独断言 —— 只测一两个的话，「把 READ_TIMEOUT 赋给了
	// WriteTimeout」这种复制粘贴 bug 照样能溜过去。
	checks := []struct {
		name      string
		got, want any
	}{
		{"Addr", c.Addr, ":9999"},
		{"ReadTimeout", c.ReadTimeout, 3 * time.Second},
		{"WriteTimeout", c.WriteTimeout, 30 * time.Second},
		{"ShutdownTimeout", c.ShutdownTimeout, time.Minute},
		{"MaxBodyBytes", c.MaxBodyBytes, int64(2048)},
		{"LogLevel", c.LogLevel, slog.LevelDebug},
		{"LogFormat", c.LogFormat, "json"},
		{"APIToken", c.APIToken.Reveal(), tokenValue},
	}
	for _, ck := range checks {
		if ck.got != ck.want {
			t.Errorf("%s = %v, want %v", ck.name, ck.got, ck.want)
		}
	}
}

func TestConfig_LogLevelCaseInsensitive(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"debug", "DEBUG", "Debug"} {
		c, err := config.Load(env(map[string]string{"LOG_LEVEL": in}))
		if err != nil {
			t.Errorf("LOG_LEVEL=%q → %v", in, err)
			continue
		}
		if c.LogLevel != slog.LevelDebug {
			t.Errorf("LOG_LEVEL=%q → %v, want Debug（大小写不敏感）", in, c.LogLevel)
		}
	}
}

// ---------- 解析错误 ----------

// TestConfig_ParseErrorsAreDiagnosable 检查错误信息够不够查问题。
//
// 「invalid duration」这种信息在线上等于没有 —— 你不知道是哪个变量。
// （review 关注点：可观测性）
func TestConfig_ParseErrorsAreDiagnosable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		vars    map[string]string
		wantVar string // 错误信息里必须出现的变量名
		wantVal string // 以及出问题的值
	}{
		{"超时格式错", map[string]string{"READ_TIMEOUT": "5 seconds"}, "READ_TIMEOUT", "5 seconds"},
		{"字节数不是数字", map[string]string{"MAX_BODY_BYTES": "1MB"}, "MAX_BODY_BYTES", "1MB"},
		{"未知日志级别", map[string]string{"LOG_LEVEL": "verbose"}, "LOG_LEVEL", "verbose"},
		{"未知日志格式", map[string]string{"LOG_FORMAT": "xml"}, "LOG_FORMAT", "xml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := config.Load(env(tt.vars))
			if err == nil {
				t.Fatalf("%v 应该报错，但 Load 成功了\n（解析失败不能静默用默认值 —— D12 §1.3 同一个道理）", tt.vars)
			}
			msg := err.Error()
			if !strings.Contains(msg, tt.wantVar) {
				t.Errorf("错误信息里没有变量名 %q，线上无法定位:\n  %s", tt.wantVar, msg)
			}
			if !strings.Contains(msg, tt.wantVal) {
				t.Errorf("错误信息里没有出错的值 %q:\n  %s", tt.wantVal, msg)
			}
		})
	}
}

// TestConfig_TokenNotInParseError 单独拎出来：token 出问题时不能把值打出来。
func TestConfig_TokenNotInParseError(t *testing.T) {
	t.Parallel()

	// 造一个会失败的配置，同时带上 token
	_, err := config.Load(env(map[string]string{
		"READ_TIMEOUT": "bad",
		"API_TOKEN":    tokenValue,
	}))
	if err == nil {
		t.Fatal("应该报错")
	}
	if strings.Contains(err.Error(), tokenValue) {
		t.Errorf("错误信息里泄漏了 API_TOKEN:\n  %s", err.Error())
	}
}

// ---------- 校验 ----------

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	base := func() config.Config {
		c, _ := config.Load(env(nil))
		return c
	}

	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr bool
	}{
		{"默认值", func(*config.Config) {}, false},
		{"Addr 为空", func(c *config.Config) { c.Addr = "" }, true},
		{"ReadTimeout 为 0", func(c *config.Config) { c.ReadTimeout = 0 }, true},
		{"ReadTimeout 为负", func(c *config.Config) { c.ReadTimeout = -time.Second }, true},
		{"WriteTimeout 为 0", func(c *config.Config) { c.WriteTimeout = 0 }, true},
		{"ShutdownTimeout 为 0", func(c *config.Config) { c.ShutdownTimeout = 0 }, true},
		{"MaxBodyBytes 为 0", func(c *config.Config) { c.MaxBodyBytes = 0 }, true},
		{"MaxBodyBytes 为负", func(c *config.Config) { c.MaxBodyBytes = -1 }, true},
		{"LogFormat 非法", func(c *config.Config) { c.LogFormat = "yaml" }, true},
		// ⭐ D11 §8：WriteTimeout 从读到请求头就开始计时
		{"WriteTimeout 比 ReadTimeout 短", func(c *config.Config) {
			c.ReadTimeout = 10 * time.Second
			c.WriteTimeout = 5 * time.Second
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := base()
			tt.mutate(&c)
			err := c.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

// TestConfig_ValidateReportsAllProblems 锁住 errors.Join 的用法。
//
// 一次只报一个问题的话，改配置要来回试五轮。
func TestConfig_ValidateReportsAllProblems(t *testing.T) {
	t.Parallel()

	c, _ := config.Load(env(nil))
	c.Addr = ""
	c.ReadTimeout = 0
	c.MaxBodyBytes = 0
	c.LogFormat = "yaml"

	err := c.Validate()
	if err == nil {
		t.Fatal("应该报错")
	}
	msg := err.Error()
	for _, want := range []string{"Addr", "ReadTimeout", "MaxBodyBytes", "LogFormat"} {
		if !strings.Contains(msg, want) {
			t.Errorf("四个问题应该【一次性】全报出来，缺少 %q:\n%s\n"+
				"（用 errors.Join 收集，别第一个问题就 return）", want, msg)
		}
	}
}

// ---------- 脱敏（今天最重要的一组） ----------

// TestConfig_SecretRedaction 检查 Secret 的三条出口。
func TestConfig_SecretRedaction(t *testing.T) {
	t.Parallel()

	s := config.Secret(tokenValue)

	t.Run("fmt %v 和 %s", func(t *testing.T) {
		for _, verb := range []string{"%v", "%s"} {
			got := fmt.Sprintf(verb, s)
			if strings.Contains(got, tokenValue) {
				t.Errorf("%s 泄漏了: %s\n（实现 String() string）", verb, got)
			}
			if got != config.Redacted {
				t.Errorf("%s = %q, want %q", verb, got, config.Redacted)
			}
		}
	})

	t.Run("encoding/json", func(t *testing.T) {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if strings.Contains(string(b), tokenValue) {
			t.Errorf("json.Marshal 泄漏了: %s\n"+
				"（⚠️ encoding/json 【不走】String()，要实现 MarshalJSON）", b)
		}
	})

	t.Run("slog", func(t *testing.T) {
		var buf bytes.Buffer
		l := slog.New(slog.NewJSONHandler(&buf, nil))
		l.Info("x", "token", s)
		if strings.Contains(buf.String(), tokenValue) {
			t.Errorf("slog 泄漏了: %s\n"+
				"（⚠️ slog 【不走】String()，要实现 LogValue() slog.Value）", buf.String())
		}
	})

	t.Run("Reveal 仍然拿得到真值", func(t *testing.T) {
		if s.Reveal() != tokenValue {
			t.Errorf("Reveal() = %q, want %q（脱敏不能把值弄丢）", s.Reveal(), tokenValue)
		}
	})
}

// TestConfig_SecretImplementsLogValuer 直接断言接口满足，而不是断言行为。
//
// ⚠️ 为什么这条不像上面那样测「有没有泄漏」？因为测不出来 ——
// String() + MarshalJSON() 已经盖住了全部输出路径（D12 §2 的表），
// 把 LogValue 删掉也【不会】产生任何可观测的泄漏。
//
// 但它仍然该实现：LogValue 是 slog 的正规入口，不走反射所以更快，
// 而且自定义 Handler 只认它。这种「有要求但没有可观测差异」的情况，
// 就直接断言接口 —— 别为了凑一个行为断言去编造场景。
func TestConfig_SecretImplementsLogValuer(t *testing.T) {
	t.Parallel()

	var v any = config.Secret("x")
	if _, ok := v.(slog.LogValuer); !ok {
		t.Error("Secret 应该实现 slog.LogValuer（LogValue() slog.Value）")
	}
	if _, ok := v.(fmt.Stringer); !ok {
		t.Error("Secret 应该实现 fmt.Stringer")
	}
	if _, ok := v.(json.Marshaler); !ok {
		t.Error("Secret 应该实现 json.Marshaler")
	}
}

// TestConfig_WholeConfigRedaction 检查「把整个 Config 记进日志」这条路。
//
// ⚠️ 实测（D12 §2 的表）：LogValue 【不向下递归】—— slog 只在最外层的值上
// 找 LogValuer。Config 自己不实现的话，它整个被丢给序列化，内层
// Secret.LogValue 根本不会被调用，只能指望 Secret 的 MarshalJSON/String
// 兜着。换个 Handler 少一个方法就漏一条路，所以两层都实现。
func TestConfig_WholeConfigRedaction(t *testing.T) {
	t.Parallel()

	c, err := config.Load(env(map[string]string{"API_TOKEN": tokenValue}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// ⭐ 两种 Handler 都要试 —— JSON handler 退回 json.Marshal（认 MarshalJSON），
	// Text handler 退回 fmt（认 String）。只测一种会漏掉另一条路。
	handlers := map[string]func(*bytes.Buffer) slog.Handler{
		"json": func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
		"text": func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
	}
	for name, mk := range handlers {
		t.Run("slog 记整个 Config/"+name, func(t *testing.T) {
			var buf bytes.Buffer
			slog.New(mk(&buf)).Info("服务启动", "cfg", c)
			if strings.Contains(buf.String(), tokenValue) {
				t.Errorf("%s handler 把整个 Config 记进日志时泄漏了:\n%s", name, buf.String())
			}
			if !strings.Contains(buf.String(), ":8080") {
				t.Errorf("脱敏不该把非敏感字段（Addr）也抹掉:\n%s", buf.String())
			}
		})
	}

	// TestConfig_LogValueFieldNames：字段名必须是 snake_case。
	//
	// 不实现 Config.LogValue 的话，slog 会用反射，字段名变成 Go 的
	// 字段名（Addr、MaxBodyBytes），和你手写的那些 attr 对不上，
	// 日志平台上没法统一查询（review 关注点：可观测性）。
	t.Run("字段名是 snake_case", func(t *testing.T) {
		var buf bytes.Buffer
		slog.New(slog.NewJSONHandler(&buf, nil)).Info("服务启动", "cfg", c)
		out := buf.String()
		for _, want := range []string{"addr", "read_timeout", "max_body_bytes", "log_format"} {
			if !strings.Contains(out, `"`+want+`"`) {
				t.Errorf("日志里缺少字段 %q（Config.LogValue 实现了吗？）:\n%s", want, out)
			}
		}
		for _, bad := range []string{`"Addr"`, `"MaxBodyBytes"`, `"APIToken"`} {
			if strings.Contains(out, bad) {
				t.Errorf("出现了 Go 字段名 %s —— 应该走 Config.LogValue 输出 snake_case:\n%s", bad, out)
			}
		}
	})

	t.Run("fmt 打整个 Config", func(t *testing.T) {
		for _, verb := range []string{"%v", "%+v"} {
			if got := fmt.Sprintf(verb, c); strings.Contains(got, tokenValue) {
				t.Errorf("%s 打整个 Config 时泄漏了: %s", verb, got)
			}
		}
		// ⚠️ %#v 堵不住 —— 它按设计就要打 Go 语法表示，绕过所有接口。
		// 这里【不】断言，只是提醒你：别用 %#v 打日志。
	})
}

// ---------- NewLogger ----------

func TestConfig_NewLogger(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format     string
		wantPrefix string
	}{
		{"json", "{"},
		{"text", "time="},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			t.Parallel()

			c, err := config.Load(env(map[string]string{"LOG_FORMAT": tt.format}))
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			var buf bytes.Buffer
			config.NewLogger(c, &buf).Info("hello")
			if !strings.HasPrefix(buf.String(), tt.wantPrefix) {
				t.Errorf("format=%q 时输出 = %q, want 以 %q 开头", tt.format, buf.String(), tt.wantPrefix)
			}
		})
	}
}

func TestConfig_NewLoggerRespectsLevel(t *testing.T) {
	t.Parallel()

	c, err := config.Load(env(map[string]string{"LOG_LEVEL": "warn", "LOG_FORMAT": "json"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var buf bytes.Buffer
	l := config.NewLogger(c, &buf)
	l.Debug("不该出现")
	l.Info("也不该出现")
	if buf.Len() != 0 {
		t.Errorf("LOG_LEVEL=warn 时 Debug/Info 不该输出，得到:\n%s\n"+
			"（HandlerOptions.Level 没设？）", buf.String())
	}
	l.Warn("该出现")
	if buf.Len() == 0 {
		t.Error("Warn 应该输出")
	}
}

// ---------- 「没设」 vs 「设成空」 ----------

// TestConfig_EmptyVsUnset 对应 D12 §1.2 —— 同一个问题换了个场景。
//
// 这条没有唯一正确答案，它锁住的是「你想清楚了并且写下来了」：
// ADDR 显式设成空是明显的配置错误，应该报错而不是悄悄换成 :8080；
// API_TOKEN 显式设成空则是合法的「关掉鉴权」。
func TestConfig_EmptyVsUnset(t *testing.T) {
	t.Parallel()

	t.Run("ADDR 显式设成空应该报错", func(t *testing.T) {
		if _, err := config.Load(env(map[string]string{"ADDR": ""})); err == nil {
			t.Error("ADDR=\"\" 时 Load 成功了\n" +
				"（用 os.LookupEnv 区分「没设」和「设成空」：\n" +
				" 没设 → 用默认值；显式设成空 → 这是配置错误，应该报出来）")
		}
	})

	t.Run("API_TOKEN 设成空是合法的（关闭鉴权）", func(t *testing.T) {
		c, err := config.Load(env(map[string]string{"API_TOKEN": ""}))
		if err != nil {
			t.Errorf("API_TOKEN=\"\" 应该合法（表示不鉴权），得到 %v", err)
		}
		if c.APIToken.Reveal() != "" {
			t.Errorf("APIToken = %q, want 空", c.APIToken.Reveal())
		}
	})
}
