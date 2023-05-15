package sDumbo

import (
	"bytes"
	// "context"
	// "encoding/hex"
	"encoding/binary"
	"encoding/json"

	// "github.com/golang/protobuf/proto"
	"github.com/niclabs/tcrsa"
	"github.com/sirupsen/logrus"

	// "github.com/syndtr/goleveldb/leveldb"
	go_hotstuff "github.com/wjbbig/go-hotstuff"
	// "github.com/wjbbig/go-hotstuff/config"
	// "github.com/wjbbig/go-hotstuff/consensus"
	// "github.com/wjbbig/go-hotstuff/logging"
	pb "github.com/wjbbig/go-hotstuff/proto"
	// "os"
	"strconv"
	"sync"
)

// var logger = logging.GetLogger()

type SpeedMvba interface {
	startSpeedMvba(vectors []Vector)
	handleSpeedMvbaMsg(msg *pb.Msg)
	controller(task string)
}

type SpeedMvbaImpl struct {
	acs *CommonSubsetImpl

	proposal []byte

	spb          StrongProvableBroadcast
	cc           CommonCoin
	doneVectors  []Vector
	finalVectors []Vector
	preVoteNo    []*tcrsa.SigShare
	leader       int
	DFlag        int
	Ready        int
	NFlag        int
	YFlag        int
	YFinal       []*tcrsa.SigShare
	NFinal       []*tcrsa.SigShare
	DocumentHash []byte
	Signature    tcrsa.Signature
	complete     bool

	lock       sync.Mutex
	waitleader *sync.Cond

	// SPB           *NewStrongProvableBroadcast
	// cc            *NewCommonCoin
}

func NewSpeedMvba(acs *CommonSubsetImpl) *SpeedMvbaImpl {
	mvba := &SpeedMvbaImpl{
		acs:      acs,
		complete: false,
	}
	return mvba
}

func (mvba *SpeedMvbaImpl) startSpeedMvba(vectors []Vector) {
	logger.Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Start Speed Mvba")

	mvba.doneVectors = make([]Vector, 0)
	mvba.finalVectors = make([]Vector, 0)
	mvba.preVoteNo = make([]*tcrsa.SigShare, 0)
	mvba.YFinal = make([]*tcrsa.SigShare, 0)
	mvba.NFinal = make([]*tcrsa.SigShare, 0)
	proposal, _ := json.Marshal(vectors)
	mvba.acs.taskPhase = "MVBA"
	mvba.proposal = proposal
	mvba.leader = 0
	mvba.DFlag = 0
	mvba.NFlag = 0
	mvba.YFlag = 0
	mvba.waitleader = sync.NewCond(&mvba.lock)
	mvba.spb = NewStrongProvableBroadcast(mvba.acs)
	mvba.cc = NewCommonCoin(mvba.acs)

	go mvba.spb.startStrongProvableBroadcast(proposal)
}

func (mvba *SpeedMvbaImpl) controller(task string) {
	switch task {
	case "getPbValue":
		mvba.spb.controller(task)
	case "getSpbValue":
		if mvba.spb.getProvableBroadcast2Status() == true {
			// mvba.spb.controller(task)
			signature1 := mvba.spb.getSignature1()
			signature2 := mvba.spb.getSignature2()
			spbFinalMsg := acs.Msg(pb.MsgType_SPBFINAL, int(mvba.acs.ID), mvba.acs.Sid, mvba.proposal, signature1, signature2)
			// broadcast msg
			err := mvba.acs.Broadcast(spbFinalMsg)
			if err != nil {
				logger.WithField("error", err.Error()).Error("Broadcast failed.")
			}
		}
	case "doneFinal":
		go mvba.cc.startCommonCoin("coin" + string(mvba.acs.ID) + string(mvba.acs.Sid))
	case "getCoin":
		signature := mvba.cc.getCoin()
		marshalData, _ := json.Marshal(signature)
		signatureHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		mvba.leader = BytesToInt(signatureHash)%mvba.acs.Config.N + 1
		mvba.waitleader.Broadcast()
		for _, vector := range mvba.finalVectors {
			if vector.id == mvba.leader {
				mvba.broadcastHalt(vector)
				mvba.complete = true
				return
			}
		}
		mvba.Ready = 1
		for _, vector := range mvba.doneVectors {
			if vector.id == mvba.leader {
				mvba.broadcastPreVote(1, vector)
				return
			}
		}
		mvba.broadcastPreVote(0, Vector{})
	}
}

