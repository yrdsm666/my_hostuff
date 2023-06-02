package sDumbo

import (
	// "bytes"
	// "context"
	"encoding/hex"
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
	"fmt"
)

// var logger = logging.GetLogger()

type SpeedMvba interface {
	startSpeedMvba(proposal []byte)
	handleSpeedMvbaMsg(msg *pb.Msg)
	controller(task string)
	getLeaderVector() Vector
	getProposal() []byte
	getSignature() tcrsa.Signature
}

type SpeedMvbaImpl struct {
	acs *CommonSubsetImpl

	proposal     []byte

	spb          StrongProvableBroadcast
	cc           CommonCoin
	doneVectors  []Vector
	finalVectors []Vector
	preVoteNo    []*tcrsa.SigShare
	leader       int
	leaderVector Vector
	DFlag        int
	Ready        int
	NFlag        int
	YFlag        int
	YFinal       []*tcrsa.SigShare
	NFinal       []*tcrsa.SigShare
	Signature    tcrsa.Signature

	// Cache for storing historical leaders
	preLeader   map[int]int

	lock        sync.Mutex
	waitleader  *sync.Cond

	complete     bool
	start       bool

	lockStart   sync.Mutex
	waitStart   *sync.Cond

	lockSet     sync.Mutex
}

func NewSpeedMvba(acs *CommonSubsetImpl) *SpeedMvbaImpl {
	mvba := &SpeedMvbaImpl{
		acs:      acs,
		complete: false,
		start:    false,
	}
	mvba.waitStart = sync.NewCond(&mvba.lockStart)
	mvba.preLeader = make(map[int]int)
	return mvba
}

func (mvba *SpeedMvbaImpl) startSpeedMvba(proposal []byte) {
	logger.Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Start Speed Mvba")

	mvba.complete = false
	mvba.doneVectors = make([]Vector, 0)
	mvba.finalVectors = make([]Vector, 0)
	mvba.preVoteNo = make([]*tcrsa.SigShare, 0)

	mvba.lockSet.Lock()
	mvba.YFinal = make([]*tcrsa.SigShare, 0)
	mvba.NFinal = make([]*tcrsa.SigShare, 0)
	mvba.lockSet.Unlock()

	mvba.leader = 0
	mvba.leaderVector = Vector{}
	mvba.DFlag = 0
	mvba.NFlag = 0
	mvba.YFlag = 0
	mvba.waitleader = sync.NewCond(&mvba.lock)

	mvba.proposal = proposal

	mvba.spb = NewStrongProvableBroadcast(mvba.acs)
	mvba.cc = NewCommonCoin(mvba.acs)
	mvba.acs.taskPhase = SPB_PHASE_1

	mvba.lockStart.Lock()
	mvba.start = true
	mvba.lockStart.Unlock()
	mvba.waitStart.Broadcast()

	go mvba.spb.startStrongProvableBroadcast(proposal)
}

