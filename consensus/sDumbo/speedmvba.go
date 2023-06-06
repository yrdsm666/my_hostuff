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
	initStatus()
}

type SpeedMvbaImpl struct {
	acs *CommonSubsetImpl

	sid          int
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

// var (
// 	strId string
// 	strRound string
// 	strSid string
// )


func (mvba *SpeedMvbaImpl) startSpeedMvba(proposal []byte) {
	mvba.sid += 1

	logger.Info("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] Start Speed Mvba")
	
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

	mvba.lockStart.Lock()
	mvba.start = true
	mvba.lockStart.Unlock()
	mvba.waitStart.Broadcast()

	mvba.spb.startStrongProvableBroadcast(proposal, mvba.sid)

	msgList := mvba.acs.getMsgFromCache(mvba.acs.round, mvba.sid)
	if msgList != nil {
		logger.Warn("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] Handle msg from msg cache")
		for _, msg := range msgList {
			mvba.handleSpeedMvbaMsg(msg)
		}
	}

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
			spbFinalMsg := mvba.acs.SpbFinalMsg(int(mvba.acs.ID), mvba.acs.round, mvba.sid, mvba.proposal, marshalData)
			// broadcast msg
			err := mvba.acs.Broadcast(spbFinalMsg)
			if err != nil {
				logger.WithField("error", err.Error()).Warn("Broadcast spbFinalMsg failed.")
			}
			// vote self
			mvba.acs.MsgEntrance <- spbFinalMsg
		}else{
			logger.Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] Strong Provable Broadcast is not complet")
		}
	case "spbFinal":
		// start common coin
		mvba.acs.taskPhase = CC_PHASE
		//strRound := fmt.Sprintf("%d", mvba.acs.round)
		cstrSid := fmt.Sprintf("%d", mvba.sid)
		go mvba.cc.startCommonCoin(COINSHARE + cstrSid, mvba.sid)
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
		mvba.preLeader[mvba.sid] = mvba.leader
		logger.Info("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] get the leader: " + strconv.Itoa(mvba.leader))
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
		logger.Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] wait mvba start")
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
	senderRound := int(spbFinal.Round)
	senderSid := int(spbFinal.Sid)

	// Check msg round and sid
	if !mvba.checkMsgMark(senderId, senderRound, senderSid, msg) {
		return
	}

	logger.WithFields(logrus.Fields{
		"senderId":  senderId,
		"senderRound":  senderRound,
		"senderSid": senderSid,
	}).Info("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] Get spbFinal msg")

	// Parse proposal and signature of message
	senderProposal := spbFinal.Proposal
	signature := &tcrsa.Signature{}
	err := json.Unmarshal(spbFinal.Signature, signature)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
	}

	// Verify the signature
	flag, err := verfiySpbSig(senderId, senderRound, senderSid, []byte(SPB_PHASE_2), senderProposal, *signature, mvba.acs.Config.PublicKey)
	if err != nil || flag == false {
		logger.WithField("error", err.Error()).Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] spbFinal: verfiy signature failed.")
		return
	}

	// Store the value of message as finalVector
	fVector := Vector{
		Id:        senderId,
		Round:     senderRound,
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
	senderRound := int(doneMsg.Round)
	senderSid := int(doneMsg.Sid)

	// Check msg round and sid
	if !mvba.checkMsgMark(senderId, senderRound, senderSid, msg) {
		return
	}

	logger.WithFields(logrus.Fields{
		"senderId":  senderId,
		"senderSid": senderSid,
	}).Info("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] Get Done msg")

	dVector := Vector{
		Id:        senderId,
		Round:     senderRound,
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
	senderRound := int(haltMsg.Round)
	senderSid := int(haltMsg.Sid)
	senderFinal := haltMsg.Final

	finalVector := &Vector{}
	err := json.Unmarshal(senderFinal, finalVector)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
	}
	fId := finalVector.Id
	fSid := finalVector.Sid
	fRound := finalVector.Round
	fProposal := finalVector.Proposal
	fsignature := finalVector.Signature

	// if senderRound > mvba.acs.round || (senderRound == mvba.acs.round && senderSid > mvba.sid) {
	if fRound > mvba.acs.round {
		// Save the new round message to cache
		mvba.acs.insertMsg(fRound, senderSid, msg)
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderRound":  senderRound,
			"senderSid": senderSid,
			"fRound": fRound,
			"fSid": fSid,
		}).Warn("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] Get HALT msg of future round, and save it in msg cache")
		return
	} else if fRound < mvba.acs.round {
		// Ignore old round's msg
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderRound":  senderRound,
			"senderSid": senderSid,
			"fRound": fRound,
			"fSid": fSid,
		}).Warn("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] Get HALT msg from old round, and ignore it")
		return
	} else {
		if fSid < mvba.sid{
			// Get the leader from the cache of previous sid
			leader, ok := mvba.preLeader[fSid]
			if !ok {
				return
			} else {
				if leader != finalVector.Id {
					logger.WithFields(logrus.Fields{
						"senderId":  senderId,
						"senderRound":  senderRound,
						"senderSid": senderSid,
					}).Warn("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] leader from halt msg is wrong")
					return
				}
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
		}
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderRound":  senderRound,
			"senderSid": senderSid,
			"fRound": fRound,
			"fSid": fSid,
		}).Info("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] Get halt msg")
	}

	flag, err := verfiySpbSig(fId, fRound, fSid, []byte(SPB_PHASE_2), fProposal, fsignature, mvba.acs.Config.PublicKey)
	if err != nil || flag == false {
		logger.WithField("error", err.Error()).Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] halt: verfiy signature failed.")
		return
	}

	// senderRound == mvba.acs.round is necessary
	if mvba.complete == false && fRound == mvba.acs.round {
		mvba.complete = true
		mvba.leaderVector = Vector{
			Id:    fId,
			Sid:   fSid,
			Round:  fRound,
			Proposal: fProposal,
			Signature: fsignature,
		}
		logger.WithFields(logrus.Fields{
			"leaderId":  mvba.leaderVector.Id,
			"leaderRound":  mvba.leaderVector.Round,
			"leaderSid": mvba.leaderVector.Sid,
			"leaderProposalLen":  len(mvba.leaderVector.Proposal),
			"leaderSignatureLen": len(mvba.leaderVector.Signature),
		}).Info("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] success end the mvba!")
		mvba.acs.taskSignal <- "end"
	}
}

