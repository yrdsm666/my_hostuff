// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.25.0
// 	protoc        v3.12.3
// source: common.proto

package proto

import (
	proto "github.com/golang/protobuf/proto"
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

// This is a compile-time assertion that a sufficiently up-to-date version
// of the legacy proto package is being used.
const _ = proto.ProtoPackageIsVersion4

type MsgType int32

const (
	MsgType_PREPARE        MsgType = 0
	MsgType_PREPARE_VOTE   MsgType = 1
	MsgType_PRECOMMIT      MsgType = 2
	MsgType_PRECOMMIT_VOTE MsgType = 3
	MsgType_COMMIT         MsgType = 4
	MsgType_COMMIT_VOTE    MsgType = 5
	MsgType_NEWVIEW        MsgType = 6
	MsgType_DECIDE         MsgType = 7
)

// Enum value maps for MsgType.
var (
	MsgType_name = map[int32]string{
		0: "PREPARE",
		1: "PREPARE_VOTE",
		2: "PRECOMMIT",
		3: "PRECOMMIT_VOTE",
		4: "COMMIT",
		5: "COMMIT_VOTE",
		6: "NEWVIEW",
		7: "DECIDE",
	}
	MsgType_value = map[string]int32{
		"PREPARE":        0,
		"PREPARE_VOTE":   1,
		"PRECOMMIT":      2,
		"PRECOMMIT_VOTE": 3,
		"COMMIT":         4,
		"COMMIT_VOTE":    5,
		"NEWVIEW":        6,
		"DECIDE":         7,
	}
)

func (x MsgType) Enum() *MsgType {
	p := new(MsgType)
	*p = x
	return p
}

func (x MsgType) String() string {
	return protoimpl.X.EnumStringOf(x.Descriptor(), protoreflect.EnumNumber(x))
}

func (MsgType) Descriptor() protoreflect.EnumDescriptor {
	return file_common_proto_enumTypes[0].Descriptor()
}

func (MsgType) Type() protoreflect.EnumType {
	return &file_common_proto_enumTypes[0]
}

func (x MsgType) Number() protoreflect.EnumNumber {
	return protoreflect.EnumNumber(x)
}

// Deprecated: Use MsgType.Descriptor instead.
func (MsgType) EnumDescriptor() ([]byte, []int) {
	return file_common_proto_rawDescGZIP(), []int{0}
}

type Block struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ParentHash []byte      `protobuf:"bytes,1,opt,name=ParentHash,proto3" json:"ParentHash,omitempty"`
	Hash       []byte      `protobuf:"bytes,2,opt,name=Hash,proto3" json:"Hash,omitempty"`
	Height     uint64      `protobuf:"varint,3,opt,name=height,proto3" json:"height,omitempty"`
	Commands   []string    `protobuf:"bytes,4,rep,name=commands,proto3" json:"commands,omitempty"`
	Justify    *QuorumCert `protobuf:"bytes,5,opt,name=Justify,proto3" json:"Justify,omitempty"`
	Committed  bool        `protobuf:"varint,6,opt,name=committed,proto3" json:"committed,omitempty"`
}

func (x *Block) Reset() {
	*x = Block{}
	if protoimpl.UnsafeEnabled {
		mi := &file_common_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Block) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Block) ProtoMessage() {}

func (x *Block) ProtoReflect() protoreflect.Message {
	mi := &file_common_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Block.ProtoReflect.Descriptor instead.
func (*Block) Descriptor() ([]byte, []int) {
	return file_common_proto_rawDescGZIP(), []int{0}
}

func (x *Block) GetParentHash() []byte {
	if x != nil {
		return x.ParentHash
	}
	return nil
}

func (x *Block) GetHash() []byte {
	if x != nil {
		return x.Hash
	}
	return nil
}

func (x *Block) GetHeight() uint64 {
	if x != nil {
		return x.Height
	}
	return 0
}

func (x *Block) GetCommands() []string {
	if x != nil {
		return x.Commands
	}
	return nil
}

func (x *Block) GetJustify() *QuorumCert {
	if x != nil {
		return x.Justify
	}
	return nil
}

func (x *Block) GetCommitted() bool {
	if x != nil {
		return x.Committed
	}
	return false
}

type QuorumCert struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	BlockHash []byte  `protobuf:"bytes,1,opt,name=BlockHash,proto3" json:"BlockHash,omitempty"`
	Type      MsgType `protobuf:"varint,2,opt,name=type,proto3,enum=proto.MsgType" json:"type,omitempty"`
	ViewNum   uint64  `protobuf:"varint,3,opt,name=viewNum,proto3" json:"viewNum,omitempty"`
	Signature []byte  `protobuf:"bytes,4,opt,name=signature,proto3" json:"signature,omitempty"`
}