func (mvba *SpeedMvbaImpl) controller(task string) {
	if mvba.complete == true{
		return
	}
	switch task {
	case "getPbValue_1":
		mvba.spb.controller(task)
	case "getPbValue_2":
		mvba.spb.controller(task)
	case "getSpbValue":
		if mvba.spb.getProvableBroadcast2Status() == true {
			// mvba.spb.controller(task)
			signature := mvba.spb.getSignature2()
			marshalData, _ := json.Marshal(signature)
			spbFinalMsg := mvba.acs.SpbFinalMsg(int(mvba.acs.ID), mvba.acs.Sid, mvba.proposal, marshalData)
			// broadcast msg
			err := mvba.acs.Broadcast(spbFinalMsg)
			if err != nil {
				logger.WithField("error", err.Error()).Warn("Broadcast spbFinalMsg failed.")
			}
			// vote self
			mvba.acs.MsgEntrance <- spbFinalMsg
		}else{
			logger.Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Strong Provable Broadcast is not complet")
		}
	case "spbFinal":
		// start common coin
		mvba.acs.taskPhase = "CC"
		strSid := fmt.Sprintf("%d", mvba.acs.Sid)
		go mvba.cc.startCommonCoin(COINSHARE + strSid)
	case "spbEnd":
		mvba.spb.controller(task)
	case "getCoin":
		signature := mvba.cc.getCoin()
		// marshalData, _ := json.Marshal(signature)
		// signatureHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		smallHash := signature[len(signature)-8:]
		// fmt.Println(smallHash)
		// fmt.Println(BytesToInt(smallHash))
		mvba.lock.Lock()
		mvba.leader = int(BytesToInt(smallHash) % uint64(mvba.acs.Config.N) + 1 )
		mvba.lock.Unlock()
		mvba.waitleader.Broadcast()
		mvba.preLeader[mvba.acs.Sid] = mvba.leader
		logger.Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] get the leader: " + strconv.Itoa(mvba.leader))
		mvba.acs.taskPhase = "PREVOTE"
		//====
		// fmt.Println("---------------- [COIN_END_1] -----------------")
		// fmt.Println("Replica: ", mvba.acs.ID)
		// fmt.Println("final vectors: ",len(mvba.finalVectors))
		// for _, vector := range mvba.finalVectors {
		// 	fmt.Println("node: ", vector.Id)
		// 	fmt.Println("Sid: ", vector.Sid)
		// 	fmt.Println("proposalLen: ", len(vector.Proposal))
		// }
		// lockVectors := mvba.spb.getProvableBroadcast2().getLockVectors()
		// fmt.Println("-----------------------------------------------")
		// fmt.Println("lock vectors: ",len(lockVectors))
		// for _, vector := range lockVectors {
		// 	fmt.Println("node: ", vector.Id)
		// 	fmt.Println("Sid: ", vector.Sid)
		// 	fmt.Println("proposalLen: ", len(vector.Proposal))
		// }
		// fmt.Println("---------------- [COIN_END_2] -----------------")
		//====

		// for _, vector := range mvba.finalVectors {
		// 	if vector.Id == mvba.leader {
		// 		mvba.broadcastHalt(vector)
		// 		return
		// 	}
		// }
		mvba.Ready = 1
		lockVectors := mvba.spb.getProvableBroadcast2().getLockVectors()
		for _, vector := range lockVectors {
			if vector.Id == mvba.leader {
				// 
				// 
				if mvba.leader != 4 && (int(mvba.acs.ID)==1 || int(mvba.acs.ID)==2 ) {
					mvba.broadcastPreVote(1, vector)
					return
				}
			}
		}
		mvba.broadcastPreVote(0, Vector{})
	}
}

func (mvba *SpeedMvbaImpl) handleSpeedMvbaMsg(msg *pb.Msg) {
	if mvba.complete == true{
		return
	}

	// ensure mvba is working
	mvba.lockStart.Lock()
	if mvba.start == false {
		logger.Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + " [CC] wait mvba start")
		mvba.waitStart.Wait()
	}
	mvba.lockStart.Unlock()

	switch msg.Payload.(type) {
	case *pb.Msg_PbValue:
		// transfer messages based on the current phase
		// in fact, no matter which PB instance the pbValue message is passed to, because the processing method is the same
		if mvba.acs.taskPhase == SPB_PHASE_1 {
			mvba.spb.getProvableBroadcast1().handleProvableBroadcastMsg(msg)
		} else {
			mvba.spb.getProvableBroadcast2().handleProvableBroadcastMsg(msg)
		}
		break
	case *pb.Msg_PbEcho:
		// transfer messages based on the current phase
		if mvba.acs.taskPhase == SPB_PHASE_1 {
			mvba.spb.getProvableBroadcast1().handleProvableBroadcastMsg(msg)
		} else if mvba.acs.taskPhase == SPB_PHASE_2 {
			mvba.spb.getProvableBroadcast2().handleProvableBroadcastMsg(msg)
		}
		break
	case *pb.Msg_CoinShare:
		mvba.cc.handleCommonCoinMsg(msg)
	case *pb.Msg_SpbFinal:
		mvba.handleSpbFinal(msg)
	case *pb.Msg_Done:
		mvba.handleDone(msg)
	case *pb.Msg_Halt:
		mvba.handleHalt(msg)
	case *pb.Msg_PreVote:
		mvba.handlePreVote(msg)
	case *pb.Msg_Vote:
		mvba.handleVote(msg)
	default:
		logger.Warn("[MVBA] Receive unsupported msg")
	}
}