func (mvba *SpeedMvbaImpl) handlePreVote(msg *pb.Msg) {
	preVoteMsg := msg.GetPreVote()
	flag := preVoteMsg.Flag
	senderId := int(preVoteMsg.Id)
	senderRound := int(preVoteMsg.Round)
	senderSid := int(preVoteMsg.Sid)

	// Test abnormal situations
	//
	//
	if senderId == 4 && int(mvba.acs.ID) != 4 {
		return
	}

	// Check msg round and sid
	if !mvba.checkMsgMark(senderId, senderRound, senderSid, msg) {
		return
	}

	logger.WithFields(logrus.Fields{
		"senderId":  senderId,
		"senderRound":  senderRound,
		"senderSid": senderSid,
		"flag":      flag,
	}).Info("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] Get PreVote msg")

	leaderProposal := preVoteMsg.Proposal
	leader := int(preVoteMsg.Leader)

	mvba.lock.Lock()
	if mvba.leader == 0 {
		mvba.waitleader.Wait()
	}
	mvba.lock.Unlock()

	if mvba.leader != leader {
		logger.Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] prevote: leader in msg is mismatch")
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

		flag, err := verfiySpbSig(leader, senderRound, senderSid, []byte(SPB_PHASE_1), leaderProposal, *signature, mvba.acs.Config.PublicKey)
		if err != nil || flag == false {
			logger.WithFields(logrus.Fields{
				"error": err.Error(),
				"leader": leader,
				"leaderRound":  senderRound,
				"leaderSid": senderSid,
				"leaderProposal": leaderProposal,
			}).Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] preVote: verfiy signature failed.")
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

		nullBytes := []byte("null" + "NO" + "Sid_" + strconv.Itoa(mvba.sid))
		marshalData := getMsgdata(mvba.leader, mvba.acs.round, mvba.sid, nullBytes)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		err = go_hotstuff.VerifyPartSig(partSig, documentHash, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error":        err.Error(),
				"documentHash": hex.EncodeToString(documentHash),
			}).Warn("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] preVote: sigShare not verified!")
			return
		}

		mvba.preVoteNo = append(mvba.preVoteNo, partSig)

		if len(mvba.preVoteNo) == 2*mvba.acs.Config.F+1 && mvba.YFlag == 0 {
			signature, err := go_hotstuff.CreateFullSignature(documentHash, mvba.preVoteNo, mvba.acs.Config.PublicKey)
			if err != nil {
				logger.WithFields(logrus.Fields{
					"error":        err.Error(),
					"documentHash": hex.EncodeToString(documentHash),
				}).Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] preVote: create signature failed!")
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
	senderRound := int(voteMsg.Round)
	senderSid := int(voteMsg.Sid)

	// Check msg round and sid
	if !mvba.checkMsgMark(senderId, senderRound, senderSid, msg) {
		return
	}

	logger.WithFields(logrus.Fields{
		"senderId":  senderId,
		"senderRound":  senderRound,
		"senderSid": senderSid,
		"flag":      flag,
	}).Info("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] Get Vote msg")

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
			"senderRound":  senderRound,
			"nowleader":  mvba.leader,
			"msgleader":  leader,
		}).Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] vote: leader in msg is mismatch")
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
		res, err := verfiySpbSig(mvba.leader, senderRound, senderSid, []byte(SPB_PHASE_1), leaderProposal, *signature, mvba.acs.Config.PublicKey)
		if err != nil || res == false {
			logger.WithFields(logrus.Fields{
				"error": err.Error(),
				"senderId":  senderId,
				"senderSid": senderSid,
				"nowleader":  mvba.leader,
				"msgleader":  leader,
				"leaderProposal":  len(leaderProposal),
				"signature":  len(*signature),
			}).Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] vote: verfiy signature failed.")
			return
		}

		newProposal :=  bytesAdd(leaderProposal, []byte(SPB_PHASE_2))
		proposalHash, _ := go_hotstuff.CreateDocumentHash(newProposal, mvba.acs.Config.PublicKey)
		marshalData := getMsgdata(mvba.leader, mvba.acs.round, mvba.sid, proposalHash)
		documentHash, _ = go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		err = go_hotstuff.VerifyPartSig(partSig, documentHash, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error":        err.Error(),
				"documentHash": hex.EncodeToString(documentHash),
			}).Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] vote: partSig not verified!")
			return
		}

		mvba.leaderVector = Vector{
			Id:    leader,
			Sid:   senderSid,
			Round:  senderRound,
			Proposal: leaderProposal,
			Signature: *signature,
		}
		
		// The check of sid and round is necessary, 
		// because after the checkMsgMark() is passed, the node's round and sid may change
		if senderRound == mvba.acs.round && senderSid == mvba.sid {
			mvba.lockSet.Lock()
			mvba.YFinal = append(mvba.YFinal, partSig)
			logger.WithFields(logrus.Fields{
				"senderId":  senderId,
				"senderRound":  senderRound,
				"senderSid": senderSid,
				"len(Y)":      len(mvba.YFinal),
				"len(N)":      len(mvba.NFinal),
			}).Info("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] APPEND Y")
			mvba.lockSet.Unlock()
		}
	} else {
		nullBytes := []byte("null" + "NO" + "Sid_" + strconv.Itoa(mvba.sid)) 
		marshalData := getMsgdata(mvba.leader, mvba.acs.round, mvba.sid, nullBytes)
		res, err := go_hotstuff.TVerify(mvba.acs.Config.PublicKey, *signature, marshalData)
		if err != nil || res == false {
			logger.WithField("error", err.Error()).Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] vote: verfiy signature failed.")
			return
		}

		unlockBytes := []byte("UnLocked")
		marshalData = getMsgdata(mvba.leader, mvba.acs.round, mvba.sid, unlockBytes)
		documentHash, _ = go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		err = go_hotstuff.VerifyPartSig(partSig, documentHash, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error":        err.Error(),
				"documentHash": hex.EncodeToString(documentHash),
			}).Warn("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] vote: partSig not verified!")
			return
		}

		// The check of sid and round is necessary, 
		// because after the checkMsgMark() is passed, the node's round and sid may change
		if senderRound == mvba.acs.round && senderSid == mvba.sid {
			mvba.lockSet.Lock()
			mvba.NFinal = append(mvba.NFinal, partSig)
			logger.WithFields(logrus.Fields{
				"senderId":  senderId,
				"senderRound":  senderRound,
				"senderSid": senderSid,
				"len(Y)":      len(mvba.YFinal),
				"len(N)":      len(mvba.NFinal),
			}).Info("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] APPEND N")
			mvba.lockSet.Unlock()
		}
	}

	if len(mvba.YFinal) + len(mvba.NFinal) == 2*mvba.acs.Config.F+1{
		if len(mvba.YFinal) == 2*mvba.acs.Config.F+1{
			leaderSignature, err := go_hotstuff.CreateFullSignature(documentHash, mvba.YFinal, mvba.acs.Config.PublicKey)
			if err != nil {
				logger.WithFields(logrus.Fields{
					"error":        err.Error(),
					"documentHash": hex.EncodeToString(documentHash),
				}).Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] vote: create signature failed!")
			}

			logger.WithFields(logrus.Fields{
				"leader":       leader,
				"leaderSid":    mvba.sid,
				"signature":    len(leaderSignature),
				// "documentHash": hex.EncodeToString(documentHash),
			}).Info("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] vote: create full signature of halt")
			
			// if len(leaderSignature)<256{
			// 	logger.WithFields(logrus.Fields{
			// 		"documentHash": hex.EncodeToString(documentHash),
			// 		"len(documentHash)": len(documentHash),
			// 		"signature":    len(leaderSignature),
			// 		"len(mvba.YFinal)": len(mvba.YFinal),
			// 	}).Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] vote: create full signature of halt")
				
			// 	for _, f := range mvba.YFinal {
			// 		fmt.Println("YFina_Id:",f.Id)
			// 		// // fmt.Println(len(f))
			// 		// fmt.Println(f)
			// 		// fmt.Println(len(f.Xi))
			// 		// fmt.Println(f.Xi)
			// 	}
			// 	fmt.Println(hex.EncodeToString(leaderSignature))
			// 	fmt.Println(signature)
			// 	return
			// }

			vector := Vector{
				Id:        leader,
				Sid:       mvba.sid,
				Round:     senderRound,
				Proposal:  leaderProposal,
				Signature: leaderSignature,
			}

			// The check of sid and round is necessary
			if senderRound == mvba.acs.round {
				mvba.broadcastHalt(vector)
			}
		} else {
			if len(mvba.NFinal) == 2*mvba.acs.Config.F+1{
				unLockSignature, err := go_hotstuff.CreateFullSignature(documentHash, mvba.NFinal, mvba.acs.Config.PublicKey)
				if err != nil {
					logger.WithFields(logrus.Fields{
						"error":        err.Error(),
						"documentHash": hex.EncodeToString(documentHash),
					}).Error("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] vote: create signature failed!")
				}
				mvba.Signature = unLockSignature
				// restart MVBA with own proposal 
				// mvba.acs.taskSignal <- "restart"
				fmt.Println("replica: ", mvba.acs.ID)
				fmt.Println("round: ", mvba.acs.round)
				fmt.Println("sid: ", mvba.sid)
				fmt.Println("len(Y): ", mvba.YFinal)
				fmt.Println("len(N): ", mvba.NFinal)
				fmt.Println("---------------- [NEXT_l_1] -----------------")
				fmt.Println("-                                           -")
				fmt.Println("-                                           -")
				fmt.Println("-                                           -")
				fmt.Println("-                 restart                   -")
				fmt.Println("-                                           -")
				fmt.Println("-                                           -")
				fmt.Println("-                  owner                    -")
				fmt.Println("-                                           -")
				fmt.Println("-                                           -")
				fmt.Println("-                                           -")
				fmt.Println("-                                           -")
				fmt.Println("---------------- [NEXT_l_2] -----------------")
				if mvba.complete == false {
					mvba.complete = true
					go mvba.startSpeedMvba(mvba.proposal)
				}
			}else{
				// restart MVBA with leader proposal
				// mvba.acs.taskSignal <- "restartWithLeaderProposal"
				fmt.Println("")
				fmt.Println("replica: ", mvba.acs.ID)
				fmt.Println("round: ", mvba.acs.round)
				fmt.Println("sid: ", mvba.sid)
				fmt.Println("len(Y): ", mvba.YFinal)
				fmt.Println("len(N): ", mvba.NFinal)
				fmt.Println("---------------- [NEXT_l_1] -----------------")
				fmt.Println("-                                           -")
				fmt.Println("-                                           -")
				fmt.Println("-                                           -")
				fmt.Println("-                 restart                   -")
				fmt.Println("-                                           -")
				fmt.Println("-                                           -")
				fmt.Println("-                 leader                    -")
				fmt.Println("-                                           -")
				fmt.Println("-                                           -")
				fmt.Println("-                                           -")
				fmt.Println("-                                           -")
				fmt.Println("---------------- [NEXT_l_2] -----------------")
				if mvba.complete == false {
					mvba.complete = true
					go mvba.startSpeedMvba(mvba.leaderVector.Proposal)
				}
			}
		}
	}
}



