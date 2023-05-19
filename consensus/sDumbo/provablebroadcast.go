package sDumbo

import (
	"bytes"
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
	startProvableBroadcast(proposal []byte, proof []byte, j string, verfiyMethod func(int, int, []byte, []byte, *tcrsa.KeyMeta) bool)
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
	invoker     string // who invoke the PB
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

func (prb *ProvableBroadcastImpl) startProvableBroadcast(proposal []byte, proof []byte, j string, valueValidation func(int, int, []byte, []byte, *tcrsa.KeyMeta) bool) {
	logger.Info("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] Start Provable Broadcast " + j)

	prb.invoker = j
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
		
		// Create threshold signature share
		marshalData := getMsgdata(senderId, senderSid, senderProposal)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, prb.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, prb.acs.Config.PrivateKey, prb.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] pbValue: create the partial signature failed.")
		}

		// Verify the partSig form message
		if senderId == int(prb.acs.ID){
			//fmt.Println("partSig1:")
			//fmt.Println(partSig)
			err = go_hotstuff.VerifyPartSig(partSig, prb.DocumentHash, prb.acs.Config.PublicKey)
			if err != nil {
				data := getMsgdata(int(prb.acs.ID), prb.acs.Sid, prb.proposal)
				fmt.Println("compare")
				fmt.Println(bytes.Compare(senderProposal, prb.proposal))
				fmt.Println(bytes.Compare(marshalData, data))
				fmt.Println("data")
				fmt.Println(int(prb.acs.ID))
				fmt.Println(prb.acs.Sid)
				fmt.Println(senderId)
				fmt.Println(senderSid)

				logger.WithFields(logrus.Fields{
					"error":        err.Error(),
					"documentHash1": hex.EncodeToString(prb.DocumentHash),
					"documentHash2": hex.EncodeToString(documentHash),
					"senderId":  senderId,
					"senderSid": senderSid,
					// "proposal": hex.EncodeToString(prb.proposal),
					// "senderProposalHash": hex.EncodeToString(senderProposal),
				}).Warn("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] ???: signature share not verified!")
				return
			}
		}
		

		partSigBytes, _ := json.Marshal(partSig)
		pbEchoMsg := prb.acs.PbEchoMsg(int(prb.acs.ID), senderSid, senderProposal, partSigBytes)
		if uint32(senderId) != prb.acs.ID{
			// reply echo msg to sender
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
		// Ignore messages when echo is enough
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
		// Ignore messagemessages from other provable broadcast instance
		senderProposal := pbEchoMsg.Proposal
		if bytes.Compare(senderProposal, prb.proposal) != 0 {
			logger.Warn("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] Get mismatch proposal PbEcho msg")
			return
		}
		logger.WithFields(logrus.Fields{
			"senderId":  int(pbEchoMsg.Id),
			"senderSid": senderSid,
		}).Info("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] Get pbEcho msg")
		
		// get the partSig form message
		partSig := &tcrsa.SigShare{}
		err := json.Unmarshal(pbEchoMsg.PartialSig, partSig)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal partSig failed.")
		}

		// Verify the partSig form message
		err = go_hotstuff.VerifyPartSig(partSig, prb.DocumentHash, prb.acs.Config.PublicKey)
		if err != nil {
			marshalData := getMsgdata(int(pbEchoMsg.Id), senderSid, senderProposal)
			documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, prb.acs.Config.PublicKey)
			// fmt.Println("partSig2:")
			// fmt.Println(partSig)
			logger.WithFields(logrus.Fields{
				"error":        err.Error(),
				"compare": bytes.Compare(prb.DocumentHash, documentHash),
				"mydocumentHash": hex.EncodeToString(prb.DocumentHash),
				"msgdocumentHash": hex.EncodeToString(documentHash),
				"senderId":  int(pbEchoMsg.Id),
				"senderSid": senderSid,
				// "proposal": hex.EncodeToString(prb.proposal),
				// "senderProposalHash": hex.EncodeToString(senderProposal),
			}).Warn("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] pbEcho: signature share not verified!")
			return
		}

		prb.EchoVote = append(prb.EchoVote, partSig)

		// Create full threshold signature when echo is enough
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
			prb.Signature = signature
			prb.complete = true
			prb.acs.taskSignal <- "getPbValue_" + prb.invoker
			logger.WithFields(logrus.Fields{
				"signature":    len(signature),
				"documentHash": len(prb.DocumentHash),
				"echoVote":     len(prb.EchoVote),
			}).Info("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] pbEcho: create signature")
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

