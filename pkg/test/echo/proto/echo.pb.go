// Code generated by protoc-gen-go. DO NOT EDIT.
// source: echo.proto

// Generate with protoc --go_out=plugins=grpc:. echo.proto

package proto

import (
	context "context"
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type EchoRequest struct {
	Message              string   `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *EchoRequest) Reset()         { *m = EchoRequest{} }
func (m *EchoRequest) String() string { return proto.CompactTextString(m) }
func (*EchoRequest) ProtoMessage()    {}
func (*EchoRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_08134aea513e0001, []int{0}
}

func (m *EchoRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_EchoRequest.Unmarshal(m, b)
}
func (m *EchoRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_EchoRequest.Marshal(b, m, deterministic)
}
func (m *EchoRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_EchoRequest.Merge(m, src)
}
func (m *EchoRequest) XXX_Size() int {
	return xxx_messageInfo_EchoRequest.Size(m)
}
func (m *EchoRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_EchoRequest.DiscardUnknown(m)
}

var xxx_messageInfo_EchoRequest proto.InternalMessageInfo

func (m *EchoRequest) GetMessage() string {
	if m != nil {
		return m.Message
	}
	return ""
}

type EchoResponse struct {
	Message              string   `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *EchoResponse) Reset()         { *m = EchoResponse{} }
func (m *EchoResponse) String() string { return proto.CompactTextString(m) }
func (*EchoResponse) ProtoMessage()    {}
func (*EchoResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_08134aea513e0001, []int{1}
}

func (m *EchoResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_EchoResponse.Unmarshal(m, b)
}
func (m *EchoResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_EchoResponse.Marshal(b, m, deterministic)
}
func (m *EchoResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_EchoResponse.Merge(m, src)
}
func (m *EchoResponse) XXX_Size() int {
	return xxx_messageInfo_EchoResponse.Size(m)
}
func (m *EchoResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_EchoResponse.DiscardUnknown(m)
}

var xxx_messageInfo_EchoResponse proto.InternalMessageInfo

func (m *EchoResponse) GetMessage() string {
	if m != nil {
		return m.Message
	}
	return ""
}

type Header struct {
	Key                  string   `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
	Value                string   `protobuf:"bytes,2,opt,name=value,proto3" json:"value,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Header) Reset()         { *m = Header{} }
func (m *Header) String() string { return proto.CompactTextString(m) }
func (*Header) ProtoMessage()    {}
func (*Header) Descriptor() ([]byte, []int) {
	return fileDescriptor_08134aea513e0001, []int{2}
}

func (m *Header) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Header.Unmarshal(m, b)
}
func (m *Header) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Header.Marshal(b, m, deterministic)
}
func (m *Header) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Header.Merge(m, src)
}
func (m *Header) XXX_Size() int {
	return xxx_messageInfo_Header.Size(m)
}
func (m *Header) XXX_DiscardUnknown() {
	xxx_messageInfo_Header.DiscardUnknown(m)
}

var xxx_messageInfo_Header proto.InternalMessageInfo

func (m *Header) GetKey() string {
	if m != nil {
		return m.Key
	}
	return ""
}

func (m *Header) GetValue() string {
	if m != nil {
		return m.Value
	}
	return ""
}