func (mvba *SpeedMvbaImpl) broadcastDone() {
	doneMsg := mvba.acs.DoneMsg(int(mvba.acs.ID), mvba.acs.round, mvba.sid)
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
	haltMsg := mvba.acs.HaltMsg(id, mvba.acs.round, mvba.sid, marshalData)
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
		nullBytes := []byte("null" + "NO" + "Sid_" + strconv.Itoa(mvba.sid))
		marshalData := getMsgdata(mvba.leader, mvba.acs.round, mvba.sid, nullBytes)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, mvba.acs.Config.PrivateKey, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("create the partial signature failed.")
		}
		partSigBytes, _ := json.Marshal(partSig)
		preVoteMsg = mvba.acs.PreVoteMsg(int(mvba.acs.ID), mvba.acs.round, mvba.sid, mvba.leader, flag, nil, nil, partSigBytes)
	} else {
		signatureBytes, _ := json.Marshal(vector.Signature)
		preVoteMsg = mvba.acs.PreVoteMsg(int(mvba.acs.ID), mvba.acs.round, mvba.sid, mvba.leader, flag, vector.Proposal, signatureBytes, nil)
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
		marshalData := getMsgdata(mvba.leader, mvba.acs.round, mvba.sid, proposalHash)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, mvba.acs.Config.PrivateKey, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("create the partial signature failed.")
		}
		partSigBytes, _ := json.Marshal(partSig)
		voteMsg = mvba.acs.VoteMsg(int(mvba.acs.ID), mvba.acs.round, mvba.sid, mvba.leader, flag, proposal, signatureBytes, partSigBytes)
	}else{
		unlockBytes := []byte("UnLocked")
		marshalData := getMsgdata(mvba.leader, mvba.acs.round, mvba.sid, unlockBytes)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, mvba.acs.Config.PrivateKey, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("create the partial signature failed.")
		}
		partSigBytes, _ := json.Marshal(partSig)
		voteMsg = mvba.acs.VoteMsg(int(mvba.acs.ID), mvba.acs.round, mvba.sid, mvba.leader, flag, nil, signatureBytes, partSigBytes)
	}

	// broadcast msg
	err := mvba.acs.Broadcast(voteMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Warn("Broadcast voteMsg failed.")
	}

	// send to self
	mvba.acs.MsgEntrance <- voteMsg
}

