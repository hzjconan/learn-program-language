// Package config 从环境变量加载服务配置（D12 §2，十二要素应用）。
//
// 设计要点：
//
//   - 默认值写在代码里，没有任何环境变量时 Load 也要返回一份能跑的配置
//   - 启动时一次性校验，而不是等到用的时候才发现端口是 0
//   - 业务代码【不】直接读 os.Getenv，全部集中在这里
//   - 敏感字段不能被打进日志
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Lookup 是查环境变量的函数，签名和 os.LookupEnv 一致。
//
// ⭐ 把它做成参数而不是直接调 os.LookupEnv，是为了【可测试】：
// 测试可以传一个假的 map，不用 t.Setenv 去动全局状态，
// 于是这些测试还能 t.Parallel()（D6）。
type Lookup func(key string) (value string, ok bool)

// Secret 包住密码、token 这类不能出现在日志里的值。
//
// ⚠️ 见 D12 §2：一个 String() 挡不住所有路径，四条路要分别堵。
type Secret string

// String 实现 fmt.Stringer，管住 %v / %s。
//
// TODO(D12)：实现我 —— 返回 Redacted。
func (s Secret) String() string {
	return Redacted
}

// LogValue 实现 slog.LogValuer，是 slog 的正规入口。
//
// TODO(D12)：实现我 —— 返回 slog.StringValue(Redacted)。
func (s Secret) LogValue() slog.Value {
	return slog.StringValue(Redacted)
}

// MarshalJSON 管住 encoding/json。
//
// TODO(D12)：实现我 —— 序列化成 Redacted 这个字符串。
//
// ⚠️ encoding/json 只认 json.Marshaler，String() 对它完全无效。
// 而 slog 的 JSON handler 在找不到 LogValuer 时会退回 json.Marshal ——
// 所以少了这个方法，换个 Handler 就开始泄漏（D12 §2 的表）。
func (s Secret) MarshalJSON() ([]byte, error) {
	return json.Marshal(Redacted)
}

// Reveal 返回真实值。⚠️ 只在真正要用的地方调用（比如拼请求头）。
func (s Secret) Reveal() string { return string(s) }

// Redacted 是脱敏后的占位串。
const Redacted = "[REDACTED]"

// Config 是服务的全部配置。
type Config struct {
	// Addr 是监听地址，如 ":8080"。
	Addr string
	// ReadTimeout / WriteTimeout 是 HTTP 服务端超时（D11 §8）。
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	// ShutdownTimeout 是优雅关闭的等待上限（D10 §8）。
	ShutdownTimeout time.Duration
	// MaxBodyBytes 是请求体大小上限（D11 §6）。
	MaxBodyBytes int64
	// LogLevel 是日志级别。
	LogLevel slog.Level
	// LogFormat 是 "text"（开发）或 "json"（生产）。
	LogFormat string
	// APIToken 是访问 API 需要的 token。空表示不鉴权。
	APIToken Secret
}

// 默认值。⭐ 没有任何环境变量时，Load 返回的就是这一份，且它必须能跑。
const (
	DefaultAddr            = ":8080"
	DefaultReadTimeout     = 5 * time.Second
	DefaultWriteTimeout    = 10 * time.Second
	DefaultShutdownTimeout = 15 * time.Second
	DefaultMaxBodyBytes    = 1 << 20 // 1MB
	DefaultLogFormat       = "text"
)

