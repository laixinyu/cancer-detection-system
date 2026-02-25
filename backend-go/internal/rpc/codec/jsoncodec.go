package codec

// 文件： internal/rpc/codec/jsoncodec.go
// 用途：内部 gRPC 桥接协议、编解码器与 HTTP 适配器。

import "encoding/json"

type JSONCodec struct{}

func New() JSONCodec {
	return JSONCodec{}
}

func (JSONCodec) Name() string {
	return "json"
}

func (JSONCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (JSONCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
