package sDumbo

import (
	"bytes"
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
	startSpeedMvba(vectors []Vector)
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
	mvba.acs.taskPhase = "MVBA"
	proposal, _ := json.Marshal(vectors)
	mvba.proposal = proposal
	fmt.Println("")
	fmt.Println("---------------- [PB_END_1] -----------------")
	fmt.Println("replica: ", mvba.acs.ID)
	for i := 0; i < len(vectors); i++ {
		fmt.Println("node: ", vectors[i].id)
		fmt.Println("Sid: ", vectors[i].sid)
		fmt.Println("proposalLen: ", len(vectors[i].proposal))
		fmt.Println("sigHashLen: ", len(vectors[i].Signature))
	}
	fmt.Println(" GOOD WORK!.")
	fmt.Println("---------------- [PB_END_2] -----------------")

	fmt.Println("\n")
	fmt.Println("---------------- [END_1] -----------------")
	fmt.Println("副本：", mvba.acs.ID)
	var resVectors []Vector
	err := json.Unmarshal(proposal, &resVectors)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Unmarshal res failed.")
	}
	fmt.Println("resVectorsLen:", len(resVectors))
	for _, v := range resVectors {
		fmt.Printf("type(v): %T", v)
		fmt.Println("")
		fmt.Println("v: ", v)
		fmt.Println("pnode: ", v.id)
		fmt.Println("pSid: ", v.sid)
		fmt.Println("proposalLen: ", len(v.proposal))
		fmt.Println("sigHashLen: ", len(v.Signature))
		// rv := hex.EncodeToString(v.proposal)
		// fmt.Println("proposalLen: ", rv[10:50])
	}
	fmt.Println(" GOOD WORK!.")
	fmt.Println("---------------- [END_2] -----------------")



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
				logger.WithField("error", err.Error()).Error("Broadcast failed.")
			}
			// vote self
			mvba.acs.MsgEntrance <- spbFinalMsg
		}else{
			logger.Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Strong Provable Broadcast is not complet")
		}
	case "spbFinal":
		// start common coin
		strSid := fmt.Sprintf("%d", mvba.acs.Sid)
		go mvba.cc.startCommonCoin("coin_sid_" + strSid)
	case "spbEnd":
		mvba.spb.controller(task)
	case "getCoin":
		signature := mvba.cc.getCoin()
		marshalData, _ := json.Marshal(signature)
		signatureHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		mvba.lock.Lock()
		mvba.leader = BytesToInt(signatureHash)%mvba.acs.Config.N + 1
		mvba.lock.Unlock()
		logger.Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] get the leader: " + strconv.Itoa(mvba.leader))
		//====
		// fmt.Println("---------------- [COIN_END_1] -----------------")
		// fmt.Println("Replica: ", mvba.acs.ID)
		// fmt.Println("final vectors: ",len(mvba.finalVectors))
		// for _, vector := range mvba.finalVectors {
		// 	fmt.Println("node: ", vector.id)
		// 	fmt.Println("Sid: ", vector.sid)
		// 	fmt.Println("proposalLen: ", len(vector.proposal))
		// }
		// lockVectors := mvba.spb.getProvableBroadcast2().getLockVectors()
		// fmt.Println("-----------------------------------------------")
		// fmt.Println("lock vectors: ",len(lockVectors))
		// for _, vector := range lockVectors {
		// 	fmt.Println("node: ", vector.id)
		// 	fmt.Println("Sid: ", vector.sid)
		// 	fmt.Println("proposalLen: ", len(vector.proposal))
		// }
		// fmt.Println("---------------- [COIN_END_2] -----------------")
		//====
		mvba.waitleader.Broadcast()
		for _, vector := range mvba.finalVectors {
			if vector.id == mvba.leader {
				mvba.broadcastHalt(vector)
				mvba.complete = true
				mvba.leaderVector = vector
				mvba.acs.taskSignal <- "end"
				return
			}
		}
		mvba.Ready = 1
		lockVectors := mvba.spb.getProvableBroadcast2().getLockVectors()
		for _, vector := range lockVectors {
			if vector.id == mvba.leader {
				mvba.broadcastPreVote(1, vector)
				return
			}
		}
		mvba.broadcastPreVote(0, Vector{})
	}
}

