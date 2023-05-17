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
	// "fmt"
)

//var logger = logging.GetLogger()

type ProvableBroadcast interface {
	startProvableBroadcast(proposal []byte, proof []byte, verfiyMethod func(int, int, []byte, []byte, *tcrsa.KeyMeta) bool)
	handleProvableBroadcastMsg(msg *pb.Msg)
	getSignature() tcrsa.Signature
	getProposal() []byte
	getStatus() bool
}

type ProvableBroadcastImpl struct {
	//consensus.AsynchronousImpl
	acs *CommonSubsetImpl

	proposal    []byte
	proof       []byte
	valueVerfiy func(int, int, []byte, []byte, *tcrsa.KeyMeta) bool
	complete    bool
	// vectors       []MvbaInputVector
	// futureVectors []MvbaInputVector
	EchoVote     []*tcrsa.SigShare
	DocumentHash []byte
	Signature    tcrsa.Signature
}

func NewProvableBroadcast(acs *CommonSubsetImpl) *ProvableBroadcastImpl {
	prb := &ProvableBroadcastImpl{
		acs:      acs,
		complete: false,
	}
	return prb
}

func (prb *ProvableBroadcastImpl) startProvableBroadcast(proposal []byte, proof []byte, valueValidation func(int, int, []byte, []byte, *tcrsa.KeyMeta) bool) {
	logger.Info("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] Start Provable Broadcast")

	prb.EchoVote = make([]*tcrsa.SigShare, 0)
	prb.valueVerfiy = valueValidation
	// fmt.Println("valueValidation:")
	// fmt.Println(valueValidation)
	// fmt.Println(prb.valueVerfiy)
	prb.proposal = proposal
	prb.proof = proof

	id := int(prb.acs.ID)

	pbValueMsg := prb.acs.PbValueMsg(id, prb.acs.Sid, prb.proposal, prb.proof)

	// create msg hash
	data := getMsgdata(id, prb.acs.Sid, prb.proposal)
	prb.DocumentHash, _ = go_hotstuff.CreateDocumentHash(data, prb.acs.Config.PublicKey)

	// broadcast msg
	err := prb.acs.Broadcast(pbValueMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Broadcast failed.")
	}

	// vote self
	prb.acs.MsgEntrance <- pbValueMsg
}

func (prb *ProvableBroadcastImpl) handleProvableBroadcastMsg(msg *pb.Msg) {
	switch msg.Payload.(type) {
	case *pb.Msg_PbValue:
		pbValueMsg := msg.GetPbValue()
		senderId := int(pbValueMsg.Id)
		senderSid := int(pbValueMsg.Sid)
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Info("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] Get PbValue msg")
		senderProposal := pbValueMsg.Proposal
		// senderProof := pbValueMsg.Proof

		// external validity verification
		// fmt.Println(senderProposal)
		// fmt.Println(senderProof)
		// fmt.Println(prb.acs.Config.PublicKey)
		// fmt.Println(prb.valueVerfiy)
		// fmt.Printf("%T", prb.valueVerfiy)
		// fmt.Println("")
		// if prb.valueVerfiy(senderId, senderSid, senderProposal, senderProof, prb.acs.Config.PublicKey) == false {
		// 	return
		// }
		
		marshalData := getMsgdata(senderId, senderSid, senderProposal)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, prb.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, prb.acs.Config.PrivateKey, prb.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] pbValue: create the partial signature failed.")
		}
		partSigBytes, _ := json.Marshal(partSig)
		pbEchoMsg := prb.acs.PbEchoMsg(int(prb.acs.ID), senderSid, partSigBytes)
		if uint32(senderId) != prb.acs.ID{
			// reply msg to sender
			err = prb.acs.Unicast(prb.acs.GetNetworkInfo()[uint32(senderId)], pbEchoMsg)
			if err != nil {
				logger.WithField("error", err.Error()).Error("Unicast failed.")
			}
		} else{
			// echo self
			prb.acs.MsgEntrance <- pbEchoMsg
		}
		break
	case *pb.Msg_PbEcho:
		// logger.WithFields(logrus.Fields{
		// 	"len(prb.EchoVote):":  len(prb.EchoVote),
		// 	"complete:": prb.complete,
		// }).Info("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] Get pbEcho msg good")
		if len(prb.EchoVote) >= 2*prb.acs.Config.F+1 {
			break
		}
		pbEchoMsg := msg.GetPbEcho()
		// Ignore messages from old sid
		senderSid := int(pbEchoMsg.Sid)
		if senderSid != prb.acs.Sid {
			logger.WithFields(logrus.Fields{
				"senderId":  int(pbEchoMsg.Id),
				"senderSid": senderSid,
			}).Warn("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] Get mismatch sid PbEcho msg")
			break
		}
		logger.WithFields(logrus.Fields{
			"senderId":  int(pbEchoMsg.Id),
			"senderSid": senderSid,
		}).Info("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] Get pbEcho msg")

		partSig := &tcrsa.SigShare{}
		err := json.Unmarshal(pbEchoMsg.PartialSig, partSig)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal partSig failed.")
		}

		err = go_hotstuff.VerifyPartSig(partSig, prb.DocumentHash, prb.acs.Config.PublicKey)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error":        err.Error(),
				"documentHash": hex.EncodeToString(prb.DocumentHash),
			}).Warn("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] pbEcho: signature not verified!")
			return
		}

		prb.EchoVote = append(prb.EchoVote, partSig)

		if len(prb.EchoVote) == 2*prb.acs.Config.F+1 {
			signature, err := go_hotstuff.CreateFullSignature(prb.DocumentHash, prb.EchoVote, prb.acs.Config.PublicKey)
			if err != nil {
				logger.WithFields(logrus.Fields{
					"error":        err.Error(),
					"documentHash": hex.EncodeToString(prb.DocumentHash),
				}).Error("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] pbEcho: create signature failed!")
				return
			}
			// flag, err := go_hotstuff.TVerify(prb.acs.Config.PublicKey, signature, prb.DocumentHash)
			// if err != nil || flag == false {
			// 	logger.WithFields(logrus.Fields{
			// 		"error":        err.Error(),
			// 		"documentHash": hex.EncodeToString(prb.DocumentHash),
			// 		"signature": signature,
			// 	}).Error("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] pbEcho: create error signature!")
			// 	//return
			// }
			prb.complete = true
			prb.Signature = signature
			logger.WithFields(logrus.Fields{
				"signature":    len(signature),
				"documentHash": len(prb.DocumentHash),
				"echoVote":     len(prb.EchoVote),
			}).Info("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] pbEcho: create signature")
			prb.acs.taskSignal <- "getPbValue"
		}
		break
	default:
		logger.Warn("[PB] Receive unsupported msg")
	}
}

func (prb *ProvableBroadcastImpl) getSignature() tcrsa.Signature {
	if prb.complete == false {
		logger.Error("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] getSignature: Provable Broadcast is not complet")
		return nil
	}
	return prb.Signature
}

func (prb *ProvableBroadcastImpl) getProposal() []byte {
	return prb.proposal
}

func (prb *ProvableBroadcastImpl) getStatus() bool {
	return prb.complete
}

func getMsgdata(senderId int, senderSid int, sednerProposal []byte) []byte {
	type msgData struct {
		Id       int
		Sid      int
		Proposal []byte
	}
	data := &msgData{
		Id:       senderId,
		Sid:      senderSid,
		Proposal: sednerProposal,
	}
	marshal, _ := json.Marshal(data)
	return marshal
}
