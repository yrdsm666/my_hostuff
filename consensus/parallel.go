package consensus

import (
	// "bytes"
	"context"
	// "encoding/json"
	// "fmt"
	//"github.com/niclabs/tcrsa"
	go_hotstuff "github.com/wjbbig/go-hotstuff"
	"github.com/wjbbig/go-hotstuff/config"
	pb "github.com/wjbbig/go-hotstuff/proto"
	"google.golang.org/grpc"
	// "os"
	// "strconv"
	// "fmt"
)

// common hotstuff func defined in the paper
type Parallel interface {
	//Msg(msgType pb.MsgType, id int, round int, sid int, proposal []byte, signatureByte []byte) *pb.Msg
	GetMsgEntrance() chan<- *pb.Msg
	GetNetworkInfo() map[uint32]string
	GetSelfInfo() *config.ReplicaInfo
	SafeExit()
	Broadcast(msg *pb.Msg) error
	Unicast(address string, msg *pb.Msg) error
	ProcessProposal(cmds []string) error
}

type ParallelImpl struct {
	ID          uint32
	MsgEntrance chan *pb.Msg // receive msg
	Config      config.HotStuffConfig
	TxnSet      go_hotstuff.CmdSet
}

// func (a *ParallelImpl) Msg(msgType pb.MsgType, id int, round int, sid int, proposal []byte, signatureByte []byte) *pb.Msg {
// 	msg := &pb.Msg{}
// 	switch msgType {
// 	case pb.MsgType_PBVALUE:
// 		msg.Payload = &pb.Msg_PbValue{PbValue: &pb.PbValue{
// 			Id: uint64(id),
// 			Sid: uint64(sid),
// 			Proposal: proposal,
// 		}}
// 		break
// 	case pb.MsgType_PBECHO:
// 		msg.Payload = &pb.Msg_PbEcho{PbEcho: &pb.PbEcho{
// 			Id: uint64(id),
// 			Sid: uint64(sid),
// 			Proposal: proposal,
// 			SignShare: signatureByte,
// 		}}
// 		break
// 	case pb.MsgType_PBFINAL:
// 		msg.Payload = &pb.Msg_PbFinal{PbFinal: &pb.PbFinal{
// 			Id: uint64(id),
// 			Sid: uint64(sid),
// 			Proposal: proposal,
// 			Signature: signatureByte,
// 		}}
// 		break
// 	}
// 	return msg
// }

func (a *ParallelImpl) SafeExit() {
	close(a.MsgEntrance)
}

func (a *ParallelImpl) GetMsgEntrance() chan<- *pb.Msg {
	return a.MsgEntrance
}

func (a *ParallelImpl) GetSelfInfo() *config.ReplicaInfo {
	self := &config.ReplicaInfo{}
	for _, info := range a.Config.Cluster {
		if info.ID == a.ID {
			self = info
			break
		}
	}
	return self
}

func (a *ParallelImpl) GetNetworkInfo() map[uint32]string {
	networkInfo := make(map[uint32]string)
	for _, info := range a.Config.Cluster {
		if info.ID == a.ID {
			continue
		}
		networkInfo[info.ID] = info.Address
	}
	return networkInfo
}

func (a *ParallelImpl) Broadcast(msg *pb.Msg) error {
	infos := a.GetNetworkInfo()

	var broadcastErr error
	broadcastErr = nil
	for _, address := range infos {
		err := a.Unicast(address, msg)
		if err != nil {
			broadcastErr = err
		}
	}
	return broadcastErr
}

func (a *ParallelImpl) Unicast(address string, msg *pb.Msg) error {
	conn, err := grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		return err
	}
	defer conn.Close()
	client := pb.NewHotStuffServiceClient(conn)
	_, err = client.SendMsg(context.Background(), msg)
	if err != nil {
		return err
	}
	return nil
}

func (a *ParallelImpl) ProcessProposal(cmds []string) error {
	for _, cmd := range cmds {
		// result := a.ProcessMethod(cmd)
		// msg := &pb.Msg{Payload: &pb.Msg_Reply{Reply: &pb.Reply{Result: result, Command: cmd}}}
		msg := &pb.Msg{Payload: &pb.Msg_Reply{Reply: &pb.Reply{Result: "100", Command: cmd}}}
		err := a.Unicast("localhost:9999", msg)
		if err != nil {
			return err
		}
	}
	a.TxnSet.Remove(cmds...)
	return nil
}