func (mvba *SpeedMvbaImpl) handleSpeedMvbaMsg(msg *pb.Msg) {
	switch msg.Payload.(type) {
	case *pb.Msg_Done:
		doneMsg := msg.GetDone()
		senderId := int(doneMsg.Id)
		senderSid := int(doneMsg.Sid)
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get Done msg")
		senderProposal := doneMsg.Proposal
		signature := &tcrsa.Signature{}
		err := json.Unmarshal(doneMsg.Signature, signature)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
		}

		flag, err := verfiySpbSig(senderId, senderSid, "1", senderProposal, *signature, mvba.acs.Config.PublicKey)
		if err != nil || flag == false {
			logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvba.acs.Sid)) + "] [MVBA] verfiy done signature failed.")
			return
		}

		dVector := Vector{
			id:        senderId,
			sid:       senderSid,
			proposal:  senderProposal,
			Signature: *signature,
		}
		mvba.doneVectors = append(mvba.doneVectors, dVector)
		if len(mvba.doneVectors) == mvba.acs.Config.F+1 && mvba.DFlag == 0 {
			mvba.DFlag = 1
			mvba.broadcastDone()
		}
		if len(mvba.doneVectors) == 2*mvba.acs.Config.F+1 {
			// start Common coin
			mvba.controller("doneFinal")
		}
		break
	case *pb.Msg_SpbFinal:
		spbFinal := msg.GetSpbFinal()
		senderId := int(spbFinal.Id)
		senderSid := int(spbFinal.Sid)
		senderProposal := spbFinal.Proposal
		if senderSid != mvba.acs.Sid {
			logger.WithFields(logrus.Fields{
				"senderId":  senderId,
				"senderSid": senderSid,
			}).Warn("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get mismatch spbFinal msg")
			break
		}
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get spbFinal msg")
		signature := &tcrsa.Signature{}
		err := json.Unmarshal(spbFinal.Signature, signature)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
		}

		flag, err := verfiySpbSig(senderId, senderSid, "2", senderProposal, *signature, mvba.acs.Config.PublicKey)
		if err != nil || flag == false {
			logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvba.acs.Sid)) + "] [MVBA] verfiy signature failed.")
			return
		}

		fVector := Vector{
			id:        senderId,
			sid:       senderSid,
			proposal:  senderProposal,
			Signature: *signature,
		}
		mvba.finalVectors = append(mvba.finalVectors, fVector)

		if len(mvba.finalVectors) == 2*mvba.acs.Config.F+1 && mvba.DFlag == 0 {
			mvba.DFlag = 1
			mvba.broadcastDone()
		}
		break
	case *pb.Msg_Halt:
		haltMsg := msg.GetHalt()
		// senderSid := int(halt.Sid)
		senderFinal := haltMsg.Final
		finalVector := &Vector{}
		err := json.Unmarshal(senderFinal, finalVector)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
		}
		if mvba.leader == 0 {
			mvba.waitleader.Wait()
		}
		if mvba.leader != finalVector.id {
			return
		}
		fId := finalVector.id
		fSid := finalVector.sid
		fProposal := finalVector.proposal
		fsignature := finalVector.Signature

		flag, err := verfiySpbSig(fId, fSid, "2", fProposal, fsignature, mvba.acs.Config.PublicKey)
		if err != nil || flag == false {
			logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvbaacs.Sid)) + "] [MVBA] verfiy signature failed.")
			return
		}
		if mvba.complete == false {
			mvba.complete = true
		}
	case *pb.Msg_PreVote:
		preVoteMsg := msg.GetPreVote()
		flag := preVoteMsg.flag
		senderId := int(preVoteMsg.Id)
		senderSid := int(preVoteMsg.Sid)
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get PreVote msg")
		senderProposal := preVoteMsg.Proposal
		leader := preVoteMsg.Leader
		if mvba.leader != leader {
			return
		}
		if flag == 1 {
			signature := &tcrsa.Signature{}
			err := json.Unmarshal(preVoteMsg.Signature, signature)
			if err != nil {
				logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
			}
	
			flag, err := verfiySpbSig(mvba.leader, senderSid, "1", senderProposal, *signature, mvba.acs.Config.PublicKey)
			if err != nil || flag == false {
				logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvba.acs.Sid)) + "] [MVBA] verfiy signature failed.")
				return
			}
			if mvba.NFlag == 0{
				mvba.YFlag = 1
				mvba.broadcastVote(1, senderProposal, signature)
			}
		} else{
			partSig := &tcrsa.SigShare{}
			err := json.Unmarshal(preVoteMsg.SignShare, partSig)
			if err != nil {
				logger.WithField("error", err.Error()).Error("Unmarshal partSig failed.")
			}
			nullBytes := []byte("null" + "NO")
			marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, nullBytes)
			documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
			err = go_hotstuff.VerifyPartSig(partSig, documentHash, mvba.acs.Config.PublicKey)
			if err != nil {
				logger.WithFields(logrus.Fields{
					"error":        err.Error(),
					"documentHash": hex.EncodeToString(documentHash),
				}).Warn("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] preVote: sigShare not verified!")
				return
			}
			mvba.preVoteNo = append(mvba.preVoteNo, partSig)
			if len(mvba.preVoteNo) == 2*mvba.acs.Config.F+1 && mvba.YFlag == 0{
				signature, err := go_hotstuff.CreateFullSignature(documentHash, mvba.preVoteNo, mvba.acs.Config.PublicKey)
				if err != nil {
					logger.WithFields(logrus.Fields{
						"error":        err.Error(),
						"documentHash": hex.EncodeToString(documentHash),
					}).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] preVote: create signature failed!")
				}
				mvba.NFlag = 1
				mvba.broadcastVote(0, nil, signature)
			}
		}
	case *pb.Msg_Vote:
		voteMsg := msg.GetVote()
		flag := voteMsg.flag
		senderId := int(voteMsg.Id)
		senderSid := int(voteMsg.Sid)
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get Vote msg")
		senderProposal := voteMsgMsg.Proposal
		leader := voteMsg.Leader
		if mvba.leader != leader {
			return
		}
		if flag == 1 {
			signature := &tcrsa.Signature{}
			err := json.Unmarshal(voteMsg.Signature, signature)
			if err != nil {
				logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
			}
	
			flag, err := verfiySpbSig(mvba.leader, senderSid, "1", senderProposal, *signature, mvba.acs.Config.PublicKey)
			if err != nil || flag == false {
				logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvba.acs.Sid)) + "] [MVBA] verfiy signature failed.")
				return
			}

			partSig := &tcrsa.SigShare{}
			err := json.Unmarshal(voteMsg.SigShare, sigShare)
			if err != nil {
				logger.WithField("error", err.Error()).Error("Unmarshal SigShare failed.")
			}
			j := []byte("2")
			newProposal := append(proposal[:], j[0])
			marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, nullBytes)
			documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
			err = go_hotstuff.VerifyPartSig(partSig, documentHash, mvba.acs.Config.PublicKey)
			if err != nil {
				logger.WithFields(logrus.Fields{
					"error":        err.Error(),
					"documentHash": hex.EncodeToString(documentHash),
				}).Warn("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] preVote: sigShare not verified!")
				return
			}
			mvba.YFinal = append(mvba.YFinal, partSig)
			if len(mvba.YFinal) + len(mvba.NFinal) == 2*mvba.acs.Config.F+1{
				if len(mvba.YFinal) == 2*mvba.acs.Config.F+1{
					signature, err := go_hotstuff.CreateFullSignature(documentHash, mvba.YFinal, mvba.acs.Config.PublicKey)
					if err != nil {
						logger.WithFields(logrus.Fields{
							"error":        err.Error(),
							"documentHash": hex.EncodeToString(documentHash),
						}).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] preVote: create signature failed!")
					}
					Vector := Vector{
						id:        leader,
						sid:       nil,
						proposal:  senderProposal,
						Signature: *signature,
					}
					mvba.broadcastHalt(vector)
					
				}
				
			}
		}

	default:
		logger.Warn("[MVBA] Receive unsupported msg")
	}
}

