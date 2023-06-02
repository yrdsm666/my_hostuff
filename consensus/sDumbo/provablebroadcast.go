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
	"sync"
	// "fmt"
)

type ProvableBroadcast interface {
	startProvableBroadcast(proposal []byte, proof []byte, j string, verfiyMethod func(int, int, []byte, []byte, *tcrsa.KeyMeta) bool)
	handleProvableBroadcastMsg(msg *pb.Msg)
	getSignature() tcrsa.Signature
	getProposal() []byte
	getProposalHash() []byte
	getStatus() bool
	getLockVectors() []Vector
	getValueVectors() map[int][]byte
}

type ProvableBroadcastImpl struct {
	acs *CommonSubsetImpl

	proposal      []byte
	proof         []byte
	valueVerfiy   func(int, int, []byte, []byte, *tcrsa.KeyMeta) bool
	complete      bool
	invokePhase   string // which phase invoke the PB
	EchoVote      []*tcrsa.SigShare
	lockVectors   []Vector
	valueVectors  map[int][]byte
	DocumentHash  []byte
	proposalHash  []byte
	Signature     tcrsa.Signature
	lockSet       sync.Mutex
}

func NewProvableBroadcast(acs *CommonSubsetImpl) *ProvableBroadcastImpl {
	prb := &ProvableBroadcastImpl{
		acs:      acs,
		complete: false,
	}
	// NOTES:
	// valueVectors needs to be initialized as early as possible.
	// Assuming valueVectors is initialized in the startProvableBroadcast function,
	// if a pbvalue message is received before starting Provable Broadcast, 
	// the value of pbvalue message will not be stored in valueVectors.
	// 注意：
	// valueVectors 需要尽可能早的初始化。
	// 假设 valueVectors 在 startProvableBroadcast 函数中被初始化，
	// 此时如果在 startProvableBroadcast 之前接收到了 pbvalue 消息，
	// 那么 pbvalue 消息的值不会被存储进入 valueVectors
	prb.valueVectors = make(map[int][]byte)
	return prb
}

