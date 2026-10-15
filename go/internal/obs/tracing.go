// Package obs 是 golearn-api 的可观测性接线（D16 §7）。
//
// 只做「进程级」的一次性配置：建 TracerProvider、设全局 propagator、返回 shutdown。
// 业务代码不 import 本包 —— 它们只 import OTel 的 API 包（go.opentelemetry.io/otel），
// 拿全局 tracer 开 span。这和 slog 是同一个分层：
//
//	slog.SetDefault(...)            ↔  otel.SetTracerProvider(...)    ← main 里做一次（本包）
//	slog.InfoContext(ctx, ...)      ↔  otel.Tracer("x").Start(ctx, …) ← 业务里到处用
package obs

import (
	"context"
	"fmt"
	"io"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// SetupTracing 建一个把 span 打到 w 的 TracerProvider，并设成全局。
//
// 返回的 shutdown 必须在进程退出前调用 —— BatchSpanProcessor 攒着一批还没导出的 span，
// 不 flush 就丢了（和 D14 的优雅关闭同一个道理：缓冲区要在退出前排空）。
//
// TODO(D16) ⑥：实现我。依赖先装上（⚠️ 代理不在跑的话要 env -u HTTP_PROXY -u HTTPS_PROXY）：
//
//	go get go.opentelemetry.io/otel \
//	       go.opentelemetry.io/otel/sdk \
//	       go.opentelemetry.io/otel/exporters/stdout/stdouttrace \
//	       go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
//
// 步骤：
//
//	① exp, err := stdouttrace.New(stdouttrace.WithWriter(w))
//	② res := resource.NewSchemaless(semconv... 或 attribute.String("service.name", serviceName))
//	   ⭐ service.name 是后端（Jaeger/Tempo）按服务分组的唯一依据，不设就是 "unknown_service:api"
//	③ tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exp), sdktrace.WithResource(res))
//	④ otel.SetTracerProvider(tp)
//	⑤ otel.SetTextMapPropagator(propagation.TraceContext{})
//	   ⚠️ 不设的话默认是【空】propagator：上游传来的 traceparent 头被无视，每个服务各起一条 trace，
//	   跨服务的链就断了 —— 而且不报任何错（§7.3）。
//	⑥ return tp.Shutdown, nil
//
// 生产上 ① 换成 otlptracegrpc 发到 collector，其它不变 —— 这就是 OTel「API 和 SDK 分离」的意义。
//
// ctx 现在没用到：stdout exporter 不需要。留着是因为换成 otlptracegrpc.New(ctx, ...) 时
// 它要用 ctx 控制连接 collector 的超时和取消 —— 到时调用方不用改签名。
func SetupTracing(ctx context.Context, w io.Writer, serviceName string) (shutdown func(context.Context) error, err error) {
	// TODO(D16) ⑥
	exp, err := stdouttrace.New(stdouttrace.WithWriter(w))
	if err != nil {
		return nil, fmt.Errorf("create stdout trace exporter: %w", err)
	}

	res := resource.NewSchemaless(
		attribute.String("service.name", serviceName),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)

	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}