func (mvba *SpeedMvbaImpl) handleSpeedMvbaMsg(msg *pb.Msg) {
	if mvba.complete == true{
		return
	}
	switch msg.Payload.(type) {
	case *pb.Msg_PbValue:
		if mvba.acs.taskPhase == "SPB_1" {
			mvba.spb.getProvableBroadcast1().handleProvableBroadcastMsg(msg)
		} else if mvba.acs.taskPhase == "SPB_2" {
			mvba.spb.getProvableBroadcast2().handleProvableBroadcastMsg(msg)
		}
		break
	case *pb.Msg_PbEcho:
		if mvba.acs.taskPhase == "SPB_1" {
			mvba.spb.getProvableBroadcast1().handleProvableBroadcastMsg(msg)
		} else if mvba.acs.taskPhase == "SPB_2" {
			mvba.spb.getProvableBroadcast2().handleProvableBroadcastMsg(msg)
		}
		break
	case *pb.Msg_CoinShare:
		mvba.cc.handleCommonCoinMsg(msg)
	case *pb.Msg_Done:
		doneMsg := msg.GetDone()
		senderId := int(doneMsg.Id)
		senderSid := int(doneMsg.Sid)
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get Done msg")
		// senderProposal := doneMsg.Proposal
		// signature := &tcrsa.Signature{}
		// err := json.Unmarshal(doneMsg.Signature, signature)
		// if err != nil {
		// 	logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
		// }

		// flag, err := verfiySpbSig(senderId, senderSid, []byte("SPB_1"), senderProposal, *signature, mvba.acs.Config.PublicKey)
		// if err != nil || flag == false {
		// 	logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvba.acs.Sid)) + "] [MVBA] done: verfiy signature failed.")
		// 	return
		// }

		dVector := Vector{
			id:        senderId,
			sid:       senderSid,
			// proposal:  senderProposal,
			// Signature: *signature,
		}
		mvba.doneVectors = append(mvba.doneVectors, dVector)
		if len(mvba.doneVectors) == mvba.acs.Config.F+1 && mvba.DFlag == 0 {
			mvba.DFlag = 1
			mvba.broadcastDone()
		}
		if len(mvba.doneVectors) == 2*mvba.acs.Config.F+1 {
			// end the spb
			// mvba.controller("doneFinal")
			mvba.controller("spbFinal")
			mvba.controller("spbEnd")
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
			}).Warn("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get mismatch sid spbFinal msg")
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

		flag, err := verfiySpbSig(senderId, senderSid, []byte("SPB_2"), senderProposal, *signature, mvba.acs.Config.PublicKey)
		if err != nil || flag == false {
			logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvba.acs.Sid)) + "] [MVBA] spbFinal: verfiy signature failed.")
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
			// start common coin
			mvba.controller("spbFinal")
			// fmt.Println("---------------- [SPB_END_1] -----------------")
			// fmt.Println("replica: ", mvba.acs.ID)
			// for i := 0; i < 2*mvba.acs.Config.F+1; i++ {
			// 	fmt.Println("node: ", mvba.finalVectors[i].id)
			// 	fmt.Println("Sid: ", mvba.finalVectors[i].sid)
			// 	fmt.Println("proposalLen: ", len(mvba.finalVectors[i].proposal))
			// }
			// fmt.Println(" GOOD WORK!.")
			// fmt.Println("---------------- [SPB_END_2] -----------------")
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
		mvba.lock.Lock()
		if mvba.leader == 0 {
			mvba.waitleader.Wait()
		}
		mvba.lock.Unlock()
		if mvba.leader != finalVector.id {
			return
		}
		senderId := int(haltMsg.Id)
		senderSid := int(haltMsg.Sid)
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get halt msg")
		fId := finalVector.id
		fSid := finalVector.sid
		fProposal := finalVector.proposal
		fsignature := finalVector.Signature

		flag, err := verfiySpbSig(fId, fSid, []byte("SPB_2"), fProposal, fsignature, mvba.acs.Config.PublicKey)
		if err != nil || flag == false {
			logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvba.acs.Sid)) + "] [MVBA] halt: verfiy signature failed.")
			return
		}
		if mvba.complete == false {
			mvba.complete = true
			mvba.leaderVector = Vector{
				id:    fId,
				sid:   fSid,
				proposal: fProposal,
				Signature: fsignature,
			}
			logger.WithFields(logrus.Fields{
				"leaderId":  mvba.leaderVector.id,
				"leaderSid": mvba.leaderVector.sid,
				"leaderProposalLen":  len(mvba.leaderVector.proposal),
				"leaderSignatureLen": len(mvba.leaderVector.Signature),
			}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] success end the mvba!")
			mvba.acs.taskSignal <- "end"
		}
	case *pb.Msg_PreVote:
		preVoteMsg := msg.GetPreVote()
		flag := preVoteMsg.Flag
		senderId := int(preVoteMsg.Id)
		senderSid := int(preVoteMsg.Sid)
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get PreVote msg")
		senderProposal := preVoteMsg.Proposal
		leader := int(preVoteMsg.Leader)
		if mvba.leader != leader {
			return
		}
		if flag == 1 && mvba.NFlag == 0{
			signature := &tcrsa.Signature{}
			err := json.Unmarshal(preVoteMsg.Signature, signature)
			if err != nil {
				logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
			}
	
			flag, err := verfiySpbSig(mvba.leader, senderSid, []byte("SPB_1"), senderProposal, *signature, mvba.acs.Config.PublicKey)
			if err != nil || flag == false {
				logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvba.acs.Sid)) + "] [MVBA] preVote: verfiy signature failed.")
				return
			}
			if mvba.NFlag == 0{
				mvba.YFlag = 1
				mvba.broadcastVote(1, senderProposal, *signature)
			}
		} else{
			if mvba.NFlag == 1{
				// NFlag == 1 means that the prevote phase has ended
				// so the following operations are unnecessary
				return
			}
			partSig := &tcrsa.SigShare{}
			err := json.Unmarshal(preVoteMsg.PartialSig, partSig)
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
	case *pb.Msg_Vote:
		voteMsg := msg.GetVote()
		flag := voteMsg.Flag
		senderId := int(voteMsg.Id)
		senderSid := int(voteMsg.Sid)
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Info("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] Get Vote msg")
		leaderProposal := voteMsg.Proposal
		leader := int(voteMsg.Leader)
		if mvba.leader != leader {
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
			res, err := verfiySpbSig(mvba.leader, senderSid, []byte("SPB_1"), leaderProposal, *signature, mvba.acs.Config.PublicKey)
			if err != nil || res == false {
				logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(int(mvba.acs.Sid)) + "] [MVBA] vote: verfiy signature failed.")
				return
			}

			newProposal := make([]byte, 0)
			newProposal = append(newProposal, leaderProposal...)
			jBytes := []byte("SPB_2")
			newProposal = append(newProposal, jBytes...)
			// j := []byte("SPB_2")
			// newProposal := append(leaderProposal[:], j[0])
			marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, newProposal)
			documentHash, _ = go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
			err = go_hotstuff.VerifyPartSig(partSig, documentHash, mvba.acs.Config.PublicKey)
			if err != nil {
				logger.WithFields(logrus.Fields{
					"error":        err.Error(),
					"documentHash": hex.EncodeToString(documentHash),
				}).Warn("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] vote: partSig not verified!")
				return
			}
			mvba.leaderVector = Vector{
				id:    mvba.leader,
				sid:   mvba.acs.Sid,
				proposal: leaderProposal,
				Signature: *signature,
			}

			mvba.YFinal = append(mvba.YFinal, partSig)
		}else{
			nullBytes := []byte("null" + "NO")
			marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, nullBytes)
			documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
			res, err := go_hotstuff.TVerify(mvba.acs.Config.PublicKey, *signature, documentHash)
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
			mvba.NFinal = append(mvba.NFinal, partSig)
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
				vector := Vector{
					id:        leader,
					sid:       mvba.acs.Sid,
					proposal:  leaderProposal,
					Signature: leaderSignature,
				}
				mvba.broadcastHalt(vector)
				mvba.complete = true
				mvba.leaderVector = vector
				mvba.acs.taskSignal <- "end"
			} else {
				if len(mvba.NFinal) == 2*mvba.acs.Config.F+1{
					unLockSignature, err := go_hotstuff.CreateFullSignature(documentHash, mvba.NFinal, mvba.acs.Config.PublicKey)
					if err != nil {
						logger.WithFields(logrus.Fields{
							"error":        err.Error(),
							"documentHash": hex.EncodeToString(documentHash),
						}).Error("[replica_" + strconv.Itoa(int(mvba.acs.ID)) + "] [sid_" + strconv.Itoa(mvba.acs.Sid) + "] [MVBA] vote: create signature failed!")
					}
					// next mvba
					mvba.Signature = unLockSignature
					mvba.acs.taskSignal <- "restart"
				}else{
					mvba.acs.taskSignal <- "restartWithLeaderProposal"
				}
			}
			
		}

	default:
		logger.Warn("[MVBA] Receive unsupported msg")
	}
}

