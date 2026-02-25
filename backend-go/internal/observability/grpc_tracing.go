package observability

// 文件： internal/observability/grpc_tracing.go
// 用途：支持 W3C Trace Context 传播的 gRPC 客户端/服务端追踪拦截器。

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"
)

const grpcRequestIDHeader = "x-request-id"

func UnaryClientTraceInterceptor(tracerName string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		tracer := otel.Tracer(tracerName)
		ctx, span := tracer.Start(ctx, method, trace.WithSpanKind(trace.SpanKindClient))
		span.SetAttributes(
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.method", method),
			attribute.String("rpc.peer", cc.Target()),
		)
		defer span.End()

		md, _ := metadata.FromOutgoingContext(ctx)
		md = md.Copy()
		if reqID := RequestIDFromContext(ctx); reqID != "" {
			md.Set(grpcRequestIDHeader, reqID)
		}
		otel.GetTextMapPropagator().Inject(ctx, metadataCarrier(md))
		ctx = metadata.NewOutgoingContext(ctx, md)

		err := invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			st := grpcstatus.Convert(err)
			span.SetAttributes(attribute.String("rpc.grpc.status_code", st.Code().String()))
			span.SetStatus(otelcodes.Error, st.Message())
			return err
		}
		span.SetStatus(otelcodes.Ok, "")
		return nil
	}
}

func UnaryServerTraceInterceptor(tracerName string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()
		md, _ := metadata.FromIncomingContext(ctx)
		requestID := strings.TrimSpace(first(md.Get(grpcRequestIDHeader)))
		ctx = otel.GetTextMapPropagator().Extract(ctx, metadataCarrier(md))
		ctx = WithRequestID(ctx, requestID)

		tracer := otel.Tracer(tracerName)
		ctx, span := tracer.Start(ctx, info.FullMethod, trace.WithSpanKind(trace.SpanKindServer))
		span.SetAttributes(
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.method", info.FullMethod),
			attribute.String("request.id", requestID),
		)
		defer span.End()

		resp, err := handler(ctx, req)
		if err != nil {
			st := grpcstatus.Convert(err)
			span.SetAttributes(attribute.String("rpc.grpc.status_code", st.Code().String()))
			span.SetStatus(otelcodes.Error, st.Message())
			slog.Default().Warn("grpc_request",
				slog.String("request_id", requestID),
				slog.String("trace_id", span.SpanContext().TraceID().String()),
				slog.String("span_id", span.SpanContext().SpanID().String()),
				slog.String("method", info.FullMethod),
				slog.String("grpc_status", st.Code().String()),
				slog.Int64("latency_ms", time.Since(start).Milliseconds()),
			)
			return nil, err
		}
		span.SetStatus(otelcodes.Ok, "")
		slog.Default().Info("grpc_request",
			slog.String("request_id", requestID),
			slog.String("trace_id", span.SpanContext().TraceID().String()),
			slog.String("span_id", span.SpanContext().SpanID().String()),
			slog.String("method", info.FullMethod),
			slog.String("grpc_status", "OK"),
			slog.Int64("latency_ms", time.Since(start).Milliseconds()),
		)
		return resp, nil
	}
}

type metadataCarrier metadata.MD

func (m metadataCarrier) Get(key string) string {
	values := metadata.MD(m).Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (m metadataCarrier) Set(key string, value string) {
	metadata.MD(m).Set(key, value)
}

func (m metadataCarrier) Keys() []string {
	md := metadata.MD(m)
	keys := make([]string, 0, len(md))
	for k := range md {
		keys = append(keys, k)
	}
	return keys
}

func first(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}
