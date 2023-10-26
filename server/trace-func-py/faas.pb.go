//
// Set up:
// $ go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.26
// $ go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.1
//
// Add `<GOPATH>/bin` to your $PATH:
// OR (!suboptimal since it overwrites $PATH by appending an additional
// line as opposed to change it directly)
// $ echo "export PATH=$PATH:$(go env GOPATH)/bin" >> ~/.profile
// $ source ~/.profile
// OR temporarily
// $ export PATH="$PATH:$(go env GOPATH)/bin"

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        v3.20.3
// source: server/trace-func-py/faas.proto

package proto

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type FaasRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Message           string `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`                      // Text message field (unused).
	RuntimeInMilliSec uint32 `protobuf:"varint,2,opt,name=runtimeInMilliSec,proto3" json:"runtimeInMilliSec,omitempty"` // Execution runtime [ms].
	MemoryInMebiBytes uint32 `protobuf:"varint,3,opt,name=memoryInMebiBytes,proto3" json:"memoryInMebiBytes,omitempty"` // Request memory usage [MiB].
}

func (x *FaasRequest) Reset() {
	*x = FaasRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_server_trace_func_py_faas_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FaasRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FaasRequest) ProtoMessage() {}

func (x *FaasRequest) ProtoReflect() protoreflect.Message {
	mi := &file_server_trace_func_py_faas_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FaasRequest.ProtoReflect.Descriptor instead.
func (*FaasRequest) Descriptor() ([]byte, []int) {
	return file_server_trace_func_py_faas_proto_rawDescGZIP(), []int{0}
}

func (x *FaasRequest) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *FaasRequest) GetRuntimeInMilliSec() uint32 {
	if x != nil {
		return x.RuntimeInMilliSec
	}
	return 0
}

func (x *FaasRequest) GetMemoryInMebiBytes() uint32 {
	if x != nil {
		return x.MemoryInMebiBytes
	}
	return 0
}

type FaasReply struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Message            string `protobuf:"bytes,1,opt,name=message,proto3" json:"message,omitempty"`                        // Text message field (unused).
	DurationInMicroSec uint32 `protobuf:"varint,2,opt,name=durationInMicroSec,proto3" json:"durationInMicroSec,omitempty"` // Execution latency [µs].
	MemoryUsageInKb    uint32 `protobuf:"varint,3,opt,name=memoryUsageInKb,proto3" json:"memoryUsageInKb,omitempty"`       // Memory usage [KB].
}

func (x *FaasReply) Reset() {
	*x = FaasReply{}
	if protoimpl.UnsafeEnabled {
		mi := &file_server_trace_func_py_faas_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *FaasReply) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*FaasReply) ProtoMessage() {}