func (mvba *SpeedMvbaImpl) broadcastDone() {
	signature := mvba.spb.getSignature1()
	signatureBytes, _ := json.Marshal(signature)
	id := int(mvba.acs.ID)
	doneMsg := mvba.acs.Msg(pb.MsgType_Done, id, mvba.acs.Sid, mvba.proposal, signatureBytes)
	// broadcast msg
	err := mvba.acs.Broadcast(doneMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Broadcast failed.")
	}

	// send to self
}

func (mvba *SpeedMvbaImpl) broadcastHalt(vector Vector) {
	marshalData, _ := json.Marshal(vector)
	id := int(mvba.acs.ID)
	haltMsg := mvba.acs.Msg(pb.MsgType_Halt, id, mvba.acs.Sid, marshalData)
	// broadcast msg
	err := mvba.acs.Broadcast(haltMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Broadcast failed.")
	}

	// send to self
}

func (mvba *SpeedMvbaImpl) broadcastPreVote(flag int, vector Vector) {
	var preVoteMsg *pb.Msg
	if flag == 0 {
		nullBytes := []byte("null" + "NO")
		marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, nullBytes)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, mvba.acs.Config.PrivateKey, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("create the partial signature failed.")
		}

		preVoteMsg = mvba.acs.Msg(pb.MsgType_PreVote, int(mvba.acs.ID), mvba.acs.Sid, mvba.leader, flag, nil, partSig, nil)
	}else{
		preVoteMsg = mvba.acs.Msg(pb.MsgType_PreVote, int(mvba.acs.ID), mvba.acs.Sid, mvba.leader, flag, vector.proposal, nil, vector.signature)
	}

	// broadcast msg
	err := mvba.acs.Broadcast(preVoteMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Broadcast failed.")
	}

	// send to self
}