type ForwardEchoRequest struct {
	Count         int32     `protobuf:"varint,1,opt,name=count,proto3" json:"count,omitempty"`
	Qps           int32     `protobuf:"varint,2,opt,name=qps,proto3" json:"qps,omitempty"`
	TimeoutMicros int64     `protobuf:"varint,3,opt,name=timeout_micros,json=timeoutMicros,proto3" json:"timeout_micros,omitempty"`
	Url           string    `protobuf:"bytes,4,opt,name=url,proto3" json:"url,omitempty"`
	Headers       []*Header `protobuf:"bytes,5,rep,name=headers,proto3" json:"headers,omitempty"`
	Message       string    `protobuf:"bytes,6,opt,name=message,proto3" json:"message,omitempty"`
	// Method for the request. Valid only for HTTP
	Method string `protobuf:"bytes,9,opt,name=method,proto3" json:"method,omitempty"`
	// If true, requests will be sent using h2c prior knowledge
	Http2 bool `protobuf:"varint,7,opt,name=http2,proto3" json:"http2,omitempty"`
	// If true, requests will be sent using http3
	Http3 bool `protobuf:"varint,15,opt,name=http3,proto3" json:"http3,omitempty"`
	// If true, requests will not be sent until magic string is received
	ServerFirst bool `protobuf:"varint,8,opt,name=serverFirst,proto3" json:"serverFirst,omitempty"`
	// If true, 301 redirects will be followed
	FollowRedirects bool `protobuf:"varint,14,opt,name=followRedirects,proto3" json:"followRedirects,omitempty"`
	// If non-empty, make the request with the corresponding cert and key.
	Cert string `protobuf:"bytes,10,opt,name=cert,proto3" json:"cert,omitempty"`
	Key  string `protobuf:"bytes,11,opt,name=key,proto3" json:"key,omitempty"`
	// If non-empty, verify the server CA
	CaCert string `protobuf:"bytes,12,opt,name=caCert,proto3" json:"caCert,omitempty"`
	// List of ALPNs to present. If not set, this will be automatically be set based on the protocol
	Alpn                 *Alpn    `protobuf:"bytes,13,opt,name=alpn,proto3" json:"alpn,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ForwardEchoRequest) Reset()         { *m = ForwardEchoRequest{} }
func (m *ForwardEchoRequest) String() string { return proto.CompactTextString(m) }
func (*ForwardEchoRequest) ProtoMessage()    {}
func (*ForwardEchoRequest) Descriptor() ([]byte, []int) {
	return fileDescriptor_08134aea513e0001, []int{3}
}

func (m *ForwardEchoRequest) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ForwardEchoRequest.Unmarshal(m, b)
}
func (m *ForwardEchoRequest) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ForwardEchoRequest.Marshal(b, m, deterministic)
}
func (m *ForwardEchoRequest) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ForwardEchoRequest.Merge(m, src)
}
func (m *ForwardEchoRequest) XXX_Size() int {
	return xxx_messageInfo_ForwardEchoRequest.Size(m)
}
func (m *ForwardEchoRequest) XXX_DiscardUnknown() {
	xxx_messageInfo_ForwardEchoRequest.DiscardUnknown(m)
}

var xxx_messageInfo_ForwardEchoRequest proto.InternalMessageInfo

func (m *ForwardEchoRequest) GetCount() int32 {
	if m != nil {
		return m.Count
	}
	return 0
}

func (m *ForwardEchoRequest) GetQps() int32 {
	if m != nil {
		return m.Qps
	}
	return 0
}

func (m *ForwardEchoRequest) GetTimeoutMicros() int64 {
	if m != nil {
		return m.TimeoutMicros
	}
	return 0
}

func (m *ForwardEchoRequest) GetUrl() string {
	if m != nil {
		return m.Url
	}
	return ""
}

func (m *ForwardEchoRequest) GetHeaders() []*Header {
	if m != nil {
		return m.Headers
	}
	return nil
}

func (m *ForwardEchoRequest) GetMessage() string {
	if m != nil {
		return m.Message
	}
	return ""
}

func (m *ForwardEchoRequest) GetMethod() string {
	if m != nil {
		return m.Method
	}
	return ""
}

func (m *ForwardEchoRequest) GetHttp2() bool {
	if m != nil {
		return m.Http2
	}
	return false
}

func (m *ForwardEchoRequest) GetHttp3() bool {
	if m != nil {
		return m.Http3
	}
	return false
}

func (m *ForwardEchoRequest) GetServerFirst() bool {
	if m != nil {
		return m.ServerFirst
	}
	return false
}

func (m *ForwardEchoRequest) GetFollowRedirects() bool {
	if m != nil {
		return m.FollowRedirects
	}
	return false
}

func (m *ForwardEchoRequest) GetCert() string {
	if m != nil {
		return m.Cert
	}
	return ""
}

func (m *ForwardEchoRequest) GetKey() string {
	if m != nil {
		return m.Key
	}
	return ""
}

func (m *ForwardEchoRequest) GetCaCert() string {
	if m != nil {
		return m.CaCert
	}
	return ""
}

func (m *ForwardEchoRequest) GetAlpn() *Alpn {
	if m != nil {
		return m.Alpn
	}
	return nil
}

type Alpn struct {
	Value                []string `protobuf:"bytes,1,rep,name=value,proto3" json:"value,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Alpn) Reset()         { *m = Alpn{} }
func (m *Alpn) String() string { return proto.CompactTextString(m) }
func (*Alpn) ProtoMessage()    {}
func (*Alpn) Descriptor() ([]byte, []int) {
	return fileDescriptor_08134aea513e0001, []int{4}
}

func (m *Alpn) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Alpn.Unmarshal(m, b)
}
func (m *Alpn) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Alpn.Marshal(b, m, deterministic)
}
func (m *Alpn) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Alpn.Merge(m, src)
}
func (m *Alpn) XXX_Size() int {
	return xxx_messageInfo_Alpn.Size(m)
}
func (m *Alpn) XXX_DiscardUnknown() {
	xxx_messageInfo_Alpn.DiscardUnknown(m)
}

