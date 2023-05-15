package sDumbo

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/golang/protobuf/proto"
	"github.com/niclabs/tcrsa"
	"github.com/sirupsen/logrus"
	"github.com/syndtr/goleveldb/leveldb"
	go_hotstuff "github.com/wjbbig/go-hotstuff"
	"github.com/wjbbig/go-hotstuff/config"
	"github.com/wjbbig/go-hotstuff/consensus"
	"github.com/wjbbig/go-hotstuff/logging"
	pb "github.com/wjbbig/go-hotstuff/proto"
	"os"
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
	acs           *CommonSubsetImpl
	
	proposal      []byte

	spb           StrongProvableBroadcast
	cc            CommonCoin
	doneVectors   []Vector
	finalVectors  []Vector
	leader        int
	DFlag         int
	Ready         int
	DocumentHash  []byte
	Signature     tcrsa.Signature
	complete      bool

	lock          sync.Mutex
	waitleader    *sync.Cond

	// SPB           *NewStrongProvableBroadcast
	// cc            *NewCommonCoin
}

func NewSpeedMvba(acs *CommonSubsetImpl) *SpeedMvbaImpl {
	mvba := &SpeedMvbaImpl{
		acs:             acs,
		complete:        false,
	}
	return mvba
}

func (mvba *SpeedMvbaImpl) startSpeedMvba(vectors []Vector) {
	logger.Info("[replica_"+strconv.Itoa(int(mvba.acs.ID))+"] [sid_"+strconv.Itoa(mvba.acs.Sid)+"] [MVBA] Start Speed Mvba")

	mvba.doneVectors = make([]Vector, 0)
	mvba.finalVectors = make([]Vector, 0)
	proposal, _ := json.Marshal(vectors)
	mvba.acs.taskPhase = "MVBA"
	mvba.proposal = proposal
	mvba.leader = 0
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
		if mvba.spb.getProvableBroadcast2Status() == true{
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
		mvba.leader = BytesToInt(signatureHash) % mvba.acs.Config.N + 1
		mvba.waitleader.Broadcast()
		for _, vector := range mvba.finalVectors{
			if vector.id == mvba.leader{
				mvba.broadcastHalt(vector)
				mvba.complete = true
				return
			}
		}
		mvba.Ready = 1
		for _, vector := range mvba.doneVectors{
			if vector.id == mvba.leader{
				mvba.broadcastPreVote(1)
				return
			}
		}
		mvba.broadcastPreVote(0)
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
			"senderSid":  senderSid,
		}).Info("[replica_"+strconv.Itoa(int(mvba.acs.ID))+"] [sid_"+strconv.Itoa(mvba.acs.Sid)+"] [MVBA] Get Done msg")
		senderProposal := doneMsg.Proposal
		signature := &tcrsa.Signature{}
		err := json.Unmarshal(doneMsg.Signature, signature)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
		}
		j := []byte("1")
		newProposal := append(senderProposal[:], j[0])
		marshalData := getMsgdata(senderId, senderSid, newProposal)
		flag, err := go_hotstuff.TVerify(mvba.acs.Config.PublicKey, *signature, marshalData)
		if ( err != nil || flag==false ) {
			logger.WithField("error", err.Error()).Error("[replica_"+strconv.Itoa(int(mvba.acs.ID))+"] [sid_"+strconv.Itoa(int(mvba.acs.Sid))+"] [MVBA] verfiy done signature failed.")
			return
		}
		dVector := Vector{
			id:            senderId,
			sid:           senderSid,
			proposal:      senderProposal,
			Signature:     *signature,
		}
		mvba.doneVectors = append(mvba.doneVectors, dVector)
		if len(mvba.doneVectors) == mvba.acs.Config.F+1 && mvba.DFlag == 0{
			mvba.DFlag = 1
			mvba.broadcastDone()
		}
		if len(mvba.doneVectors) == 2*mvba.acs.Config.F+1{
			// start Common coin
			mvba.controller("doneFinal")
		}
		break
	case *pb.Msg_SpbFinal:
		spbFinal := msg.GetSpbFinal()
		senderId := int(spbFinal.Id)
		senderSid := int(spbFinal.Sid)
		senderProposal := spbFinal.Proposal
		if senderSid != mvba.acs.Sid{
			logger.WithFields(logrus.Fields{
				"senderId":  senderId,
				"senderSid":  senderSid,
			}).Warn("[replica_"+strconv.Itoa(int(mvba.acs.ID))+"] [sid_"+strconv.Itoa(mvba.acs.Sid)+"] [MVBA] Get mismatch spbFinal msg")
			break
		}
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid":  senderSid,
		}).Info("[replica_"+strconv.Itoa(int(mvba.acs.ID))+"] [sid_"+strconv.Itoa(mvba.acs.Sid)+"] [MVBA] Get spbFinal msg")
		signature := &tcrsa.Signature{}
		err := json.Unmarshal(spbFina.Signature, signature)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
		}
		

		flag, err := verfiyFinalMsg(senderId, senderSid, proposal, *signature, mvba.acs.Config.PublicKey)
		if ( err != nil || flag==false ) {
			logger.WithField("error", err.Error()).Error("[replica_"+strconv.Itoa(int(mvba.acs.ID))+"] [sid_"+strconv.Itoa(int(mvbaacs.Sid))+"] [MVBA] verfiy signature failed.")
		}

		fVector := Vector{
			id:            senderId,
			sid:           senderSid,
			proposal:      senderProposal,
			Signature:     *signature,
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
		finalVector := &Vector
		err := json.Unmarshal(senderFinal, finalVector)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
		}
		if mvba.leader == 0{
			mvba.waitleader.Wait()
		}
		if mvba.leader != finalVector.id {
			return
		}
		fId := finalVector.id
		fSid := finalVector.sid
		fProposal := finalVector.proposal
		fsignature := finalVector.Signature

		flag, err := verfiyFinalMsg(fId, fSid, fProposal, fsignature, mvba.acs.Config.PublicKey)
		if ( err != nil || flag==false ) {
			logger.WithField("error", err.Error()).Error("[replica_"+strconv.Itoa(int(mvba.acs.ID))+"] [sid_"+strconv.Itoa(int(mvbaacs.Sid))+"] [MVBA] verfiy signature failed.")
			return
		}
		if mvba.complete == false{
			mvba.complete = true
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
	id := int(mvba.acs.ID)
	haltMsg := mvba.acs.Msg(pb.MsgType_PreVote, id, mvba.acs.Sid, mvba.leader, vector.proposal, vector.signature)
	// broadcast msg
	err := mvba.acs.Broadcast(haltMsg)
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

func verfiyFinalMsg(id int, sid int, proposal []byte, signature tcrsa.Signature, publicKey *tcrsa.KeyMeta) bool{
	j := []byte("2")
	newProposal := append(senderProposal[:], j[0])
	marshalData := getMsgdata(senderId, senderSid, newProposal)

	flag, err := go_hotstuff.TVerify(publicKey, signature, marshalData)
	if ( err != nil || flag==false ) {
		logger.WithField("error", err.Error()).Error("verfiyFinalMsg failed.")
		return false
	}
	return true

}