package consensus

import (
	"context"

	pb "github.com/wjbbig/go-hotstuff/proto"
)

type ParallelService struct {
	parallel Parallel
}

func (basic *ParallelService) SendMsg(ctx context.Context, in *pb.Msg) (*pb.Empty, error) {
	basic.parallel.GetMsgEntrance() <- in
	return &pb.Empty{}, nil
}

func (basic *ParallelService) SendRequest(ctx context.Context, in *pb.Msg) (*pb.Empty, error) {
	basic.parallel.GetMsgEntrance() <- in
	return &pb.Empty{}, nil
}

func (basic *ParallelService) SendReply(ctx context.Context, in *pb.Msg) (*pb.Empty, error) {
	return &pb.Empty{}, nil
}

func (basic *ParallelService) GetImpl() Parallel {
	return basic.parallel
}

func NewParallelService(impl Parallel) *ParallelService {
	return &ParallelService{parallel: impl}
}