// LogValue 实现 slog.LogValuer，控制整个 Config 记进日志时的样子。
//
// TODO(D12)：实现我。
//
// 返回一个 slog.GroupValue，字段名【必须是 snake_case】：
// addr / read_timeout / write_timeout / shutdown_timeout /
// max_body_bytes / log_level / log_format / api_token。
//
// ⚠️ 两个理由：
//
//  1. 日志字段名要和其余日志保持一致。不实现的话，slog 会退回反射序列化，
//     字段名变成 Go 的字段名（Addr、APIToken），跟你手写的那些 attr 对不上，
//     日志平台上没法统一查询（review 关注点：可观测性）。
//  2. LogValue【不向下递归】—— slog 只在最外层的值上找 LogValuer。
//     Config 不实现的话，内层 Secret.LogValue 根本不会被调用，
//     脱敏就只能指望 Secret.MarshalJSON / String 兜着（D12 §2 的表）。
//     少写一个方法就漏一条路，与其记住哪条路安全，不如都堵上。
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("addr", c.Addr),
		slog.Duration("read_timeout", c.ReadTimeout),
		slog.Duration("write_timeout", c.WriteTimeout),
		slog.Duration("shutdown_timeout", c.ShutdownTimeout),
		slog.Int64("max_body_bytes", c.MaxBodyBytes),
		slog.String("log_level", c.LogLevel.String()),
		slog.String("log_format", c.LogFormat),
		slog.String("api_token", c.APIToken.String()),
	)
}

// Load 从 lookup 读取配置，缺失的项用默认值，最后校验。
//
// TODO(D12)：实现我。
//
// 环境变量名 → 字段：
//
//	ADDR              → Addr              （字符串）
//	READ_TIMEOUT      → ReadTimeout       （time.ParseDuration，如 "5s"）
//	WRITE_TIMEOUT     → WriteTimeout      （同上）
//	SHUTDOWN_TIMEOUT  → ShutdownTimeout   （同上）
//	MAX_BODY_BYTES    → MaxBodyBytes      （strconv.ParseInt）
//	LOG_LEVEL         → LogLevel          （"debug"/"info"/"warn"/"error"，大小写不敏感）
//	LOG_FORMAT        → LogFormat         （"text" 或 "json"）
//	API_TOKEN         → APIToken
//
// 硬要求：
//
//   - **lookup 为 nil 时用 os.LookupEnv** —— 让 Load(nil) 就是「读真实环境」
//   - **区分「没设」和「设成空」**：ok == false 才用默认值；
//     显式设成空字符串是用户的选择，要么接受要么报错，但不能悄悄换成默认值。
//     ⚠️ 想清楚每个字段哪种更合理 —— ADDR="" 和 API_TOKEN="" 的含义并不一样。
//   - **解析失败要报错**，并且错误信息里要带上【是哪个变量】和【它的值】，
//     否则线上只看到 "invalid duration" 根本不知道改哪个（review 关注点：可观测性）
//     ⚠️ 但 API_TOKEN 解析失败时【不能】把值打出来
//   - **返回前调用 Validate()**
func Load(lookup Lookup) (Config, error) {
	if lookup == nil {
		lookup = os.LookupEnv
	}

	var errs []error
	c := Config{
		Addr:            DefaultAddr,
		ReadTimeout:     DefaultReadTimeout,
		WriteTimeout:    DefaultWriteTimeout,
		ShutdownTimeout: DefaultShutdownTimeout,
		MaxBodyBytes:    DefaultMaxBodyBytes,
		LogLevel:        slog.LevelInfo,
		LogFormat:       DefaultLogFormat,
	}

	if v, ok := lookup("ADDR"); ok {
		c.Addr = v
	}

	if v, ok := lookup("READ_TIMEOUT"); ok {
		t, e := time.ParseDuration(v)
		if e != nil {
			errs = append(errs, fmt.Errorf("READ_TIMEOUT %v 无效: %w", v, e))
		} else {
			c.ReadTimeout = t
		}
	}

	if v, ok := lookup("WRITE_TIMEOUT"); ok {
		t, e := time.ParseDuration(v)
		if e != nil {
			errs = append(errs, fmt.Errorf("WRITE_TIMEOUT %v 无效: %w", v, e))
		} else {
			c.WriteTimeout = t
		}
	}

	if v, ok := lookup("SHUTDOWN_TIMEOUT"); ok {
		t, e := time.ParseDuration(v)
		if e != nil {
			errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT %v 无效: %w", v, e))
		} else {
			c.ShutdownTimeout = t
		}
	}

	if v, ok := lookup("MAX_BODY_BYTES"); ok {
		t, e := strconv.ParseInt(v, 10, 64)
		if e != nil {
			errs = append(errs, fmt.Errorf("MAX_BODY_BYTES %v 无效: %w", v, e))
		} else {
			c.MaxBodyBytes = t
		}
	}

	if v, ok := lookup("LOG_LEVEL"); ok {
		switch strings.ToLower(v) {
		case "debug":
			c.LogLevel = slog.LevelDebug
		case "info":
			c.LogLevel = slog.LevelInfo
		case "warn":
			c.LogLevel = slog.LevelWarn
		case "error":
			c.LogLevel = slog.LevelError
		default:
			errs = append(errs, fmt.Errorf("LOG_LEVEL=%q: 未知级别（debug/info/warn/error）", v))
		}
	}

	if v, ok := lookup("LOG_FORMAT"); ok {
		switch v {
		case "text", "json":
			c.LogFormat = v
		default:
			errs = append(errs, fmt.Errorf("LOG_FORMAT=%q: 未知格式（text/json）", v))
		}
	}

	if v, ok := lookup("API_TOKEN"); ok {
		c.APIToken = Secret(v)
	}

	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}

	if err := c.Validate(); err != nil {
		return Config{}, err
	}

	return c, nil
}

