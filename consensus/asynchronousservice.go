package consensus

import (
	"context"
	pb "github.com/wjbbig/go-hotstuff/proto"
)



type AsynchronousService struct {
	asynchronous Asynchronous
}

func (basic *AsynchronousService) SendMsg(ctx context.Context, in *pb.Msg) (*pb.Empty, error) {
	basic.asynchronous.GetMsgEntrance() <- in
	return &pb.Empty{}, nil
}

func (basic *AsynchronousService) SendRequest(ctx context.Context, in *pb.Msg) (*pb.Empty, error) {
	basic.asynchronous.GetMsgEntrance() <- in
	return &pb.Empty{}, nil
}


func (basic *AsynchronousService) SendReply(ctx context.Context, in *pb.Msg) (*pb.Empty, error) {
	return &pb.Empty{}, nil
}

func (basic *AsynchronousService) GetImpl() Asynchronous {
	return basic.asynchronous
}

func NewAsynchronousService(impl Asynchronous) *AsynchronousService {
	return &AsynchronousService{asynchronous: impl}
}

