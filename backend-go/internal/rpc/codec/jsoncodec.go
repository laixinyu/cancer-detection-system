package codec

// File: internal/rpc/codec/jsoncodec.go
// Purpose: Internal gRPC bridge contracts, codec, and HTTP bridge adapter.

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
