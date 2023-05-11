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
	startSpeedMvba()
	handleSpeedMvbaMsg(msg *pb.Msg)
}

type SpeedMvbaImpl struct {
	acs           *CommonSubsetImpl
	
	// Proposal      []byte
	
	CoinShare     []*tcrsa.SigShare
	DocumentHash  []byte
	Signature     tcrsa.Signature

	// SPB           *NewStrongProvableBroadcast
	// cc            *NewCommonCoin
}

func NewSpeedMvba(acs *CommonSubsetImpl) *SpeedMvbaImpl {
	vba := &SpeedMvbaImpl{
		acs:             acs,
		//Proposal:      proposal,
	}
	return vba
}

func (vba *SpeedMvbaImpl) startSpeedMvba() {
	logger.Info("[replica_"+strconv.Itoa(int(vba.acs.ID))+"] [sid_"+strconv.Itoa(vba.acs.Sid)+"] [MVBA] Start Speed Mvba")

	// prb.EchoVote = make([]*tcrsa.SigShare, 0)

	// id := int(prb.acs.ID)

	// pbValueMsg := prb.acs.Msg(pb.MsgType_PBVALUE, id, prb.acs.Sid, prb.acs.proposalHash, nil)

	// // create msg hash
	// data := getMsgdata(id, prb.acs.Sid, prb.acs.proposalHash)
	// prb.DocumentHash, _ = go_hotstuff.CreateDocumentHash(data, prb.acs.Config.PublicKey)

	// // broadcast msg
	// err := prb.acs.Broadcast(pbValueMsg)
	// if err != nil {
	// 	logger.WithField("error", err.Error()).Error("Broadcast failed.")
	// }
}

func NewSpeedMvbaImpl(id int, Proposal []byte) *SpeedMvbaImpl {
	logger.Debugf("[COMMIN COIN] Start Provable Broadcast")
	ctx, cancel := context.WithCancel(context.Background())
	mvba := &CommonCoinImpll{
		Proposal:      proposal,
		cancel:        cancel,
	}

	msgEntrance := make(chan *pb.Msg)
	mvba.MsgEntrance = msgEntrance
	mvba.ID = uint32(id)
	logger.WithField("replicaID", id).Debug("[COMMIN COIN] Init command cache.")

	// read config
	mvba.Config = config.HotStuffConfig{}
	mvba.Config.ReadConfig()

	privateKey, err := go_hotstuff.ReadThresholdPrivateKeyFromFile(mvba.GetSelfInfo().PrivateKey)
	if err != nil {
		logger.Fatal(err)
	}
	mvba.Config.PrivateKey = privateKey

	go mvba.receiveMsg(ctx)
	r := 0

	for {
		sid := 0
		mvba.SPB = NewStrongProvableBroadcast(id, sid, proposal)
		sigma_1 := nspb.Signature1
		sigma_2 := nspb.Signature2
	}

	return mvba
}

func (mvba *ProvableBroadcastImpl) receiveMsg(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-mvba.MsgEntrance:
			go mvba.handleMsg(msg)
		}
	}
}

func (mvba *ProvableBroadcastImpl) handleMsg(msg *pb.Msg) {
	switch msg.Payload.(type) {
	case *pb.Msg_COINSHARE:
		coinShare := msg.GetCoinShare()
		partSig := &tcrsa.SigShare{}
		err := json.Unmarshal(coinShare.PartialSig, partSig)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal partSig failed.")
		}

		err := go_hotstuff.VerifyPartSig(partSig, mvba.DocumentHash, mvba.Config.PublicKey)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error":        err.Error(),
				"documentHash": hex.EncodeToString(mvba.DocumentHash),
			}).Warn("[COMMIN COIN] PBEchoVote: signature not verified!")
			return
		}

		mvba.CoinShare = append(mvba.CoinShare, partSig)

		if len(mvba.CoinShare) == 2*mvba.Config.F+1 {
			signature, _ := go_hotstuff.CreateFullSignature(mvba.DocumentHash, mvba.CoinShare, mvba.Config.PublicKey)
			mvba.Signature = signature
			mvba.cancel()
		} 
		break
	default:
		logger.Warn("Receive unsupported msg")
	}
}
