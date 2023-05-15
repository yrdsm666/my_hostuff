package sDumbo

import (
	// "bytes"
	// "context"
	"encoding/hex"
	"encoding/json"

	// "errors"
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
	// "sync"
)

// var logger = logging.GetLogger()

type CommonCoin interface {
	startCommonCoin(sidStr string)
	handleCommonCoinMsg(msg *pb.Msg)
	getCoin() tcrsa.Signature
}

type CommonCoinImpl struct {
	acs *CommonSubsetImpl

	sidStr       string
	complete     bool
	coinShares   []*tcrsa.SigShare
	DocumentHash []byte
	Coin         tcrsa.Signature
}

func NewCommonCoin(acs *CommonSubsetImpl) *CommonCoinImpl {
	cc := &CommonCoinImpl{
		acs:      acs,
		complete: false,
	}
	return cc
}

func (cc *CommonCoinImpl) startCommonCoin(sidStr string) {
	logger.Info("[replica_" + strconv.Itoa(int(cc.acs.ID)) + "] [sid_" + strconv.Itoa(cc.acs.Sid) + "] [CC] Start Provable Broadcast")

	cc.coinShares = make([]*tcrsa.SigShare, 0)
	cc.sidStr = sidStr

	id := int(cc.acs.ID)

	coinShare, err := go_hotstuff.TSign([]byte(cc.sidStr), cc.acs.Config.PrivateKey, cc.acs.Config.PublicKey)
	if err != nil {
		logger.WithField("error", err.Error()).Error("[CC] create the partial signature failed.")
	}

	coinShareMsg := cc.acs.Msg(pb.MsgType_COINSHARE, id, cc.acs.Sid, nil, coinShare)

	// broadcast msg
	err = cc.acs.Broadcast(coinShareMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Error("[CC] Broadcast failed.")
	}

	// vote self
	// prb.acs.MsgEntrance <- pbValueMsg
}

func (cc *CommonCoinImpl) handleCommonCoinMsg(msg *pb.Msg) {
	switch msg.Payload.(type) {
	case *pb.Msg_COINSHARE:
		coinShare := msg.GetCoinShare()
		partSig := &tcrsa.SigShare{}
		err := json.Unmarshal(coinShare.PartialSig, partSig)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal partSig failed.")
		}

		err = go_hotstuff.VerifyPartSig(partSig, []byte(cc.sidStr), cc.acs.Config.PublicKey)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error":        err.Error(),
				"documentHash": hex.EncodeToString(cc.DocumentHash),
			}).Warn("[CC] PBEchoVote: signature not verified!")
			return
		}

		cc.coinShares = append(cc.coinShares, partSig)

		if len(cc.coinShares) == 2*cc.acs.Config.F+1 {
			signature, _ := go_hotstuff.CreateFullSignature([]byte(cc.sidStr), cc.coinShares, cc.acs.Config.PublicKey)
			cc.Coin = signature
			cc.complete = true
			cc.acs.taskSignal <- "getCoin"
		}
		break
	default:
		logger.Warn("Receive unsupported msg")
	}
}

func (cc *CommonCoinImpl) getCoin() tcrsa.Signature {
	if cc.complete == false {
		logger.Error("[replica_" + strconv.Itoa(int(cc.acs.ID)) + "] [sid_" + strconv.Itoa(cc.acs.Sid) + "] [cc] Common coin is not complet")
		return nil
	}
	return cc.Coin
}
