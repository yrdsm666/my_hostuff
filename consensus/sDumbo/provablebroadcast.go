package sDumbo

import (
	//"bytes"
	//"context"
	"encoding/hex"
	"encoding/json"
	//"errors"
	//"github.com/golang/protobuf/proto"
	"github.com/niclabs/tcrsa"
	"github.com/sirupsen/logrus"
	//"github.com/syndtr/goleveldb/leveldb"
	go_hotstuff "github.com/wjbbig/go-hotstuff"
	"github.com/wjbbig/go-hotstuff/config"
	"github.com/wjbbig/go-hotstuff/consensus"
	//"github.com/wjbbig/go-hotstuff/logging"
	pb "github.com/wjbbig/go-hotstuff/proto"
	//"os"
	//"strconv"
	//"sync"
	"fmt"
)

//var logger = logging.GetLogger()

type ProvableBroadcast struct {
	consensus.AsynchronousImpl

	Sid           int

	Proposal      []byte
	EchoVote      []*tcrsa.SigShare
	DocumentHash  []byte
	DocumentHash2 []byte
	Signature     tcrsa.Signature
}

// sid: session id
func NewProvableBroadcast(id int, sid int, proposal []byte, config config.HotStuffConfig) *ProvableBroadcast {
	logger.Debugf("[PROVABLE BROADCAST] Start Provable Broadcast")
	prb := &ProvableBroadcast{
		Sid:           sid,
		Proposal:      proposal,
	}

	prb.ID = uint32(id)
	prb.Config = config
	prb.EchoVote = make([]*tcrsa.SigShare, 0)

	pbValueMsg := prb.Msg(pb.MsgType_PBVALUE, id, sid, proposal, nil)

	// create msg hash
	//fmt.Println("-------GOOD----------")
	data := getMsgdata(id, sid, proposal)
	//fmt.Println(data)
	//fmt.Printf("variable data is of type %T \n", data)
	prb.DocumentHash, _ = go_hotstuff.CreateDocumentHash(data, prb.Config.PublicKey)

	// broadcast msg
	err := prb.Broadcast(pbValueMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Broadcast failed.")
	}

	return prb
}

func (prb *ProvableBroadcast) handleProvableBroadcastMsg(msg *pb.Msg) {
	switch msg.Payload.(type) {
	case *pb.Msg_PbValue:
		pbValueMsg := msg.GetPbValue()
		logger.WithField("content", "").Debug("[ACS] Get PbValue msg.")
		var senderId int = int(pbValueMsg.Id)
		var senderSid int = int(pbValueMsg.Sid)
		senderProposal := pbValueMsg.Proposal
		fmt.Println(senderId)
		fmt.Println(senderSid)
		fmt.Println(senderProposal)
		fmt.Println("----------------1---------------")
		fmt.Println(prb.Config)
		// marshalData := getMsgdata(senderId, senderSid, prb.Proposal)
		fmt.Println("------------- --2---------------")
		// fmt.Println(prb.Config.PublicKey)
		// fmt.Println("----------------3---------------")
		// fmt.Printf("variable marshalData is of type %T \n", marshalData)

		// // defer func() {
		// // 	if r := recover(); r != nil {
		// // 		fmt.Println("Some error happened!", r)
		// // 		//ret = -1
		// // 	}
		// // }()
		// prb.DocumentHash2, _ = go_hotstuff.CreateDocumentHash(marshalData, prb.Config.PublicKey)
		// fmt.Println("----------------4---------------")
		// fmt.Printf("hash good\n %v\n", prb.DocumentHash2)
		// logger.WithField("content", "hahahha").Debug("HELLO.")
		// logger.WithField("content", prb.DocumentHash2).Debug("[ACS] Get PbValue msg.")
		// // if err != nil {
		// // 	logger.WithField("error", err.Error()).Error("create Document Hash failed.")
		// // }
		// partSig, err := go_hotstuff.TSign(prb.DocumentHash2, prb.Config.PrivateKey, prb.Config.PublicKey)
		// if err != nil {
		// 	logger.WithField("error", err.Error()).Error("create the partial signature failed.")
		// }
		// partSigBytes, _ := json.Marshal(partSig)
		// pbEchoMsg := prb.Msg(pb.MsgType_PBECHO, int(prb.ID), senderSid, nil, partSigBytes)
		// // // reply msg to sender
		// err = prb.Unicast(prb.GetNetworkInfo()[uint32(senderId)], pbEchoMsg)
		// if err != nil {
		// 	logger.WithField("error", err.Error()).Error("Unicast failed.")
		// }
		break
	case *pb.Msg_PbEcho:
		if len(prb.EchoVote) >= 2*prb.Config.F+1{
			break
		}
		pbEchoMsg := msg.GetPbEcho()
		logger.WithField("content", "").Debug("[ACS] Get pbEcho msg.")
		partSig := &tcrsa.SigShare{}
		err := json.Unmarshal(pbEchoMsg.SignShare, partSig)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal partSig failed.")
		}

		err = go_hotstuff.VerifyPartSig(partSig, prb.DocumentHash, prb.Config.PublicKey)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error":        err.Error(),
				"documentHash": hex.EncodeToString(prb.DocumentHash),
			}).Warn("[PROVABLE BROADCAST] pbEchoVote: signature not verified!")
			return
		}

		prb.EchoVote = append(prb.EchoVote, partSig)

		if len(prb.EchoVote) == 2*prb.Config.F+1 {
			signature, _ := go_hotstuff.CreateFullSignature(prb.DocumentHash,  prb.EchoVote, prb.Config.PublicKey)
			prb.Signature = signature
			marshal, _ := json.Marshal(signature)
			pbFinalMsg := prb.Msg(pb.MsgType_PBFINAL, int(prb.ID), prb.Sid, prb.Proposal, marshal)
			// broadcast msg
			err := prb.Broadcast(pbFinalMsg)
			if err != nil {
				logger.WithField("error", err.Error()).Error("Broadcast failed.")
			}
		
		}
		break
	default:
		logger.Warn("[PROVABLE BROADCAST] Receive unsupported msg")
	}
}

func getMsgdata (senderId int, senderSid int, sednerProposal []byte) []byte {
	type msgData struct{
		Id         int
		Sid        int
		Proposal   []byte
	}
	data := &msgData{
		Id:         senderId,
		Sid:        senderSid,
		Proposal:   sednerProposal,
	}
	marshal, _ := json.Marshal(data)
	return marshal
}