func (mvba *SpeedMvbaImpl) broadcastVote(flag int, proposal []byte, signature tcrsa.Signature) {
	var voteMsg *pb.Msg
	if flag == 0 {
		unlockBytes := []byte("UnLocked")
		marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, unlockBytes)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, mvba.acs.Config.PrivateKey, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("create the partial signature failed.")
		}

		voteMsg = mvba.acs.Msg(pb.MsgType_Vote, int(mvba.acs.ID), mvba.acs.Sid, mvba.leader, flag, nil, signature, partSig)
	}else{
		j := []byte("2")
		newProposal := append(proposal[:], j[0])
		marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, newProposal)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, mvba.acs.Config.PrivateKey, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("create the partial signature failed.")
		}
		voteMsg = mvba.acs.Msg(pb.MsgType_Vote, int(mvba.acs.ID), mvba.acs.Sid, mvba.leader, flag, proposal, signature, partSig)
	}

	// broadcast msg
	err := mvba.acs.Broadcast(voteMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Broadcast failed.")
	}

	// send to self
}

func BytesToInt(bys []byte) int {
	bytebuff := bytes.NewBuffer(bys)
	var data int64
	binary.Read(bytebuff, binary.BigEndian, &data)
	return int(data)
}

func verfiySpbSig(id int, sid int, j string, proposal []byte, signature tcrsa.Signature, publicKey *tcrsa.KeyMeta) (bool, error) {
	jBytes := []byte(j)
	newProposal := append(proposal[:], jBytes[0])
	marshalData := getMsgdata(id, sid, newProposal)

	flag, err := go_hotstuff.TVerify(publicKey, signature, marshalData)
	if err != nil || flag == false {
		logger.WithField("error", err.Error()).Error("verfiySpbSig failed.")
		return false, err
	}
	return true, nil

}