func (x *QuorumCert) Reset() {
	*x = QuorumCert{}
	if protoimpl.UnsafeEnabled {
		mi := &file_common_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *QuorumCert) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*QuorumCert) ProtoMessage() {}

func (x *QuorumCert) ProtoReflect() protoreflect.Message {
	mi := &file_common_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use QuorumCert.ProtoReflect.Descriptor instead.
func (*QuorumCert) Descriptor() ([]byte, []int) {
	return file_common_proto_rawDescGZIP(), []int{1}
}

func (x *QuorumCert) GetBlockHash() []byte {
	if x != nil {
		return x.BlockHash
	}
	return nil
}

func (x *QuorumCert) GetType() MsgType {
	if x != nil {
		return x.Type
	}
	return MsgType_PREPARE
}

func (x *QuorumCert) GetViewNum() uint64 {
	if x != nil {
		return x.ViewNum
	}
	return 0
}

func (x *QuorumCert) GetSignature() []byte {
	if x != nil {
		return x.Signature
	}
	return nil
}

var File_common_proto protoreflect.FileDescriptor

var file_common_proto_rawDesc = []byte{
	0x0a, 0x0c, 0x63, 0x6f, 0x6d, 0x6d, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x05,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0xba, 0x01, 0x0a, 0x05, 0x42, 0x6c, 0x6f, 0x63, 0x6b, 0x12,
	0x1e, 0x0a, 0x0a, 0x50, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x48, 0x61, 0x73, 0x68, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x0c, 0x52, 0x0a, 0x50, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x48, 0x61, 0x73, 0x68, 0x12,
	0x12, 0x0a, 0x04, 0x48, 0x61, 0x73, 0x68, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x04, 0x48,
	0x61, 0x73, 0x68, 0x12, 0x16, 0x0a, 0x06, 0x68, 0x65, 0x69, 0x67, 0x68, 0x74, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x04, 0x52, 0x06, 0x68, 0x65, 0x69, 0x67, 0x68, 0x74, 0x12, 0x1a, 0x0a, 0x08, 0x63,
	0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64, 0x73, 0x18, 0x04, 0x20, 0x03, 0x28, 0x09, 0x52, 0x08, 0x63,
	0x6f, 0x6d, 0x6d, 0x61, 0x6e, 0x64, 0x73, 0x12, 0x2b, 0x0a, 0x07, 0x4a, 0x75, 0x73, 0x74, 0x69,
	0x66, 0x79, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x11, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x2e, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x65, 0x72, 0x74, 0x52, 0x07, 0x4a, 0x75, 0x73,
	0x74, 0x69, 0x66, 0x79, 0x12, 0x1c, 0x0a, 0x09, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x74, 0x65,
	0x64, 0x18, 0x06, 0x20, 0x01, 0x28, 0x08, 0x52, 0x09, 0x63, 0x6f, 0x6d, 0x6d, 0x69, 0x74, 0x74,
	0x65, 0x64, 0x22, 0x86, 0x01, 0x0a, 0x0a, 0x51, 0x75, 0x6f, 0x72, 0x75, 0x6d, 0x43, 0x65, 0x72,
	0x74, 0x12, 0x1c, 0x0a, 0x09, 0x42, 0x6c, 0x6f, 0x63, 0x6b, 0x48, 0x61, 0x73, 0x68, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x0c, 0x52, 0x09, 0x42, 0x6c, 0x6f, 0x63, 0x6b, 0x48, 0x61, 0x73, 0x68, 0x12,
	0x22, 0x0a, 0x04, 0x74, 0x79, 0x70, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0e, 0x32, 0x0e, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x4d, 0x73, 0x67, 0x54, 0x79, 0x70, 0x65, 0x52, 0x04, 0x74,
	0x79, 0x70, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x76, 0x69, 0x65, 0x77, 0x4e, 0x75, 0x6d, 0x18, 0x03,
	0x20, 0x01, 0x28, 0x04, 0x52, 0x07, 0x76, 0x69, 0x65, 0x77, 0x4e, 0x75, 0x6d, 0x12, 0x1c, 0x0a,
	0x09, 0x73, 0x69, 0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0c,
	0x52, 0x09, 0x73, 0x69, 0x67, 0x6e, 0x61, 0x74, 0x75, 0x72, 0x65, 0x2a, 0x81, 0x01, 0x0a, 0x07,
	0x4d, 0x73, 0x67, 0x54, 0x79, 0x70, 0x65, 0x12, 0x0b, 0x0a, 0x07, 0x50, 0x52, 0x45, 0x50, 0x41,
	0x52, 0x45, 0x10, 0x00, 0x12, 0x10, 0x0a, 0x0c, 0x50, 0x52, 0x45, 0x50, 0x41, 0x52, 0x45, 0x5f,
	0x56, 0x4f, 0x54, 0x45, 0x10, 0x01, 0x12, 0x0d, 0x0a, 0x09, 0x50, 0x52, 0x45, 0x43, 0x4f, 0x4d,
	0x4d, 0x49, 0x54, 0x10, 0x02, 0x12, 0x12, 0x0a, 0x0e, 0x50, 0x52, 0x45, 0x43, 0x4f, 0x4d, 0x4d,
	0x49, 0x54, 0x5f, 0x56, 0x4f, 0x54, 0x45, 0x10, 0x03, 0x12, 0x0a, 0x0a, 0x06, 0x43, 0x4f, 0x4d,
	0x4d, 0x49, 0x54, 0x10, 0x04, 0x12, 0x0f, 0x0a, 0x0b, 0x43, 0x4f, 0x4d, 0x4d, 0x49, 0x54, 0x5f,
	0x56, 0x4f, 0x54, 0x45, 0x10, 0x05, 0x12, 0x0b, 0x0a, 0x07, 0x4e, 0x45, 0x57, 0x56, 0x49, 0x45,
	0x57, 0x10, 0x06, 0x12, 0x0a, 0x0a, 0x06, 0x44, 0x45, 0x43, 0x49, 0x44, 0x45, 0x10, 0x07, 0x62,
	0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_common_proto_rawDescOnce sync.Once
	file_common_proto_rawDescData = file_common_proto_rawDesc
)

func file_common_proto_rawDescGZIP() []byte {
	file_common_proto_rawDescOnce.Do(func() {
		file_common_proto_rawDescData = protoimpl.X.CompressGZIP(file_common_proto_rawDescData)
	})
	return file_common_proto_rawDescData
}

var file_common_proto_enumTypes = make([]protoimpl.EnumInfo, 1)
var file_common_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_common_proto_goTypes = []interface{}{
	(MsgType)(0),       // 0: proto.MsgType
	(*Block)(nil),      // 1: proto.Block
	(*QuorumCert)(nil), // 2: proto.QuorumCert
}
var file_common_proto_depIdxs = []int32{
	2, // 0: proto.Block.Justify:type_name -> proto.QuorumCert
	0, // 1: proto.QuorumCert.type:type_name -> proto.MsgType
	2, // [2:2] is the sub-list for method output_type
	2, // [2:2] is the sub-list for method input_type
	2, // [2:2] is the sub-list for extension type_name
	2, // [2:2] is the sub-list for extension extendee
	0, // [0:2] is the sub-list for field type_name
}

func init() { file_common_proto_init() }
func file_common_proto_init() {
	if File_common_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_common_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Block); i {
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
		file_common_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*QuorumCert); i {
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
			RawDescriptor: file_common_proto_rawDesc,
			NumEnums:      1,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_common_proto_goTypes,
		DependencyIndexes: file_common_proto_depIdxs,
		EnumInfos:         file_common_proto_enumTypes,
		MessageInfos:      file_common_proto_msgTypes,
	}.Build()
	File_common_proto = out.File
	file_common_proto_rawDesc = nil
	file_common_proto_goTypes = nil
	file_common_proto_depIdxs = nil
}