func (prb *ProvableBroadcastImpl) startProvableBroadcast(proposal []byte, proof []byte, invokePhase string, valueValidation func(int, int, []byte, []byte, *tcrsa.KeyMeta) bool) {
	logger.Info("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] Start Provable Broadcast " + invokePhase)

	prb.invokePhase = invokePhase
	prb.EchoVote = make([]*tcrsa.SigShare, 0)
	prb.lockSet.Lock()
	prb.lockVectors = make([]Vector, 0)
	prb.lockSet.Unlock()
	prb.valueVerfiy = valueValidation
	// fmt.Println("valueValidation:")
	// fmt.Println(valueValidation)
	// fmt.Println(prb.valueVerfiy)
	prb.proposal = proposal
	prb.proof = proof

	// create msg 
	id := int(prb.acs.ID)
	pbValueMsg := prb.acs.PbValueMsg(id, prb.acs.Sid, invokePhase, prb.proposal, prb.proof)

	prb.proposalHash, _ = go_hotstuff.CreateDocumentHash(prb.proposal, prb.acs.Config.PublicKey)
	data := getMsgdata(id, prb.acs.Sid, prb.proposalHash)
	prb.DocumentHash, _ = go_hotstuff.CreateDocumentHash(data, prb.acs.Config.PublicKey)
	
	// broadcast msg
	err := prb.acs.Broadcast(pbValueMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Warn("Broadcast pbValueMsg failed.")
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
		invokePhase := pbValueMsg.InvokePhase
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

		// Collect PB_1 proof in SPB
		if prb.invokePhase == "2" && invokePhase == prb.invokePhase {
			// get the proof form message
			proof := &tcrsa.Signature{}
			err := json.Unmarshal(pbValueMsg.Proof, proof)
			if err != nil {
				logger.WithField("error", err.Error()).Error("Unmarshal partSig failed.")
			}
			// verify the proof
			initProposal := bytesSub(senderProposal, []byte(SPB_PHASE_2))
			newProposal := bytesAdd(initProposal, []byte(SPB_PHASE_1))
			proposalHash, _ := go_hotstuff.CreateDocumentHash(newProposal, prb.acs.Config.PublicKey)
			marshalData := getMsgdata(senderId, senderSid, proposalHash)
			flag, err := go_hotstuff.TVerify(prb.acs.Config.PublicKey, *proof, marshalData)
			if err != nil || flag == false {
				logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(int(prb.acs.Sid)) + "] [PB] pbValue: verfiy proof of SPB_1 failed in SPB_2.")
				return
			}
			lockVector := Vector{
				Id:        senderId,
				Sid:       senderSid,
				Proposal:  initProposal,
				Signature: *proof,
			}
			prb.lockSet.Lock()
			prb.lockVectors = append(prb.lockVectors, lockVector)
			prb.lockSet.Unlock()
		}

		// Collect PB values in ACS
		if invokePhase == PB_PHASE {
			prb.valueVectors[senderId] = senderProposal
			logger.WithFields(logrus.Fields{
				"senderId":  senderId,
				"senderSid": senderSid,
				"len(senderProposal):": len(senderProposal),
			}).Info("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] Save value of PbValue msg")
		}
		
		// Create threshold signature share
		// Perform threshold signature on the proposed Hash
		proposalHash, _ := go_hotstuff.CreateDocumentHash(senderProposal, prb.acs.Config.PublicKey)
		marshalData := getMsgdata(senderId, senderSid, proposalHash)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, prb.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, prb.acs.Config.PrivateKey, prb.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] pbValue: create the partial signature failed.")
		}
		
		// create msg 
		partSigBytes, _ := json.Marshal(partSig)
		pbEchoMsg := prb.acs.PbEchoMsg(int(prb.acs.ID), senderSid, prb.invokePhase, proposalHash, partSigBytes)
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
			return
		}
		
		pbEchoMsg := msg.GetPbEcho()
		senderSid := int(pbEchoMsg.Sid)

		// Ignore messages from old sid
		if senderSid != prb.acs.Sid {
			logger.WithFields(logrus.Fields{
				"senderId":  int(pbEchoMsg.Id),
				"senderSid": senderSid,
			}).Warn("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] Get mismatch sid PbEcho msg")
			return
		}

		// Ignore messages from other provable broadcast instance
		senderProposal := pbEchoMsg.Proposal
		if bytes.Compare(senderProposal, prb.proposalHash) != 0 {
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
			logger.WithFields(logrus.Fields{
				"error":        err.Error(),
				"mydocumentHash": hex.EncodeToString(prb.DocumentHash),
				"senderId":  int(pbEchoMsg.Id),
				"senderSid": senderSid,
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
				}).Error("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] pbEcho: create full signature failed!")
				return
			}
			prb.Signature = signature
			prb.complete = true
			prb.acs.taskSignal <- "getPbValue_" + prb.invokePhase
			logger.WithFields(logrus.Fields{
				"signature":    len(signature),
				"documentHash": len(prb.DocumentHash),
				"echoVote":     len(prb.EchoVote),
			}).Info("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] pbEcho: create full signature")
			// if len(signature) != 256 {
			// 	logger.WithFields(logrus.Fields{
			// 		"signature":    hex.EncodeToString(signature),
			// 	}).Error("[replica_" + strconv.Itoa(int(prb.acs.ID)) + "] [sid_" + strconv.Itoa(prb.acs.Sid) + "] [PB] pbEcho: create full signature")
			// }
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

func (prb *ProvableBroadcastImpl) getProposalHash() []byte {
	return prb.proposalHash
}

func (prb *ProvableBroadcastImpl) getStatus() bool {
	return prb.complete
}

func (prb *ProvableBroadcastImpl) getLockVectors() []Vector {
	return prb.lockVectors
}

func (prb *ProvableBroadcastImpl) getValueVectors() map[int][]byte {
	return prb.valueVectors
}