func (mvba *SpeedMvbaImpl) handleSpbFinal(msg *pb.Msg) {
	// Parse senderId and senderSid of message
	spbFinal := msg.GetSpbFinal()
	senderId := int(spbFinal.Id)
	senderSid := int(spbFinal.Sid)

	// Ignore messages from other sid
	if senderSid != mvba.acs.Sid {
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Warn("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get unmatched sid of spbFinal msg")
		return
	}

	logger.WithFields(logrus.Fields{
		"senderId":  senderId,
		"senderSid": senderSid,
	}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get spbFinal msg")

	// Parse proposal and signature of message
	senderProposal := spbFinal.Proposal
	signature := &tcrsa.Signature{}
	err := json.Unmarshal(spbFinal.Signature, signature)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
	}

	// Verify the signature
	flag, err := verfiySpbSig(senderId, senderSid, []byte(SPB_PHASE_2), senderProposal, *signature, mvba.acs.Config.PublicKey)
	if err != nil || flag == false {
		logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvba.acs.Sid)) + "] [MVBA] spbFinal: verfiy signature failed.")
		return
	}

	// Store the value of message as finalVector
	fVector := Vector{
		Id:        senderId,
		Sid:       senderSid,
		Proposal:  senderProposal,
		Signature: *signature,
	}
	mvba.finalVectors = append(mvba.finalVectors, fVector)

	if len(mvba.finalVectors) == 2*mvba.acs.Config.F+1 {
		if mvba.DFlag == 0 {
			mvba.DFlag = 1
			mvba.broadcastDone()
		} 
		
		// start common coin
		if mvba.acs.taskPhase != CC_PHASE && mvba.acs.taskPhase != "PREVOTE" && mvba.acs.taskPhase != "VOTE"{
			mvba.controller("spbFinal")
		}
	}
}

