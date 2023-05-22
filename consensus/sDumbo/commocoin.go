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
	"sync"
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
	start        bool
	coinShares   []*tcrsa.SigShare
	DocumentHash []byte
	Coin         tcrsa.Signature

	lock         sync.Mutex
	waitstart    *sync.Cond
}

func NewCommonCoin(acs *CommonSubsetImpl) *CommonCoinImpl {
	cc := &CommonCoinImpl{
		acs:      acs,
		complete: false,
		start:    false,
	}
	cc.waitstart = sync.NewCond(&cc.lock)
	return cc
}

func (cc *CommonCoinImpl) startCommonCoin(sidStr string) {
	logger.Info("[replica_" + strconv.Itoa(int(cc.acs.ID)) + "] [sid_" + strconv.Itoa(cc.acs.Sid) + "] [CC] Start Common Coin with SidStr " + sidStr)

	cc.coinShares = make([]*tcrsa.SigShare, 0)
	cc.sidStr = sidStr

	id := int(cc.acs.ID)

	cc.DocumentHash, _ = go_hotstuff.CreateDocumentHash([]byte(cc.sidStr), cc.acs.Config.PublicKey)
	coinShare, err := go_hotstuff.TSign(cc.DocumentHash, cc.acs.Config.PrivateKey, cc.acs.Config.PublicKey)
	if err != nil {
		logger.Error("[replica_" + strconv.Itoa(int(cc.acs.ID)) + "] [sid_" + strconv.Itoa(cc.acs.Sid) + " [CC] create the partial signature failed.")
		return
	}

	cc.lock.Lock()
	cc.start = true
	cc.lock.Unlock()
	cc.waitstart.Broadcast()

	coinShareBytes, _ := json.Marshal(coinShare)
	coinShareMsg := cc.acs.CoinShareMsg(id, cc.acs.Sid, coinShareBytes)

	// broadcast msg
	err = cc.acs.Broadcast(coinShareMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Warn("[CC] Broadcast coinShareMsg failed.")
	}

	// vote self
	cc.acs.MsgEntrance <- coinShareMsg
}

func (cc *CommonCoinImpl) handleCommonCoinMsg(msg *pb.Msg) {
	if cc.complete == true {
		return
	}
	cc.lock.Lock()
	if cc.start == false {
		logger.Warn("[replica_" + strconv.Itoa(int(cc.acs.ID)) + "] [sid_" + strconv.Itoa(cc.acs.Sid) + " [CC] wait common coin start")
		cc.waitstart.Wait()
	}
	cc.lock.Unlock()
	switch msg.Payload.(type) {
	case *pb.Msg_CoinShare:
		coinShare := msg.GetCoinShare()
		senderId := int(coinShare.Id)
		senderSid := int(coinShare.Sid)
		logger.WithFields(logrus.Fields{
			"senderId":        senderId,
			"senderSid":       senderSid,
		}).Info("[replica_" + strconv.Itoa(int(cc.acs.ID)) + "] [sid_" + strconv.Itoa(cc.acs.Sid) + " [CC] Get share msg")
		
		partSig := &tcrsa.SigShare{}
		err := json.Unmarshal(coinShare.PartialSig, partSig)
		if err != nil {
			logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(cc.acs.ID)) + "] [sid_" + strconv.Itoa(cc.acs.Sid) + " [CC] Unmarshal partSig failed.")
		}

		err = go_hotstuff.VerifyPartSig(partSig, cc.DocumentHash, cc.acs.Config.PublicKey)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error":        err.Error(),
				"documentHash": hex.EncodeToString(cc.DocumentHash),
			}).Warn("[replica_" + strconv.Itoa(int(cc.acs.ID)) + "] [sid_" + strconv.Itoa(cc.acs.Sid) + " [CC] CoinShare: share signature not verified!")
			return
		}

		cc.coinShares = append(cc.coinShares, partSig)

		if len(cc.coinShares) == 2*cc.acs.Config.F+1 {
			signature, err := go_hotstuff.CreateFullSignature([]byte(cc.sidStr), cc.coinShares, cc.acs.Config.PublicKey)
			if err != nil {
				logger.WithFields(logrus.Fields{
					"error":        err.Error(),
				}).Error("[replica_" + strconv.Itoa(int(cc.acs.ID)) + "] [sid_" + strconv.Itoa(cc.acs.Sid) + "] [CC] CoinShare: create signature failed!")
				return
			}
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