func (mvba *SpeedMvbaImpl) broadcastDone() {
	doneMsg := mvba.acs.DoneMsg(int(mvba.acs.ID), mvba.acs.Sid)
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
	haltMsg := mvba.acs.HaltMsg(id, mvba.acs.Sid, marshalData)
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
		partSigBytes, _ := json.Marshal(partSig)
		preVoteMsg = mvba.acs.PreVoteMsg(int(mvba.acs.ID), mvba.acs.Sid, mvba.leader, flag, nil, nil, partSigBytes)
	}else{
		signatureBytes, _ := json.Marshal(vector.Signature)
		preVoteMsg = mvba.acs.PreVoteMsg(int(mvba.acs.ID), mvba.acs.Sid, mvba.leader, flag, vector.proposal, signatureBytes, nil)
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
	signatureBytes, _ := json.Marshal(signature)
	if flag == 0 {
		unlockBytes := []byte("UnLocked")
		marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, unlockBytes)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, mvba.acs.Config.PrivateKey, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("create the partial signature failed.")
		}
		partSigBytes, _ := json.Marshal(partSig)
		voteMsg = mvba.acs.VoteMsg(int(mvba.acs.ID), mvba.acs.Sid, mvba.leader, flag, nil, signatureBytes, partSigBytes)
	}else{
		// j := []byte("2")
		// newProposal := append(proposal[:], j[0])
		newProposal := make([]byte, 0)
		newProposal = append(newProposal, proposal...)
		jBytes := []byte("SPB_2")
		newProposal = append(newProposal, jBytes...)
		marshalData := getMsgdata(mvba.leader, mvba.acs.Sid, newProposal)
		documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, mvba.acs.Config.PublicKey)
		partSig, err := go_hotstuff.TSign(documentHash, mvba.acs.Config.PrivateKey, mvba.acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("create the partial signature failed.")
		}
		partSigBytes, _ := json.Marshal(partSig)
		voteMsg = mvba.acs.VoteMsg(int(mvba.acs.ID), mvba.acs.Sid, mvba.leader, flag, proposal, signatureBytes, partSigBytes)
	}

	// broadcast msg
	err := mvba.acs.Broadcast(voteMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Broadcast failed.")
	}

	// send to self
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

func BytesToInt(bys []byte) int {
	bytebuff := bytes.NewBuffer(bys)
	var data int64
	binary.Read(bytebuff, binary.BigEndian, &data)
	return int(data)
}

func verfiySpbSig(id int, sid int, jBytes []byte, proposal []byte, signature tcrsa.Signature, publicKey *tcrsa.KeyMeta) (bool, error) {
	// deep copy
	newProposal := make([]byte, 0)
	newProposal = append(newProposal, proposal...)
	newProposal = append(newProposal, jBytes...)

	marshalData := getMsgdata(id, sid, newProposal)

	flag, err := go_hotstuff.TVerify(publicKey, signature, marshalData)
	if err != nil || flag == false {
		logger.WithField("error", err.Error()).Error("verfiySpbSig failed.")
		return false, err
	}
	return true, nil

}

// return proposal + jBytes
func bytesAdd(proposal []byte, jBytes []byte) []byte {
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
