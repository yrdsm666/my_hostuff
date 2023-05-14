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
	acs           *CommonSubsetImpl

	proposal      []byte
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
		acs:             acs,
		// complete:        false,
	}
	return spb
}

// sid: session id
func (spb *StrongProvableBroadcastImpl) startStrongProvableBroadcast(proposal []byte) {
	logger.Info("[replica_"+strconv.Itoa(int(prb.acs.ID))+"] [sid_"+strconv.Itoa(prb.acs.Sid)+"] [SPB] Start Strong Provable Broadcast")

	spb.acs.taskPhase = "SPB"
	spb.proposal = proposal
	// spb.Signature1 = tcrsa.SigShare{}
	// spb.Signature2 = tcrsa.SigShare{}
	spb.proBroadcast1 = NewProvableBroadcast(spb.acs)
	spb.proBroadcast2 = NewProvableBroadcast(spb.acs)
	newProposal = proposal + []byte(1)

	go spb.proBroadcast1.startProvableBroadcast(newProposal, nil, CheckValue)
}

func (spb *StrongProvableBroadcastImpl) controller(task string) {
	if task=="getPbValue"{
		if spb.proBroadcast2.complete == false{
			signature := spb.proBroadcast1.getSignature()
			spb.Signature1 = signature
			marshalData, _ := json.Marshal(signature1)
			newProposal := spb.proposal + []byte(2)
			go acs.proBroadcast2.startProvableBroadcast(spb.proposal, marshalData, verfiyThld)
		}else{
			signature := spb.proBroadcast2.getSignature()
			spb.acs.taskSignal <- "getSpbValue"
			// spb.Signature2 = signature
			// go spb.acs.broadcastPbFinal(signature2)
		}
	}
}

func (spb *StrongProvableBroadcastImpl) getSignature1() tcrsa.Signature {
	if spb.proBroadcast1.complete == false{
		logger.Error("[replica_"+strconv.Itoa(int(spb.acs.ID))+"] [sid_"+strconv.Itoa(spb.acs.Sid)+"] [SPB] Provable Broadcast 1 is not complet")
		return nil
	}
	return spb.Signature1
}

func (spb *StrongProvableBroadcastImpl) getSignature2() tcrsa.Signature {
	if spb.proBroadcast2.complete == false{
		logger.Error("[replica_"+strconv.Itoa(int(spb.acs.ID))+"] [sid_"+strconv.Itoa(spb.acs.Sid)+"] [SPB] Provable Broadcast 2 is not complet")
		return nil
	}
	return spb.Signature2
}

func (spb *StrongProvableBroadcastImpl) getProvableBroadcast1Status() bool {
	return spb.proBroadcast1.complete
}

func (spb *StrongProvableBroadcastImpl) getProvableBroadcast2Status() bool {
	return spb.proBroadcast2.complete
}