func (mvba *SpeedMvbaImpl) handleDone(msg *pb.Msg) {
	doneMsg := msg.GetDone()
	senderId := int(doneMsg.Id)
	senderSid := int(doneMsg.Sid)

	// Ignore messages from other sid
	if senderSid != mvba.acs.Sid {
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Warn("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [ACS] Get unmatched sid of done msg")
		return
	}

	logger.WithFields(logrus.Fields{
		"senderId":  senderId,
		"senderSid": senderSid,
	}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get Done msg")

	dVector := Vector{
		Id:        senderId,
		Sid:       senderSid,
	}
	mvba.doneVectors = append(mvba.doneVectors, dVector)

	if len(mvba.doneVectors) == mvba.acs.Config.F+1 && mvba.DFlag == 0 {
		mvba.DFlag = 1
		mvba.broadcastDone()
	}

	if len(mvba.doneVectors) == 2*mvba.acs.Config.F+1 {
		// end the spb
		if mvba.acs.taskPhase != CC_PHASE && mvba.acs.taskPhase != "PREVOTE" && mvba.acs.taskPhase != "VOTE"{
			mvba.controller("spbFinal")
		}
		mvba.controller("spbEnd")
	}
}

func (mvba *SpeedMvbaImpl) handleHalt(msg *pb.Msg) {
	haltMsg := msg.GetHalt()
	senderId := int(haltMsg.Id)
	senderSid := int(haltMsg.Sid)
	senderFinal := haltMsg.Final

	finalVector := &Vector{}
	err := json.Unmarshal(senderFinal, finalVector)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
	}

	if senderSid > mvba.acs.Sid {
		// Ignore messages from new sid
		// TOOD: save the new sid message to cache
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Warn("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [ACS] Get unmatched sid of halt msg")
		return
	} else if senderSid < mvba.acs.Sid{
		// The mvba of the previous sid has actually ended
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get halt msg from old sid")
		
		// Get the leader from the cache of previous sid
		leader := mvba.preLeader[mvba.acs.Sid-1]
		if leader != finalVector.Id {
			logger.WithFields(logrus.Fields{
				"senderId":  senderId,
				"senderSid": senderSid,
			}).Warn("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] leader from halt msg is wrong")
			return
		}
	} else {
		// message from current sid
		mvba.lock.Lock()
		if mvba.leader == 0 {
			mvba.waitleader.Wait()
		}
		mvba.lock.Unlock()

		if mvba.leader != finalVector.Id {
			return
		}

		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get halt msg")
	}

	fId := finalVector.Id
	fSid := finalVector.Sid
	fProposal := finalVector.Proposal
	fsignature := finalVector.Signature

	flag, err := verfiySpbSig(fId, fSid, []byte(SPB_PHASE_2), fProposal, fsignature, mvba.acs.Config.PublicKey)
	if err != nil || flag == false {
		logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvba.acs.Sid)) + "] [MVBA] halt: verfiy signature failed.")
		return
	}

	if mvba.complete == false {
		mvba.complete = true
		mvba.leaderVector = Vector{
			Id:    fId,
			Sid:   fSid,
			Proposal: fProposal,
			Signature: fsignature,
		}
		logger.WithFields(logrus.Fields{
			"leaderId":  mvba.leaderVector.Id,
			"leaderSid": mvba.leaderVector.Sid,
			"leaderProposalLen":  len(mvba.leaderVector.Proposal),
			"leaderSignatureLen": len(mvba.leaderVector.Signature),
		}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] success end the mvba!")
		mvba.acs.taskSignal <- "end"
	}
}

func (mvba *SpeedMvbaImpl) handlePreVote(msg *pb.Msg) {
	preVoteMsg := msg.GetPreVote()
	flag := preVoteMsg.Flag
	senderId := int(preVoteMsg.Id)
	senderSid := int(preVoteMsg.Sid)

	// Test abnormal situations
	//
	//
	if senderId == 4 && int(mvba.acs.ID) != 4 {
		return
	}

	// Ignore messages from other sid
	if senderSid != mvba.acs.Sid {
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Warn("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [ACS] Get unmatched sid of preVote msg")
		return
	}

	logger.WithFields(logrus.Fields{
		"senderId":  senderId,
		"senderSid": senderSid,
		"flag":      flag,
	}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get PreVote msg")

	leaderProposal := preVoteMsg.Proposal
	leader := int(preVoteMsg.Leader)

	mvba.lock.Lock()
	if mvba.leader == 0 {
		mvba.waitleader.Wait()
	}
	mvba.lock.Unlock()

	if mvba.leader != leader {
		logger.Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] prevote: leader in msg is mismatch")
		return
	}

	if mvba.YFlag == 1 || mvba.NFlag == 1{
		// YFlag == 1 or mvba.NFlag == 1 means that the prevote phase has ended,
		// so the following operations are unnecessary
		return
	}

	if flag == 1 && mvba.NFlag == 0 && mvba.YFlag == 0{
		signature := &tcrsa.Signature{}
		err := json.Unmarshal(preVoteMsg.Signature, signature)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
		}

		flag, err := verfiySpbSig(leader, senderSid, []byte(SPB_PHASE_1), leaderProposal, *signature, mvba.acs.Config.PublicKey)
		if err != nil || flag == false {
			logger.WithFields(logrus.Fields{
				"error": err.Error(),
				"leader": leader,
				"leaderSid": senderSid,
				"leaderProposal": leaderProposal,
			}).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] preVote: verfiy signature failed.")
			return
		}

		if mvba.NFlag == 0 && mvba.YFlag == 0{
			mvba.YFlag = 1
			mvba.broadcastVote(1, leaderProposal, *signature)
		}
	} else {
		partSig := &tcrsa.SigShare{}
		err := json.Unmarshal(preVoteMsg.PartialSig, partSig)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal partSig failed.")
		}

		nullBytes := []byte("null" + "NO" + "Sid_" + strconv.Itoa(mvba.acs.Sid))
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

		if len(mvba.preVoteNo) == 2*mvba.acs.Config.F+1 && mvba.YFlag == 0 {
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
}