func (mvba *SpeedMvbaImpl) checkMsgMark(id int, round int, sid int, msg *pb.Msg) bool {
	if round < mvba.acs.round || (round == mvba.acs.round && sid < mvba.sid) {
		// Ignore messages from old sid or round
		logger.WithFields(logrus.Fields{
			"senderId":  id,
			"senderRound":  round,
			"senderSid": sid,
		}).Warn("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] Get msg of old sid or round")
		return false
	} else if round > mvba.acs.round || (round == mvba.acs.round && sid > mvba.sid) {
		// Save messages from future sid or round
		mvba.acs.insertMsg(round, sid, msg)
		logger.WithFields(logrus.Fields{
			"senderId":  id,
			"senderRound":  round,
			"senderSid": sid,
		}).Warn("[p_" + strconv.Itoa(int(mvba.acs.ID)) + "] [r_" + strconv.Itoa(mvba.acs.round) + "] [s_" + strconv.Itoa(mvba.sid) + "] [MVBA] Get future msg of sid or round, and save it in msg cache")
		return false
	} else {
		return true
	}
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

func (mvba *SpeedMvbaImpl) initStatus() {
	mvba.sid = 0
	mvba.start = false
	mvba.complete = false
	mvba.preLeader = make(map[int]int)
}

func BytesToInt(bys []byte) uint64 {
	if len(bys) < 8{
		logger.Error("BytesToInt: bytes is too small!")
		return 0
	}
	data := binary.BigEndian.Uint64(bys)
	return data
}

func verfiySpbSig(id int, round int, sid int, jBytes []byte, proposal []byte, signature tcrsa.Signature, publicKey *tcrsa.KeyMeta) (bool, error) {
	// deep copy
	newProposal := bytesAdd(proposal, jBytes)
	proposalHash, _ := go_hotstuff.CreateDocumentHash(newProposal, publicKey)
	marshalData := getMsgdata(id, round, sid, proposalHash)
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
