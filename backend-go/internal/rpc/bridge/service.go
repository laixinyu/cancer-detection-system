package bridge

// File: internal/rpc/bridge/service.go
// Purpose: Internal gRPC bridge contracts, codec, and HTTP bridge adapter.

import (
	"context"

	"google.golang.org/grpc"
)

const (
	ServiceName      = "bridge.Service"
	HandleMethodName = "Handle"
	HandleFullMethod = "/" + ServiceName + "/" + HandleMethodName
)

type RequestEnvelope struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Query   map[string]string `json:"query"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body"`
}

type ResponseEnvelope struct {
	Status int               `json:"status"`
	Body   any               `json:"body"`
	Header map[string]string `json:"header"`
}

type ServiceServer interface {
	Handle(context.Context, *RequestEnvelope) (*ResponseEnvelope, error)
}

type ServiceClient interface {
	Handle(ctx context.Context, in *RequestEnvelope, opts ...grpc.CallOption) (*ResponseEnvelope, error)
}

type serviceClient struct {
	cc grpc.ClientConnInterface
}

func NewServiceClient(cc grpc.ClientConnInterface) ServiceClient {
	return &serviceClient{cc: cc}
}

func (c *serviceClient) Handle(ctx context.Context, in *RequestEnvelope, opts ...grpc.CallOption) (*ResponseEnvelope, error) {
	out := new(ResponseEnvelope)
	if err := c.cc.Invoke(ctx, HandleFullMethod, in, out, opts...); err != nil {
		return nil, err
	}
	return out, nil
}

func RegisterServiceServer(registrar grpc.ServiceRegistrar, srv ServiceServer) {
	registrar.RegisterService(&grpc.ServiceDesc{
		ServiceName: ServiceName,
		HandlerType: (*ServiceServer)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: HandleMethodName,
				Handler: func(service any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
					in := new(RequestEnvelope)
					if err := dec(in); err != nil {
						return nil, err
					}
					return service.(ServiceServer).Handle(ctx, in)
				},
			},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "bridge",
	}, srv)
}