func (mvba *SpeedMvbaImpl) handleVote(msg *pb.Msg) {
	voteMsg := msg.GetVote()
	flag := voteMsg.Flag
	senderId := int(voteMsg.Id)
	senderSid := int(voteMsg.Sid)

	// Ignore messages from other sid
	if senderSid != mvba.acs.Sid {
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
			"flag":      flag,
		}).Warn("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get unmatched sid of vote msg")
		return
	}

	logger.WithFields(logrus.Fields{
		"senderId":  senderId,
		"senderSid": senderSid,
		"flag":      flag,
	}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get Vote msg")

	leaderProposal := voteMsg.Proposal
	leader := int(voteMsg.Leader)

	mvba.lock.Lock()
	if mvba.leader == 0 {
		mvba.waitleader.Wait()
	}
	mvba.lock.Unlock()

	if mvba.leader != leader {
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
			"nowleader":  mvba.leader,
			"msgleader":  leader,
		}).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] vote: leader in msg is mismatch")
		return
	}

	signature := &tcrsa.Signature{}
	err := json.Unmarshal(voteMsg.Signature, signature)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
	}

	partSig := &tcrsa.SigShare{}
	err = json.Unmarshal(voteMsg.PartialSig, partSig)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Unmarshal SigShare failed.")
	}

	var documentHash []byte
	if flag == 1 {
		res, err := verfiySpbSig(mvba.leader, senderSid, []byte(SPB_PHASE_1), leaderProposal, *signature, mvba.acs.Config.PublicKey)
		if err != nil || res == false {
			logger.WithFields(logrus.Fields{
				"error": err.Error(),
				"senderId":  senderId,
				"senderSid": senderSid,
				"nowleader":  mvba.leader,
				"msgleader":  leader,
				"leaderProposal":  len(leaderProposal),
				"signature":  len(*signature),
			}).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] vote: verfiy signature failed.")
			logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvba.acs.Sid)) + "] [MVBA] vote: verfiy signature failed.")
			return
		}

		newProposal :=  bytesAdd(leaderProposal, []byte(SPB_PHASE_2))
		proposalHash, _ := go_hotstuff.CreateDocumentHash(newProposal, mvba.acs.Config.PublicKey)
		marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, proposalHash)
		documentHash, _ = go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		err = go_hotstuff.VerifyPartSig(partSig, documentHash, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error":        err.Error(),
				"documentHash": hex.EncodeToString(documentHash),
			}).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] vote: partSig not verified!")
			return
		}

		mvba.leaderVector = Vector{
			Id:    mvba.leader,
			Sid:   mvba.acs.Sid,
			Proposal: leaderProposal,
			Signature: *signature,
		}
		
		mvba.lockSet.Lock()
		mvba.YFinal = append(mvba.YFinal, partSig)
		mvba.lockSet.Unlock()
	} else {
		nullBytes := []byte("null" + "NO" + "Sid_" + strconv.Itoa(mvba.acs.Sid)) 
		marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, nullBytes)
		res, err := go_hotstuff.TVerify(mvba.acs.Config.PublicKey, *signature, marshalData)
		if err != nil || res == false {
			logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvba.acs.Sid)) + "] [MVBA] vote: verfiy signature failed.")
			return
		}

		unlockBytes := []byte("UnLocked")
		marshalData = getMsgdata(mvba.leader, mvba.acs.Sid, unlockBytes)
		documentHash, _ = go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		err = go_hotstuff.VerifyPartSig(partSig, documentHash, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error":        err.Error(),
				"documentHash": hex.EncodeToString(documentHash),
			}).Warn("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] vote: partSig not verified!")
			return
		}

		mvba.lockSet.Lock()
		mvba.NFinal = append(mvba.NFinal, partSig)
		mvba.lockSet.Unlock()
	}

	if len(mvba.YFinal) + len(mvba.NFinal) == 2*mvba.acs.Config.F+1{
		if len(mvba.YFinal) == 2*mvba.acs.Config.F+1{
			leaderSignature, err := go_hotstuff.CreateFullSignature(documentHash, mvba.YFinal, mvba.acs.Config.PublicKey)
			if err != nil {
				logger.WithFields(logrus.Fields{
					"error":        err.Error(),
					"documentHash": hex.EncodeToString(documentHash),
				}).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] vote: create signature failed!")
			}

			logger.WithFields(logrus.Fields{
				"leader":       leader,
				"leaderSid":    mvba.acs.Sid,
				"signature":    len(leaderSignature),
				// "documentHash": hex.EncodeToString(documentHash),
			}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] vote: create full signature of halt")

			vector := Vector{
				Id:        leader,
				Sid:       mvba.acs.Sid,
				Proposal:  leaderProposal,
				Signature: leaderSignature,
			}

			mvba.broadcastHalt(vector)
		} else {
			if len(mvba.NFinal) == 2*mvba.acs.Config.F+1{
				unLockSignature, err := go_hotstuff.CreateFullSignature(documentHash, mvba.NFinal, mvba.acs.Config.PublicKey)
				if err != nil {
					logger.WithFields(logrus.Fields{
						"error":        err.Error(),
						"documentHash": hex.EncodeToString(documentHash),
					}).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] vote: create signature failed!")
				}
				// restart MVBA with own proposal 
				mvba.Signature = unLockSignature
				mvba.acs.taskSignal <- "restart"
			}else{
				// restart MVBA with leader proposal
				mvba.acs.taskSignal <- "restartWithLeaderProposal"
			}
		}
	}
}



