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
	//"github.com/wjbbig/go-hotstuff/config"
	//"github.com/wjbbig/go-hotstuff/consensus"
	//"github.com/wjbbig/go-hotstuff/logging"
	pb "github.com/wjbbig/go-hotstuff/proto"
	//"os"
	"strconv"
	//"sync"
	"fmt"
)

//var logger = logging.GetLogger()

type ProvableBroadcast interface {
	startProvableBroadcast()
	handleProvableBroadcastMsg(msg *pb.Msg)
}

type ProvableBroadcastImpl struct {
	//consensus.AsynchronousImpl
	acs           *CommonSubsetImpl
	//Sid           int

	//Proposal      []byte
	// vectors       []MvbaInputVector
	// futureVectors []MvbaInputVector
	EchoVote      []*tcrsa.SigShare
	DocumentHash  []byte
	//DocumentHash2 []byte
	Signature     tcrsa.Signature
}

func NewProvableBroadcast(acs *CommonSubsetImpl) *ProvableBroadcastImpl {
	prb := &ProvableBroadcastImpl{
		acs:             acs,
		//Proposal:      proposal,
	}
	return prb
}

func (prb *ProvableBroadcastImpl) startProvableBroadcast() {
	logger.Info("[replica_"+strconv.Itoa(int(prb.acs.ID))+"] [sid_"+strconv.Itoa(prb.acs.Sid)+"] [PB] Start Provable Broadcast")

	prb.EchoVote = make([]*tcrsa.SigShare, 0)

	id := int(prb.acs.ID)

	pbValueMsg := prb.acs.Msg(pb.MsgType_PBVALUE, id, prb.acs.Sid, prb.acs.proposalHash, nil)

	// create msg hash
	data := getMsgdata(id, prb.acs.Sid, prb.acs.proposalHash)
	prb.DocumentHash, _ = go_hotstuff.CreateDocumentHash(data, prb.acs.Config.PublicKey)

	// broadcast msg
	err := prb.acs.Broadcast(pbValueMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Broadcast failed.")
	}

	// vote self
	// prb.acs.MsgEntrance <- pbValueMsg
}

