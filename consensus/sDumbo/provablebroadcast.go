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
	startProvableBroadcast(proposal []byte, proof []byte, sid int, invokePhase string, verfiyMethod func(int, int, int, []byte, []byte, *tcrsa.KeyMeta) bool)
	handleProvableBroadcastMsg(msg *pb.Msg)
	getSignature() tcrsa.Signature
	getProposal() []byte
	getProposalHash() []byte
	getStatus() bool
	getLockVectors() []Vector
}

type ProvableBroadcastImpl struct {
	acs *CommonSubsetImpl

	complete bool

	// Provable Broadcast information
	proposal     []byte
	proof        []byte
	proposalHash []byte
	DocumentHash []byte

	sid         int
	invokePhase string // which phase invoke the PB
	valueVerfiy func(int, int, int, []byte, []byte, *tcrsa.KeyMeta) bool

	EchoVote    []*tcrsa.SigShare
	lockVectors []Vector

	Signature tcrsa.Signature
	lockSet   sync.Mutex
}

func NewProvableBroadcast(acs *CommonSubsetImpl) *ProvableBroadcastImpl {
	prb := &ProvableBroadcastImpl{
		acs:      acs,
		complete: false,
	}
	return prb
}

// proposal: Provable Broadcast proposal
// proof: proof of proposal
// invokePhase: identify which phase called PB
// valueValidation: external validation function
func (prb *ProvableBroadcastImpl) startProvableBroadcast(proposal []byte, proof []byte, sid int, invokePhase string, valueValidation func(int, int, int, []byte, []byte, *tcrsa.KeyMeta) bool) {
	logger.Info("[p_" + strconv.Itoa(int(prb.acs.ID)) + "] [r_" + strconv.Itoa(prb.acs.round) + "] [PB] Start Provable Broadcast " + invokePhase)

	prb.proposal = proposal
	prb.proof = proof
	prb.sid = sid
	prb.invokePhase = invokePhase
	prb.valueVerfiy = valueValidation
	// fmt.Println("valueValidation:")
	// fmt.Println(valueValidation)
	// fmt.Println(prb.valueVerfiy)

	prb.EchoVote = make([]*tcrsa.SigShare, 0)
	prb.lockSet.Lock()
	prb.lockVectors = make([]Vector, 0)
	prb.lockSet.Unlock()

	// create msg
	id := int(prb.acs.ID)
	pbValueMsg := prb.acs.PbValueMsg(id, prb.acs.round, sid, invokePhase, proposal, proof)

	prb.proposalHash, _ = go_hotstuff.CreateDocumentHash(prb.proposal, prb.acs.Config.PublicKey)
	data := getMsgdata(id, prb.acs.round, sid, prb.proposalHash)
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
		senderRound := int(pbValueMsg.Round)
		senderSid := int(pbValueMsg.Sid)
		invokePhase := pbValueMsg.InvokePhase
		senderProposal := pbValueMsg.Proposal
		// senderProof := pbValueMsg.Proof

		logger.WithFields(logrus.Fields{
			"senderId":    senderId,
			"senderRound": senderRound,
			"senderSid":   senderSid,
		}).Info("[p_" + strconv.Itoa(int(prb.acs.ID)) + "] [r_" + strconv.Itoa(prb.acs.round) + "] [PB] Get PbValue msg")

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
		if prb.invokePhase == SPB_PHASE_2 && invokePhase == prb.invokePhase {
			// get the proof form message
			proof := &tcrsa.Signature{}
			err := json.Unmarshal(pbValueMsg.Proof, proof)
			if err != nil {
				logger.WithField("error", err.Error()).Error("Unmarshal partSig failed.")
			}
			// verify the proof
			initProposal := bytesSub(senderProposal, []byte(SPB_PHASE_2))
			flag, err := verfiySpbSig(senderId, senderRound, senderSid, []byte(SPB_PHASE_1), initProposal, *proof, prb.acs.Config.PublicKey)
			if err != nil || !flag {
				logger.WithField("error", err.Error()).Error("[p_" + strconv.Itoa(int(prb.acs.ID)) + "] [r_" + strconv.Itoa(prb.acs.round) + "] [PB] pbValue: verfiy proof of SPB_1 failed in SPB_2.")
				return
			}

			lockVector := Vector{
				Id:        senderId,
				Round:     senderRound,
				Sid:       senderSid,
				Proposal:  initProposal,
				Signature: *proof,
			}
			prb.lockSet.Lock()
			prb.lockVectors = append(prb.lockVectors, lockVector)
			prb.lockSet.Unlock()
		}

		// Collect PB values in ACS
		if invokePhase == PB_PHASE && senderRound >= prb.acs.round {
			prb.acs.insertValue(senderId, senderRound, senderProposal)
			// logger.WithFields(logrus.Fields{
			// 	"senderId":               senderId,
			// 	"senderRound":            senderRound,
			// 	"senderSid":              senderSid,
			// 	"len(senderProposal):":   len(senderProposal),
			// 	"len(value):":            len(prb.acs.valueVectors),
			// 	"len(value[senderSid]):": len(prb.acs.valueVectors[senderRound]),
			// }).Info("[p_" + strconv.Itoa(int(prb.acs.ID)) + "] [r_" + strconv.Itoa(prb.acs.round) + "] [PB] Save value of PbValue msg")
		}

		// Create threshold signature share
		// Perform threshold signature on the proposed Hash
		proposalHash, _ := go_hotstuff.CreateDocumentHash(senderProposal, prb.acs.Config.PublicKey)
		marshalData := getMsgdata(senderId, senderRound, senderSid, proposalHash)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, prb.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, prb.acs.Config.PrivateKey, prb.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("[p_" + strconv.Itoa(int(prb.acs.ID)) + "] [r_" + strconv.Itoa(prb.acs.round) + "] [PB] pbValue: create the partial signature failed.")
		}

		// create msg
		partSigBytes, _ := json.Marshal(partSig)
		pbEchoMsg := prb.acs.PbEchoMsg(int(prb.acs.ID), senderRound, senderSid, prb.invokePhase, proposalHash, partSigBytes)
		if uint32(senderId) != prb.acs.ID {
			// reply echo msg to sender
			err = prb.acs.Unicast(prb.acs.GetNetworkInfo()[uint32(senderId)], pbEchoMsg)
			if err != nil {
				logger.WithField("error", err.Error()).Error("Unicast failed.")
			}
		} else {
			// echo self
			prb.acs.MsgEntrance <- pbEchoMsg
		}
	case *pb.Msg_PbEcho:
		// Ignore messages when echo is enough
		if len(prb.EchoVote) >= 2*prb.acs.Config.F+1 {
			return
		}

		pbEchoMsg := msg.GetPbEcho()
		senderRound := int(pbEchoMsg.Round)
		senderSid := int(pbEchoMsg.Sid)

		// Ignore messages from old round or sid
		if senderRound != prb.acs.round || senderSid != prb.sid {
			logger.WithFields(logrus.Fields{
				"senderId":    int(pbEchoMsg.Id),
				"senderRound": senderRound,
				"senderSid":   senderSid,
			}).Warn("[p_" + strconv.Itoa(int(prb.acs.ID)) + "] [r_" + strconv.Itoa(prb.acs.round) + "] [PB] Get mismatch sid or round PbEcho msg")
			return
		}

		// Ignore messages from other provable broadcast instance
		senderProposal := pbEchoMsg.Proposal
		if !bytes.Equal(senderProposal, prb.proposalHash) {
			logger.Warn("[p_" + strconv.Itoa(int(prb.acs.ID)) + "] [r_" + strconv.Itoa(prb.acs.round) + "] [PB] Get mismatch proposal PbEcho msg")
			return
		}

		logger.WithFields(logrus.Fields{
			"senderId":  int(pbEchoMsg.Id),
			"senderSid": senderSid,
		}).Info("[p_" + strconv.Itoa(int(prb.acs.ID)) + "] [r_" + strconv.Itoa(prb.acs.round) + "] [PB] Get pbEcho msg")

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
				"error":          err.Error(),
				"mydocumentHash": hex.EncodeToString(prb.DocumentHash),
				"senderId":       int(pbEchoMsg.Id),
				"senderRound":    senderRound,
				"senderSid":      senderSid,
			}).Warn("[p_" + strconv.Itoa(int(prb.acs.ID)) + "] [r_" + strconv.Itoa(prb.acs.round) + "] [PB] pbEcho: signature share not verified!")
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
				}).Error("[p_" + strconv.Itoa(int(prb.acs.ID)) + "] [r_" + strconv.Itoa(prb.acs.round) + "] [PB] pbEcho: create full signature failed!")
				return
			}
			prb.Signature = signature
			prb.complete = true
			prb.acs.controller("getPbValue_" + prb.invokePhase)
			logger.WithFields(logrus.Fields{
				"signature":    len(signature),
				"documentHash": len(prb.DocumentHash),
				"echoVote":     len(prb.EchoVote),
			}).Info("[p_" + strconv.Itoa(int(prb.acs.ID)) + "] [r_" + strconv.Itoa(prb.acs.round) + "] [PB] pbEcho: create full signature")
		}
	default:
		logger.Warn("[PB] Receive unsupported msg")
	}
}

func (prb *ProvableBroadcastImpl) getSignature() tcrsa.Signature {
	if !prb.complete {
		logger.Error("[p_" + strconv.Itoa(int(prb.acs.ID)) + "] [r_" + strconv.Itoa(prb.acs.round) + "] [PB] getSignature: Provable Broadcast is not complet")
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