func (mvba *SpeedMvbaImpl) broadcastDone() {
	doneMsg := mvba.acs.DoneMsg(int(mvba.acs.ID), mvba.acs.Sid)
	// broadcast msg
	err := mvba.acs.Broadcast(doneMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Warn("Broadcast doneMsg failed.")
	}

	// send to self
	mvba.acs.MsgEntrance <- doneMsg
}

func (mvba *SpeedMvbaImpl) broadcastHalt(vector Vector) {
	marshalData, _ := json.Marshal(vector)
	id := int(mvba.acs.ID)
	haltMsg := mvba.acs.HaltMsg(id, mvba.acs.Sid, marshalData)
	// broadcast msg
	err := mvba.acs.Broadcast(haltMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Warn("Broadcast haltMsg failed.")
	}

	// send to self
	mvba.acs.MsgEntrance <- haltMsg
}

func (mvba *SpeedMvbaImpl) broadcastPreVote(flag int, vector Vector) {
	var preVoteMsg *pb.Msg
	if flag == 0 {
		nullBytes := []byte("null" + "NO" + "Sid_" + strconv.Itoa(mvba.acs.Sid))
		marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, nullBytes)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, mvba.acs.Config.PrivateKey, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("create the partial signature failed.")
		}
		partSigBytes, _ := json.Marshal(partSig)
		preVoteMsg = mvba.acs.PreVoteMsg(int(mvba.acs.ID), mvba.acs.Sid, mvba.leader, flag, nil, nil, partSigBytes)
	} else {
		signatureBytes, _ := json.Marshal(vector.Signature)
		preVoteMsg = mvba.acs.PreVoteMsg(int(mvba.acs.ID), mvba.acs.Sid, mvba.leader, flag, vector.Proposal, signatureBytes, nil)
	}

	// broadcast msg
	err := mvba.acs.Broadcast(preVoteMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Warn("Broadcast preVote failed.")
	}

	// send to self
	mvba.acs.MsgEntrance <- preVoteMsg
}

