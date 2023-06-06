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
type Asynchronous interface {
	//Msg(msgType pb.MsgType, id int, round int, sid int, proposal []byte, signatureByte []byte) *pb.Msg
	GetMsgEntrance() chan<- *pb.Msg
	GetNetworkInfo() map[uint32]string
	GetSelfInfo() *config.ReplicaInfo
	SafeExit()
	Broadcast(msg *pb.Msg) error
	Unicast(address string, msg *pb.Msg) error
	ProcessProposal(cmds []string) error

	PbValueMsg(id int, round int, sid int, invokePhase string, proposal []byte, proof []byte) *pb.Msg
	PbEchoMsg(id int, round int, sid int, invokePhase string, proposal []byte, partialSig []byte) *pb.Msg
	PbFinalMsg(id int, round int, proposal []byte, signature []byte) *pb.Msg
	CoinShareMsg(id int, round int, sid int, sigShare []byte) *pb.Msg
	SpbFinalMsg(id int, round int, sid int, proposal []byte, signature []byte) *pb.Msg
	DoneMsg(id int, round int, sid int) *pb.Msg
	HaltMsg(id int, round int, sid int, final []byte) *pb.Msg
	PreVoteMsg(id int, round int, sid int, leader int, flag int, proposal []byte, signature []byte, partialSig []byte) *pb.Msg
	VoteMsg(id int, round int, sid int, leader int, flag int, proposal []byte, signature []byte, partialSig []byte) *pb.Msg
}

type AsynchronousImpl struct {
	ID            uint32
	MsgEntrance   chan *pb.Msg // receive msg
	Config        config.HotStuffConfig
	TxnSet        go_hotstuff.CmdSet
}

// func (a *AsynchronousImpl) Msg(msgType pb.MsgType, id int, round int, sid int, proposal []byte, signatureByte []byte) *pb.Msg {
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

func (a *AsynchronousImpl) SafeExit() {
	close(a.MsgEntrance)
}

func (a *AsynchronousImpl) GetMsgEntrance() chan<- *pb.Msg {
	return a.MsgEntrance
}

func (a *AsynchronousImpl) GetSelfInfo() *config.ReplicaInfo {
	self := &config.ReplicaInfo{}
	for _, info := range a.Config.Cluster {
		if info.ID == a.ID {
			self = info
			break
		}
	}
	return self
}

func (a *AsynchronousImpl) GetNetworkInfo() map[uint32]string {
	networkInfo := make(map[uint32]string)
	for _, info := range a.Config.Cluster {
		if info.ID == a.ID {
			continue
		}
		networkInfo[info.ID] = info.Address
	}
	return networkInfo
}

func (a *AsynchronousImpl) Broadcast(msg *pb.Msg) error {
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

func (a *AsynchronousImpl) Unicast(address string, msg *pb.Msg) error {
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

func (a *AsynchronousImpl) ProcessProposal(cmds []string) error {
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

func (a *AsynchronousImpl) PbValueMsg(id int, round int, sid int, invokePhase string, proposal []byte, proof []byte) *pb.Msg {
	msg := &pb.Msg{}
	msg.Payload = &pb.Msg_PbValue{PbValue: &pb.PbValue{
		Id: uint64(id),
		Round: uint64(round),
		Sid: uint64(sid),
		InvokePhase: invokePhase,
		Proposal: proposal,
		Proof: proof,
	}}
	return msg
}

func (a *AsynchronousImpl) PbEchoMsg(id int, round int, sid int, invokePhase string, proposal []byte, partialSig []byte) *pb.Msg {
	msg := &pb.Msg{}
	msg.Payload = &pb.Msg_PbEcho{PbEcho: &pb.PbEcho{
		Id: uint64(id),
		Round: uint64(round),
		Sid: uint64(sid),
		InvokePhase: invokePhase,
		Proposal: proposal,
		PartialSig: partialSig,
	}}
	return msg
}

func (a *AsynchronousImpl) PbFinalMsg(id int, round int, proposal []byte, signature []byte) *pb.Msg {
	msg := &pb.Msg{}
	msg.Payload = &pb.Msg_PbFinal{PbFinal: &pb.PbFinal{
		Id: uint64(id),
		Round: uint64(round),
		Proposal: proposal,
		Signature: signature,
	}}
	return msg
}

func (a *AsynchronousImpl) CoinShareMsg(id int, round int, sid int, partialSig []byte) *pb.Msg {
	msg := &pb.Msg{}
	msg.Payload = &pb.Msg_CoinShare{CoinShare: &pb.CoinShare{
		Id: uint64(id),
		Round: uint64(round),
		Sid: uint64(sid),
		PartialSig: partialSig,
	}}
	return msg
}

func (a *AsynchronousImpl) SpbFinalMsg(id int, round int, sid int, proposal []byte, signature []byte) *pb.Msg {
	msg := &pb.Msg{}
	msg.Payload = &pb.Msg_SpbFinal{SpbFinal: &pb.SpbFinal{
		Id: uint64(id),
		Round: uint64(round),
		Sid: uint64(sid),
		Proposal: proposal,
		Signature: signature,
	}}
	return msg
}

func (a *AsynchronousImpl) DoneMsg(id int, round int, sid int) *pb.Msg {
	msg := &pb.Msg{}
	msg.Payload = &pb.Msg_Done{Done: &pb.Done{
		Id: uint64(id),
		Round: uint64(round),
		Sid: uint64(sid),
	}}
	return msg
}

func (a *AsynchronousImpl) HaltMsg(id int, round int, sid int, final []byte) *pb.Msg {
	msg := &pb.Msg{}
	msg.Payload = &pb.Msg_Halt{Halt: &pb.Halt{
		Id: uint64(id),
		Round: uint64(round),
		Sid: uint64(sid),
		Final: final,
	}}
	return msg
}

func (a *AsynchronousImpl) PreVoteMsg(id int, round int, sid int, leader int, flag int, proposal []byte, signature []byte, partialSig []byte) *pb.Msg {
	msg := &pb.Msg{}
	msg.Payload = &pb.Msg_PreVote{PreVote: &pb.PreVote{
		Id: uint64(id),
		Round: uint64(round),
		Sid: uint64(sid),
		Leader: uint64(leader),
		Flag: uint64(flag),
		Proposal: proposal,
		Signature: signature,
		PartialSig: partialSig,
	}}
	return msg
}

func (a *AsynchronousImpl) VoteMsg(id int, round int, sid int, leader int, flag int, proposal []byte, signature []byte, partialSig []byte) *pb.Msg {
	msg := &pb.Msg{}
	msg.Payload = &pb.Msg_Vote{Vote: &pb.Vote{
		Id: uint64(id),
		Round: uint64(round),
		Sid: uint64(sid),
		Leader: uint64(leader),
		Flag: uint64(flag),
		Proposal: proposal,
		Signature: signature,
		PartialSig: partialSig,
	}}
	return msg
}