var xxx_messageInfo_Alpn proto.InternalMessageInfo

func (m *Alpn) GetValue() []string {
	if m != nil {
		return m.Value
	}
	return nil
}

type ForwardEchoResponse struct {
	Output               []string `protobuf:"bytes,1,rep,name=output,proto3" json:"output,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ForwardEchoResponse) Reset()         { *m = ForwardEchoResponse{} }
func (m *ForwardEchoResponse) String() string { return proto.CompactTextString(m) }
func (*ForwardEchoResponse) ProtoMessage()    {}
func (*ForwardEchoResponse) Descriptor() ([]byte, []int) {
	return fileDescriptor_08134aea513e0001, []int{5}
}

func (m *ForwardEchoResponse) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ForwardEchoResponse.Unmarshal(m, b)
}
func (m *ForwardEchoResponse) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ForwardEchoResponse.Marshal(b, m, deterministic)
}
func (m *ForwardEchoResponse) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ForwardEchoResponse.Merge(m, src)
}
func (m *ForwardEchoResponse) XXX_Size() int {
	return xxx_messageInfo_ForwardEchoResponse.Size(m)
}
func (m *ForwardEchoResponse) XXX_DiscardUnknown() {
	xxx_messageInfo_ForwardEchoResponse.DiscardUnknown(m)
}

var xxx_messageInfo_ForwardEchoResponse proto.InternalMessageInfo

func (m *ForwardEchoResponse) GetOutput() []string {
	if m != nil {
		return m.Output
	}
	return nil
}

func init() {
	proto.RegisterType((*EchoRequest)(nil), "proto.EchoRequest")
	proto.RegisterType((*EchoResponse)(nil), "proto.EchoResponse")
	proto.RegisterType((*Header)(nil), "proto.Header")
	proto.RegisterType((*ForwardEchoRequest)(nil), "proto.ForwardEchoRequest")
	proto.RegisterType((*Alpn)(nil), "proto.Alpn")
	proto.RegisterType((*ForwardEchoResponse)(nil), "proto.ForwardEchoResponse")
}

func init() { proto.RegisterFile("echo.proto", fileDescriptor_08134aea513e0001) }

var fileDescriptor_08134aea513e0001 = []byte{
	// 425 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x7c, 0x52, 0xdb, 0x6e, 0xd3, 0x40,
	0x10, 0x95, 0xf1, 0x25, 0xed, 0xb8, 0x69, 0xd0, 0xb4, 0x8a, 0x96, 0x08, 0x09, 0x2b, 0x12, 0xaa,
	0x5f, 0x28, 0x28, 0xf9, 0x02, 0x04, 0x54, 0xbc, 0xf0, 0xb2, 0xf0, 0x8e, 0xcc, 0x66, 0xc0, 0x16,
	0x4e, 0xd6, 0xdd, 0x4b, 0x2a, 0xfe, 0x80, 0x3f, 0xe1, 0x37, 0xd1, 0x5e, 0xa2, 0x38, 0x80, 0x78,
	0xf2, 0x9c, 0x8b, 0x67, 0xcf, 0xee, 0x0c, 0x00, 0x89, 0x56, 0xde, 0x0e, 0x4a, 0x1a, 0x89, 0xb9,
	0xff, 0x2c, 0x6f, 0xa0, 0x7c, 0x27, 0x5a, 0xc9, 0xe9, 0xde, 0x92, 0x36, 0xc8, 0x60, 0xb2, 0x25,
	0xad, 0x9b, 0x6f, 0xc4, 0x92, 0x2a, 0xa9, 0xcf, 0xf9, 0x01, 0x2e, 0x6b, 0xb8, 0x08, 0x46, 0x3d,
	0xc8, 0x9d, 0xa6, 0xff, 0x38, 0x5f, 0x41, 0xf1, 0x9e, 0x9a, 0x0d, 0x29, 0x7c, 0x0c, 0xe9, 0x77,
	0xfa, 0x11, 0x75, 0x57, 0xe2, 0x35, 0xe4, 0xfb, 0xa6, 0xb7, 0xc4, 0x1e, 0x79, 0x2e, 0x80, 0xe5,
	0xaf, 0x14, 0xf0, 0x4e, 0xaa, 0x87, 0x46, 0x6d, 0xc6, 0x61, 0xae, 0x21, 0x17, 0xd2, 0xee, 0x8c,
	0x6f, 0x90, 0xf3, 0x00, 0x5c, 0xd3, 0xfb, 0x41, 0xfb, 0x06, 0x39, 0x77, 0x25, 0x3e, 0x87, 0x4b,
	0xd3, 0x6d, 0x49, 0x5a, 0xf3, 0x79, 0xdb, 0x09, 0x25, 0x35, 0x4b, 0xab, 0xa4, 0x4e, 0xf9, 0x34,
	0xb2, 0x1f, 0x3c, 0xe9, 0x7e, 0xb4, 0xaa, 0x67, 0x59, 0x48, 0x63, 0x55, 0x8f, 0x37, 0x30, 0x69,
	0x7d, 0x52, 0xcd, 0xf2, 0x2a, 0xad, 0xcb, 0xd5, 0x34, 0x3c, 0xce, 0x6d, 0xc8, 0xcf, 0x0f, 0xea,
	0xf8, 0xb2, 0xc5, 0xc9, 0x65, 0x71, 0x0e, 0xc5, 0x96, 0x4c, 0x2b, 0x37, 0xec, 0xdc, 0x0b, 0x11,
	0xb9, 0xec, 0xad, 0x31, 0xc3, 0x8a, 0x4d, 0xaa, 0xa4, 0x3e, 0xe3, 0x01, 0x1c, 0xd8, 0x35, 0x9b,
	0x1d, 0xd9, 0x35, 0x56, 0x50, 0x6a, 0x52, 0x7b, 0x52, 0x77, 0x9d, 0xd2, 0x86, 0x9d, 0x79, 0x6d,
	0x4c, 0x61, 0x0d, 0xb3, 0xaf, 0xb2, 0xef, 0xe5, 0x03, 0xa7, 0x4d, 0xa7, 0x48, 0x18, 0xcd, 0x2e,
	0xbd, 0xeb, 0x4f, 0x1a, 0x11, 0x32, 0x41, 0xca, 0x30, 0xf0, 0x69, 0x7c, 0x7d, 0x18, 0x43, 0x79,
	0x1c, 0xc3, 0x1c, 0x0a, 0xd1, 0xbc, 0x71, 0xbe, 0x8b, 0x90, 0x3a, 0x20, 0x7c, 0x06, 0x59, 0xd3,
	0x0f, 0x3b, 0x36, 0xad, 0x92, 0xba, 0x5c, 0x95, 0xf1, 0x35, 0x5e, 0xf7, 0xc3, 0x8e, 0x7b, 0x61,
	0xf9, 0x14, 0x32, 0x87, 0x8e, 0x73, 0x4c, 0xaa, 0xf4, 0x38, 0xc7, 0x17, 0x70, 0x75, 0x32, 0xc6,
	0xb8, 0x2a, 0x73, 0x28, 0xa4, 0x35, 0x83, 0x35, 0xd1, 0x1d, 0xd1, 0xea, 0x67, 0x02, 0x33, 0x67,
	0xfc, 0x44, 0xda, 0x7c, 0x24, 0xb5, 0xef, 0x04, 0xe1, 0x4b, 0xc8, 0x1c, 0x85, 0x18, 0xcf, 0x1e,
	0xed, 0xc3, 0xe2, 0xea, 0x84, 0x8b, 0xcd, 0xdf, 0x42, 0x39, 0x3a, 0x13, 0x9f, 0x44, 0xcf, 0xdf,
	0xeb, 0xb4, 0x58, 0xfc, 0x4b, 0x0a, 0x5d, 0xbe, 0x14, 0x5e, 0x5a, 0xff, 0x0e, 0x00, 0x00, 0xff,
	0xff, 0xb5, 0x92, 0x02, 0xf4, 0x22, 0x03, 0x00, 0x00,
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// EchoTestServiceClient is the client API for EchoTestService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://godoc.org/google.golang.org/grpc#ClientConn.NewStream.
type EchoTestServiceClient interface {
	Echo(ctx context.Context, in *EchoRequest, opts ...grpc.CallOption) (*EchoResponse, error)
	ForwardEcho(ctx context.Context, in *ForwardEchoRequest, opts ...grpc.CallOption) (*ForwardEchoResponse, error)
}

type echoTestServiceClient struct {
	cc *grpc.ClientConn
}

func NewEchoTestServiceClient(cc *grpc.ClientConn) EchoTestServiceClient {
	return &echoTestServiceClient{cc}
}

func (c *echoTestServiceClient) Echo(ctx context.Context, in *EchoRequest, opts ...grpc.CallOption) (*EchoResponse, error) {
	out := new(EchoResponse)
	err := c.cc.Invoke(ctx, "/proto.EchoTestService/Echo", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *echoTestServiceClient) ForwardEcho(ctx context.Context, in *ForwardEchoRequest, opts ...grpc.CallOption) (*ForwardEchoResponse, error) {
	out := new(ForwardEchoResponse)
	err := c.cc.Invoke(ctx, "/proto.EchoTestService/ForwardEcho", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// EchoTestServiceServer is the server API for EchoTestService service.
type EchoTestServiceServer interface {
	Echo(context.Context, *EchoRequest) (*EchoResponse, error)
	ForwardEcho(context.Context, *ForwardEchoRequest) (*ForwardEchoResponse, error)
}

// UnimplementedEchoTestServiceServer can be embedded to have forward compatible implementations.
type UnimplementedEchoTestServiceServer struct {
}

func (*UnimplementedEchoTestServiceServer) Echo(ctx context.Context, req *EchoRequest) (*EchoResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Echo not implemented")
}
func (*UnimplementedEchoTestServiceServer) ForwardEcho(ctx context.Context, req *ForwardEchoRequest) (*ForwardEchoResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ForwardEcho not implemented")
}

func RegisterEchoTestServiceServer(s *grpc.Server, srv EchoTestServiceServer) {
	s.RegisterService(&_EchoTestService_serviceDesc, srv)
}

func _EchoTestService_Echo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(EchoRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(EchoTestServiceServer).Echo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.EchoTestService/Echo",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(EchoTestServiceServer).Echo(ctx, req.(*EchoRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _EchoTestService_ForwardEcho_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ForwardEchoRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(EchoTestServiceServer).ForwardEcho(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/proto.EchoTestService/ForwardEcho",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(EchoTestServiceServer).ForwardEcho(ctx, req.(*ForwardEchoRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _EchoTestService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "proto.EchoTestService",
	HandlerType: (*EchoTestServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Echo",
			Handler:    _EchoTestService_Echo_Handler,
		},
		{
			MethodName: "ForwardEcho",
			Handler:    _EchoTestService_ForwardEcho_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "echo.proto",
}