func (mvba *SpeedMvbaImpl) broadcastVote(flag int, proposal []byte, signature tcrsa.Signature) {
	var voteMsg *pb.Msg
	signatureBytes, _ := json.Marshal(signature)
	if flag == 1 {
		newProposal :=  bytesAdd(proposal, []byte(SPB_PHASE_2))
		proposalHash, _ := go_hotstuff.CreateDocumentHash(newProposal, mvba.acs.Config.PublicKey)
		marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, proposalHash)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, mvba.acs.Config.PrivateKey, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("create the partial signature failed.")
		}
		partSigBytes, _ := json.Marshal(partSig)
		voteMsg = mvba.acs.VoteMsg(int(mvba.acs.ID), mvba.acs.Sid, mvba.leader, flag, proposal, signatureBytes, partSigBytes)
	}else{
		unlockBytes := []byte("UnLocked")
		marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, unlockBytes)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, mvba.acs.Config.PrivateKey, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("create the partial signature failed.")
		}
		partSigBytes, _ := json.Marshal(partSig)
		voteMsg = mvba.acs.VoteMsg(int(mvba.acs.ID), mvba.acs.Sid, mvba.leader, flag, nil, signatureBytes, partSigBytes)
	}

	// broadcast msg
	err := mvba.acs.Broadcast(voteMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Warn("Broadcast voteMsg failed.")
	}

	// send to self
	mvba.acs.MsgEntrance <- voteMsg
}

func (mvba *SpeedMvbaImpl) getLeaderVector() Vector {
	return mvba.leaderVector
}

func (mvba *SpeedMvbaImpl) getProposal() []byte {
	return mvba.proposal
}

func (mvba *SpeedMvbaImpl) getSignature() tcrsa.Signature {
	return mvba.Signature
}

func BytesToInt(bys []byte) uint64 {
	if len(bys) < 8{
		logger.Error("BytesToInt: bytes is too small!")
		return 0
	}
	data := binary.BigEndian.Uint64(bys)
	return data
}

func verfiySpbSig(id int, sid int, jBytes []byte, proposal []byte, signature tcrsa.Signature, publicKey *tcrsa.KeyMeta) (bool, error) {
	// deep copy
	newProposal := bytesAdd(proposal, jBytes)
	proposalHash, _ := go_hotstuff.CreateDocumentHash(newProposal, publicKey)
	marshalData := getMsgdata(id, sid, proposalHash)
	flag, err := go_hotstuff.TVerify(publicKey, signature, marshalData)
	if err != nil || flag == false {
		logger.WithField("error", err.Error()).Error("verfiySpbSig failed.")
		return false, err
	}
	return true, nil

}

// return proposal + jBytes
func bytesAdd(proposal []byte, jBytes []byte) []byte {
	// deep copy
	newProposal := make([]byte, 0)
	newProposal = append(newProposal, proposal...)
	newProposal = append(newProposal, jBytes...)
	return newProposal
}

// return proposal - jBytes
func bytesSub(proposal []byte, jBytes []byte) []byte {
	newProposal := make([]byte, 0)
	newProposal = append(newProposal, proposal...)
	newProposal = newProposal[:len(newProposal) - len(jBytes)]
	return newProposal
}
