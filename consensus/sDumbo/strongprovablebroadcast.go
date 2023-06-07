package sDumbo

import (
	// "bytes"
	// "encoding/hex"
	// "context"
	// "encoding/hex"
	"encoding/json"
	// "errors"
	// "github.com/golang/protobuf/proto"
	"github.com/niclabs/tcrsa"
	// "github.com/sirupsen/logrus"
	// "github.com/syndtr/goleveldb/leveldb"

	// "github.com/wjbbig/go-hotstuff/config"
	// "github.com/wjbbig/go-hotstuff/consensus"
	// "github.com/wjbbig/go-hotstuff/logging"

	// "os"
	"strconv"
	// "fmt"
)

// var logger = logging.GetLogger()

type StrongProvableBroadcast interface {
	startStrongProvableBroadcast(proposal []byte, sid int)
	controller(task string)
	getSignature1() tcrsa.Signature
	getSignature2() tcrsa.Signature
	getProvableBroadcast1Status() bool
	getProvableBroadcast2Status() bool
	getProvableBroadcast1() ProvableBroadcast
	getProvableBroadcast2() ProvableBroadcast
	// handleStrongProvableBroadcastMsg(msg *pb.Msg)
}

type StrongProvableBroadcastImpl struct {
	acs *CommonSubsetImpl

	// SPB information
	proposal []byte
	complete bool
	sid      int
	// start         bool

	// SPB components
	proBroadcast1 ProvableBroadcast
	proBroadcast2 ProvableBroadcast

	DocumentHash1 []byte
	DocumentHash2 []byte
	Signature1    tcrsa.Signature // PB_1 signature
	Signature2    tcrsa.Signature // PB_2 signature
}

func NewStrongProvableBroadcast(acs *CommonSubsetImpl) *StrongProvableBroadcastImpl {
	spb := &StrongProvableBroadcastImpl{
		acs:      acs,
		complete: false,
		// start:  false,
	}
	spb.proBroadcast1 = NewProvableBroadcast(acs)
	spb.proBroadcast2 = NewProvableBroadcast(acs)
	return spb
}

// sid: session id
func (spb *StrongProvableBroadcastImpl) startStrongProvableBroadcast(proposal []byte, sid int) {
	logger.Info("[p_" + strconv.Itoa(int(spb.acs.ID)) + "] [r_" + strconv.Itoa(spb.acs.round) + "] [s_" + strconv.Itoa(spb.sid) + "] [SPB] Start Strong Provable Broadcast")

	spb.proposal = proposal
	spb.sid = sid
	// spb.proBroadcast1 = NewProvableBroadcast(spb.acs)
	// spb.proBroadcast2 = NewProvableBroadcast(spb.acs)
	spb.acs.taskPhase = SPB_PHASE_1

	newProposal := bytesAdd(proposal, []byte(SPB_PHASE_1))

	go spb.proBroadcast1.startProvableBroadcast(newProposal, nil, sid, SPB_PHASE_1, CheckValue)
}


func (spb *StrongProvableBroadcastImpl) controller(task string) {
	if spb.complete {
		return
	}
	if task == "getPbValue_" + SPB_PHASE_1 {
		signature := spb.proBroadcast1.getSignature()
		spb.Signature1 = signature
		marshalData, _ := json.Marshal(signature)

		newProposal := bytesAdd(spb.proposal, []byte(SPB_PHASE_2))

		spb.acs.taskPhase = SPB_PHASE_2
		go spb.proBroadcast2.startProvableBroadcast(newProposal, marshalData, spb.sid, SPB_PHASE_2, verfiyThld)
	}
	if task == "getPbValue_" + SPB_PHASE_2 {
		signature := spb.proBroadcast2.getSignature()
		spb.Signature2 = signature
		spb.complete = true
		spb.acs.controller("getSpbValue")
	}
	if task == "spbEnd" {
		spb.complete = true
	}
}

func (spb *StrongProvableBroadcastImpl) getSignature1() tcrsa.Signature {
	if !spb.proBroadcast1.getStatus() {
		logger.Error("[p_" + strconv.Itoa(int(spb.acs.ID)) + "] [r_" + strconv.Itoa(spb.acs.round) + "] [s_" + strconv.Itoa(spb.sid) + "] [SPB] Provable Broadcast 1 is not complet")
		return nil
	}
	return spb.Signature1
}

func (spb *StrongProvableBroadcastImpl) getSignature2() tcrsa.Signature {
	if !spb.proBroadcast2.getStatus() {
		logger.Error("[p_" + strconv.Itoa(int(spb.acs.ID)) + "] [r_" + strconv.Itoa(spb.acs.round) + "] [s_" + strconv.Itoa(spb.sid) + "] [SPB] Provable Broadcast 2 is not complet")
		return nil
	}
	return spb.Signature2
}

func (spb *StrongProvableBroadcastImpl) getProvableBroadcast1Status() bool {
	return spb.proBroadcast1.getStatus()
}

func (spb *StrongProvableBroadcastImpl) getProvableBroadcast2Status() bool {
	return spb.proBroadcast2.getStatus()
}

func (spb *StrongProvableBroadcastImpl) getProvableBroadcast1() ProvableBroadcast {
	return spb.proBroadcast1
}

func (spb *StrongProvableBroadcastImpl) getProvableBroadcast2() ProvableBroadcast {
	return spb.proBroadcast2
}
