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
	DFlag         int
	DocumentHash  []byte
	Signature     tcrsa.Signature

	// SPB           *NewStrongProvableBroadcast
	// cc            *NewCommonCoin
}

func NewSpeedMvba(acs *CommonSubsetImpl) *SpeedMvbaImpl {
	mvba := &SpeedMvbaImpl{
		acs:             acs,
		//Proposal:      proposal,
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
		marshalData := getMsgdata(senderId, senderSid, senderProposal)
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
		signature1 := &tcrsa.Signature{}
		err := json.Unmarshal(spbFina.Signature1, signature1)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
		}
		signature2 := &tcrsa.Signature{}
		err = json.Unmarshal(spbFina.Signature2, signature2)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
		}

		marshalData1 := getMsgdata(senderId, senderSid, senderProposal)
		flag, err := go_hotstuff.TVerify(acs.Config.PublicKey, *signature1, marshalData)
		if ( err != nil || flag==false ) {
			logger.WithField("error", err.Error()).Error("[replica_"+strconv.Itoa(int(mvba.acs.ID))+"] [sid_"+strconv.Itoa(int(mvbaacs.Sid))+"] [MVBA] verfiy signature_1 failed.")
		}
		signatureBytes, _ := json.Marshal(signature2)
		marshalData2 := getMsgdata(senderId, senderSid, senderProposal)
		flag, err = go_hotstuff.TVerify(acs.Config.PublicKey, *signature2, signatureBytes)
		if ( err != nil || flag==false ) {
			logger.WithField("error", err.Error()).Error("[replica_"+strconv.Itoa(int(mvba.acs.ID))+"] [sid_"+strconv.Itoa(int(mvbaacs.Sid))+"] [MVBA] verfiy signature_2 failed.")
		}

		mvba.finalVectors = append(mvba.finalVectors, dVector)

		if len(mvba.finalVectors) == 2*mvba.acs.Config.F+1 && mvba.DFlag == 0 {
			mvba.DFlag = 1
			mvba.broadcastDone()
		}
		break
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