func (prb *ProvableBroadcastImpl) handleProvableBroadcastMsg(msg *pb.Msg) {
	switch msg.Payload.(type) {
	case *pb.Msg_PbValue:
		pbValueMsg := msg.GetPbValue()
		senderId := int(pbValueMsg.Id)
		senderSid := int(pbValueMsg.Sid)
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid":  senderSid,
		}).Info("[replica_"+strconv.Itoa(int(prb.acs.ID))+"] [sid_"+strconv.Itoa(prb.acs.Sid)+"] [PB] Get PbValue msg")

		senderProposalHash := pbValueMsg.Proposal
		marshalData := getMsgdata(senderId, senderSid, senderProposalHash)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, prb.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, prb.acs.Config.PrivateKey, prb.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("create the partial signature failed.")
		}
		partSigBytes, _ := json.Marshal(partSig)
		pbEchoMsg := prb.acs.Msg(pb.MsgType_PBECHO, int(prb.acs.ID), senderSid, nil, partSigBytes)
		// reply msg to sender
		err = prb.acs.Unicast(prb.acs.GetNetworkInfo()[uint32(senderId)], pbEchoMsg)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unicast failed.")
		}
		// echo self

		break
	case *pb.Msg_PbEcho:
		if len(prb.EchoVote) >= 2*prb.acs.Config.F+1{
			break
		}
		pbEchoMsg := msg.GetPbEcho()
		// Ignore messages from old sid
		senderSid := int(pbEchoMsg.Sid)
		if senderSid != prb.acs.Sid{
			logger.WithFields(logrus.Fields{
				"senderId":  int(pbEchoMsg.Id),
				"senderSid":  senderSid,
			}).Warn("[replica_"+strconv.Itoa(int(prb.acs.ID))+"] [sid_"+strconv.Itoa(prb.acs.Sid)+"] [PB] Get mismatch PbEcho msg")
			break
		}
		logger.WithFields(logrus.Fields{
			"senderId":  int(pbEchoMsg.Id),
			"senderSid":  senderSid,
		}).Info("[replica_"+strconv.Itoa(int(prb.acs.ID))+"] [sid_"+strconv.Itoa(prb.acs.Sid)+"] [PB] Get pbEcho msg")

		partSig := &tcrsa.SigShare{}
		err := json.Unmarshal(pbEchoMsg.SignShare, partSig)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal partSig failed.")
		}

		err = go_hotstuff.VerifyPartSig(partSig, prb.DocumentHash, prb.acs.Config.PublicKey)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error":        err.Error(),
				"documentHash": hex.EncodeToString(prb.DocumentHash),
			}).Warn("[replica_"+strconv.Itoa(int(prb.acs.ID))+"] [sid_"+strconv.Itoa(prb.acs.Sid)+"] [PB] pbEchoVote: signature not verified!")
			return
		}

		prb.EchoVote = append(prb.EchoVote, partSig)

		if len(prb.EchoVote) == 2*prb.acs.Config.F+1 {
			signature, err := go_hotstuff.CreateFullSignature(prb.DocumentHash,  prb.EchoVote, prb.acs.Config.PublicKey)
			if err != nil {
				logger.WithFields(logrus.Fields{
					"error":        err.Error(),
					"documentHash": hex.EncodeToString(prb.DocumentHash),
				}).Error("[replica_"+strconv.Itoa(int(prb.acs.ID))+"] [sid_"+strconv.Itoa(prb.acs.Sid)+"] [PB] pbEchoVote: create signature failed!")
			}
			prb.Signature = signature
			logger.WithFields(logrus.Fields{
				"signature":        len(signature),
				"documentHash": len(prb.DocumentHash),
				"echoVote": len(prb.EchoVote),
			}).Warn("[replica_"+strconv.Itoa(int(prb.acs.ID))+"] [sid_"+strconv.Itoa(prb.acs.Sid)+"] [PB] pbEchoVote: create signature")
			marshal, _ := json.Marshal(signature)
			pbFinalMsg := prb.acs.Msg(pb.MsgType_PBFINAL, int(prb.acs.ID), prb.acs.Sid, prb.acs.proposalHash, marshal)
			// broadcast msg
			err = prb.acs.Broadcast(pbFinalMsg)
			if err != nil {
				logger.WithField("error", err.Error()).Error("Broadcast failed.")
			}
			// send to self
			
		}
		break
	case *pb.Msg_PbFinal:
		pbFinal := msg.GetPbFinal()
		senderId := int(pbFinal.Id)
		senderSid := int(pbFinal.Sid)
		senderProposalHash := pbFinal.Proposal
		// Ignore messages from old sid
		if senderSid < prb.acs.Sid{
			logger.WithFields(logrus.Fields{
				"senderId":  senderId,
				"senderSid":  senderSid,
			}).Warn("[replica_"+strconv.Itoa(int(prb.acs.ID))+"] [sid_"+strconv.Itoa(prb.acs.Sid)+"] [PB] Get mismatch Pbfinal msg")
			break
		}
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid":  senderSid,
		}).Info("[replica_"+strconv.Itoa(int(prb.acs.ID))+"] [sid_"+strconv.Itoa(int(prb.acs.Sid))+"] [PB] Get PbFinal msg")
		signature := &tcrsa.Signature{}
		err := json.Unmarshal(pbFinal.Signature, signature)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
		}
		marshalData := getMsgdata(senderId, senderSid, senderProposalHash)
		flag, err := go_hotstuff.TVerify(prb.acs.Config.PublicKey, *signature, marshalData)
		if ( err != nil || flag==false ) {
			logger.WithField("error", err.Error()).Error("[replica_"+strconv.Itoa(int(prb.acs.ID))+"] [sid_"+strconv.Itoa(int(prb.acs.Sid))+"] verfiy signature failed.")
		}
		wVector := MvbaInputVector{
			id:            senderId,
			sid:           senderSid,
			proposalHash:  senderProposalHash,
			Signature:     *signature,
		}
		if senderSid > prb.acs.Sid {
			prb.acs.futureVectorsCache = append(prb.acs.vectors, wVector)
		} else {
			prb.acs.vectors = append(prb.acs.vectors, wVector)
		} 
		if len(prb.acs.vectors) == 2*prb.acs.Config.F+1 {
			fmt.Println("")
			fmt.Println("---------------- [ACS] -----------------")
			fmt.Println("副本：", prb.acs.ID)
			for i := 0; i < 2*prb.acs.Config.F+1; i++{
				fmt.Println("node: ", prb.acs.vectors[i].id)
				fmt.Println("Sid: ", prb.acs.vectors[i].sid)
				fmt.Println("proposalHashLen: ",len(prb.acs.vectors[i].proposalHash))
			}
			fmt.Println("[ACS] GOOD WORK!.")
			fmt.Println("---------------- [ACS] -----------------")
			// 需要保证旧实例结束再启动新实例
			// 不能同时执行新旧实例，代码不允许
			// 达到新实例的启动条件时，仍然要执行旧实例，不然其他节点可能无法在上一个实例中结束
			// 也就是说，当节点广播完上一个实例的的pbfinal消息后，才可启动新实例
			//go acs.startNewInstance()
			// start a new instance if the conditions are met
			// acs.msgEndFlag = true
			// if acs.taskEndFlag == true {
			// 	acs.restart <- true
			// }
			prb.acs.taskSignal <- "ProvableBroadcastFinal"
		}
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