// Validate 检查配置是否自洽。
//
// TODO(D12)：实现我。
//
// 至少检查：
//
//   - Addr 非空
//   - 三个 timeout 都 > 0
//   - MaxBodyBytes > 0
//   - LogFormat 是 "text" 或 "json"
//   - ⭐ WriteTimeout > ReadTimeout（想想 D11 §8：WriteTimeout 从【读到请求头】
//     就开始计时，比 ReadTimeout 短的话慢请求会被莫名其妙掐断）
//
// 用 errors.Join 把所有问题一次性报出来 —— 比「改一个报一个」友好得多。
//
// ⚠️ 错误信息里要带上是【哪个字段】出了问题，但注意 lint 的 ST1005：
// error 字符串不许首字母大写。
//
//	errors.New("Addr 不能为空")        ← 被 staticcheck 挡下
//	errors.New("配置项 Addr 不能为空")  ← 可以
//
// 有意思的是 "ReadTimeout 必须为正" 【不会】被挡 —— ST1005 对含内部大写的
// 标识符（ReadTimeout、MaxBodyBytes）有豁免，而 Addr 看起来就是个普通单词。
func (c Config) Validate() error {
	var errs []error

	if c.Addr == "" {
		errs = append(errs, errors.New("配置项 Addr 不能为空"))
	}
	if c.ReadTimeout <= 0 {
		errs = append(errs, errors.New("配置项 ReadTimeout 必须为正"))
	}
	if c.WriteTimeout <= 0 {
		errs = append(errs, errors.New("配置项 WriteTimeout 必须为正"))
	}
	if c.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("配置项 ShutdownTimeout 必须为正"))
	}
	if c.MaxBodyBytes <= 0 {
		errs = append(errs, errors.New("配置项 MaxBodyBytes 必须为正"))
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		errs = append(errs, fmt.Errorf("配置项 LogFormat=%q 必须是 text 或 json", c.LogFormat))
	}
	// D11 §8：WriteTimeout 从读到请求头就开始计时，比 ReadTimeout 短的话慢请求被掐断
	if c.WriteTimeout <= c.ReadTimeout {
		errs = append(errs, errors.New("配置项 WriteTimeout 必须大于 ReadTimeout"))
	}

	return errors.Join(errs...)
}

// NewLogger 按配置造一个 logger。
//
// TODO(D12)：实现我。
//
//   - LogFormat == "json" → slog.NewJSONHandler，否则 slog.NewTextHandler
//   - Level 用 c.LogLevel
//   - 写到传入的 w
func NewLogger(c Config, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.LogLevel}
	var h slog.Handler
	if c.LogFormat == "json" {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h)
}
