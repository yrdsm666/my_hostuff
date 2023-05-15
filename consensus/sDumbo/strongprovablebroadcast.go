package sDumbo

import (
	// "bytes"
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
	// "sync"
)

// var logger = logging.GetLogger()

type StrongProvableBroadcast interface {
	startStrongProvableBroadcast(proposal []byte)
	controller(task string)
	getSignature1() tcrsa.Signature
	getSignature2() tcrsa.Signature
	getProvableBroadcast1Status() bool
	getProvableBroadcast2Status() bool
	// handleStrongProvableBroadcastMsg(msg *pb.Msg)
}

type StrongProvableBroadcastImpl struct {
	acs *CommonSubsetImpl

	proposal []byte
	// complete      bool
	proBroadcast1 ProvableBroadcast
	proBroadcast2 ProvableBroadcast
	// EchoVote      []*tcrsa.SigShare
	// ReadyVote     []*tcrsa.SigShare
	DocumentHash1 []byte
	DocumentHash2 []byte
	Signature1    tcrsa.Signature
	Signature2    tcrsa.Signature
}

func NewStrongProvableBroadcast(acs *CommonSubsetImpl) *StrongProvableBroadcastImpl {
	spb := &StrongProvableBroadcastImpl{
		acs: acs,
		// complete:        false,
	}
	return spb
}

// sid: session id
func (spb *StrongProvableBroadcastImpl) startStrongProvableBroadcast(proposal []byte) {
	logger.Info("[replica_" + strconv.Itoa(int(spb.acs.ID)) + "] [sid_" + strconv.Itoa(spb.acs.Sid) + "] [SPB] Start Strong Provable Broadcast")

	spb.acs.taskPhase = "SPB"
	spb.proposal = proposal
	// spb.Signature1 = tcrsa.SigShare{}
	// spb.Signature2 = tcrsa.SigShare{}
	spb.proBroadcast1 = NewProvableBroadcast(spb.acs)
	spb.proBroadcast2 = NewProvableBroadcast(spb.acs)
	j := []byte("1")
	newProposal := append(proposal[:], j[0])

	go spb.proBroadcast1.startProvableBroadcast(newProposal, nil, CheckValue)
}

func (spb *StrongProvableBroadcastImpl) controller(task string) {
	if task == "getPbValue" {
		if spb.proBroadcast2.getStatus() == false {
			signature := spb.proBroadcast1.getSignature()
			spb.Signature1 = signature
			marshalData, _ := json.Marshal(signature)
			j := []byte("2")
			newProposal := append(spb.proposal[:], j[0])
			go spb.proBroadcast2.startProvableBroadcast(newProposal, marshalData, verfiyThld)
		} else {
			signature := spb.proBroadcast2.getSignature()
			spb.acs.taskSignal <- "getSpbValue"
			spb.Signature2 = signature
			// go spb.acs.broadcastPbFinal(signature2)
		}
	}
}

func (spb *StrongProvableBroadcastImpl) getSignature1() tcrsa.Signature {
	if spb.proBroadcast1.getStatus() == false {
		logger.Error("[replica_" + strconv.Itoa(int(spb.acs.ID)) + "] [sid_" + strconv.Itoa(spb.acs.Sid) + "] [SPB] Provable Broadcast 1 is not complet")
		return nil
	}
	return spb.Signature1
}

func (spb *StrongProvableBroadcastImpl) getSignature2() tcrsa.Signature {
	if spb.proBroadcast2.getStatus() == false {
		logger.Error("[replica_" + strconv.Itoa(int(spb.acs.ID)) + "] [sid_" + strconv.Itoa(spb.acs.Sid) + "] [SPB] Provable Broadcast 2 is not complet")
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
