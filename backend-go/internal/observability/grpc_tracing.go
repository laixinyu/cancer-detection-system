package observability

// File: internal/observability/grpc_tracing.go
// Purpose: gRPC client/server tracing interceptors with W3C trace context propagation.

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"
)

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
		md, _ := metadata.FromIncomingContext(ctx)
		ctx = otel.GetTextMapPropagator().Extract(ctx, metadataCarrier(md))

		tracer := otel.Tracer(tracerName)
		ctx, span := tracer.Start(ctx, info.FullMethod, trace.WithSpanKind(trace.SpanKindServer))
		span.SetAttributes(
			attribute.String("rpc.system", "grpc"),
			attribute.String("rpc.method", info.FullMethod),
		)
		defer span.End()

		resp, err := handler(ctx, req)
		if err != nil {
			st := grpcstatus.Convert(err)
			span.SetAttributes(attribute.String("rpc.grpc.status_code", st.Code().String()))
			span.SetStatus(otelcodes.Error, st.Message())
			return nil, err
		}
		span.SetStatus(otelcodes.Ok, "")
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