func (x *FaasReply) ProtoReflect() protoreflect.Message {
	mi := &file_server_trace_func_py_faas_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use FaasReply.ProtoReflect.Descriptor instead.
func (*FaasReply) Descriptor() ([]byte, []int) {
	return file_server_trace_func_py_faas_proto_rawDescGZIP(), []int{1}
}

func (x *FaasReply) GetMessage() string {
	if x != nil {
		return x.Message
	}
	return ""
}

func (x *FaasReply) GetDurationInMicroSec() uint32 {
	if x != nil {
		return x.DurationInMicroSec
	}
	return 0
}

func (x *FaasReply) GetMemoryUsageInKb() uint32 {
	if x != nil {
		return x.MemoryUsageInKb
	}
	return 0
}

var File_server_trace_func_py_faas_proto protoreflect.FileDescriptor

var file_server_trace_func_py_faas_proto_rawDesc = []byte{
	0x0a, 0x1f, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x2f, 0x74, 0x72, 0x61, 0x63, 0x65, 0x2d, 0x66,
	0x75, 0x6e, 0x63, 0x2d, 0x70, 0x79, 0x2f, 0x66, 0x61, 0x61, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x12, 0x04, 0x66, 0x61, 0x61, 0x73, 0x22, 0x83, 0x01, 0x0a, 0x0b, 0x46, 0x61, 0x61, 0x73,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61,
	0x67, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67,
	0x65, 0x12, 0x2c, 0x0a, 0x11, 0x72, 0x75, 0x6e, 0x74, 0x69, 0x6d, 0x65, 0x49, 0x6e, 0x4d, 0x69,
	0x6c, 0x6c, 0x69, 0x53, 0x65, 0x63, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x11, 0x72, 0x75,
	0x6e, 0x74, 0x69, 0x6d, 0x65, 0x49, 0x6e, 0x4d, 0x69, 0x6c, 0x6c, 0x69, 0x53, 0x65, 0x63, 0x12,
	0x2c, 0x0a, 0x11, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x49, 0x6e, 0x4d, 0x65, 0x62, 0x69, 0x42,
	0x79, 0x74, 0x65, 0x73, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x11, 0x6d, 0x65, 0x6d, 0x6f,
	0x72, 0x79, 0x49, 0x6e, 0x4d, 0x65, 0x62, 0x69, 0x42, 0x79, 0x74, 0x65, 0x73, 0x22, 0x7f, 0x0a,
	0x09, 0x46, 0x61, 0x61, 0x73, 0x52, 0x65, 0x70, 0x6c, 0x79, 0x12, 0x18, 0x0a, 0x07, 0x6d, 0x65,
	0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x6d, 0x65, 0x73,
	0x73, 0x61, 0x67, 0x65, 0x12, 0x2e, 0x0a, 0x12, 0x64, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e,
	0x49, 0x6e, 0x4d, 0x69, 0x63, 0x72, 0x6f, 0x53, 0x65, 0x63, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d,
	0x52, 0x12, 0x64, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x6e, 0x4d, 0x69, 0x63, 0x72,
	0x6f, 0x53, 0x65, 0x63, 0x12, 0x28, 0x0a, 0x0f, 0x6d, 0x65, 0x6d, 0x6f, 0x72, 0x79, 0x55, 0x73,
	0x61, 0x67, 0x65, 0x49, 0x6e, 0x4b, 0x62, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0f, 0x6d,
	0x65, 0x6d, 0x6f, 0x72, 0x79, 0x55, 0x73, 0x61, 0x67, 0x65, 0x49, 0x6e, 0x4b, 0x62, 0x32, 0x3b,
	0x0a, 0x08, 0x45, 0x78, 0x65, 0x63, 0x75, 0x74, 0x6f, 0x72, 0x12, 0x2f, 0x0a, 0x07, 0x45, 0x78,
	0x65, 0x63, 0x75, 0x74, 0x65, 0x12, 0x11, 0x2e, 0x66, 0x61, 0x61, 0x73, 0x2e, 0x46, 0x61, 0x61,
	0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x0f, 0x2e, 0x66, 0x61, 0x61, 0x73, 0x2e,
	0x46, 0x61, 0x61, 0x73, 0x52, 0x65, 0x70, 0x6c, 0x79, 0x22, 0x00, 0x42, 0x33, 0x5a, 0x31, 0x67,
	0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x76, 0x68, 0x69, 0x76, 0x65, 0x2d,
	0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x6c, 0x65, 0x73, 0x73, 0x2f, 0x6c, 0x6f, 0x61, 0x64, 0x65,
	0x72, 0x2f, 0x77, 0x6f, 0x72, 0x6b, 0x6c, 0x6f, 0x61, 0x64, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_server_trace_func_py_faas_proto_rawDescOnce sync.Once
	file_server_trace_func_py_faas_proto_rawDescData = file_server_trace_func_py_faas_proto_rawDesc
)

func file_server_trace_func_py_faas_proto_rawDescGZIP() []byte {
	file_server_trace_func_py_faas_proto_rawDescOnce.Do(func() {
		file_server_trace_func_py_faas_proto_rawDescData = protoimpl.X.CompressGZIP(file_server_trace_func_py_faas_proto_rawDescData)
	})
	return file_server_trace_func_py_faas_proto_rawDescData
}

var file_server_trace_func_py_faas_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_server_trace_func_py_faas_proto_goTypes = []interface{}{
	(*FaasRequest)(nil), // 0: faas.FaasRequest
	(*FaasReply)(nil),   // 1: faas.FaasReply
}
var file_server_trace_func_py_faas_proto_depIdxs = []int32{
	0, // 0: faas.Executor.Execute:input_type -> faas.FaasRequest
	1, // 1: faas.Executor.Execute:output_type -> faas.FaasReply
	1, // [1:2] is the sub-list for method output_type
	0, // [0:1] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_server_trace_func_py_faas_proto_init() }
func file_server_trace_func_py_faas_proto_init() {
	if File_server_trace_func_py_faas_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_server_trace_func_py_faas_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*FaasRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_server_trace_func_py_faas_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*FaasReply); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_server_trace_func_py_faas_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_server_trace_func_py_faas_proto_goTypes,
		DependencyIndexes: file_server_trace_func_py_faas_proto_depIdxs,
		MessageInfos:      file_server_trace_func_py_faas_proto_msgTypes,
	}.Build()
	File_server_trace_func_py_faas_proto = out.File
	file_server_trace_func_py_faas_proto_rawDesc = nil
	file_server_trace_func_py_faas_proto_goTypes = nil
	file_server_trace_func_py_faas_proto_depIdxs = nil
}