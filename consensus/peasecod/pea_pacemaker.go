package peasecod

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/niclabs/tcrsa"
	"github.com/sirupsen/logrus"

	go_hotstuff "github.com/wjbbig/go-hotstuff"
	pb "github.com/wjbbig/go-hotstuff/proto"
)

type Pacemaker interface {
	broadcatTimeout()
	handleTimeout(msg *pb.Msg)
	Run(ctx context.Context)
}

type pacemakerImpl struct {
	pea          *PeasecodImpl
	documentHash []byte
	timeouts     []*tcrsa.SigShare
}

func NewPacemaker(pea *PeasecodImpl) *pacemakerImpl {
	return &pacemakerImpl{pea: pea}
}

func (p *pacemakerImpl) broadcatTimeout() {
	p.documentHash, _ = go_hotstuff.CreateDocumentHash([]byte("epoch_"+strconv.Itoa(p.pea.epoch)), p.pea.Config.PublicKey)
	partSig, err := go_hotstuff.TSign(p.documentHash, p.pea.Config.PrivateKey, p.pea.Config.PublicKey)
	if err != nil {
		logger.Error("[p_" + strconv.Itoa(int(p.pea.ID)) + "] [r_" + strconv.Itoa(p.pea.epoch) + "] [PEA] create the partial signature failed.")
		return
	}
	partSigBytes, _ := json.Marshal(partSig)
	timeoutMsg := p.pea.TimeoutMsg(int(p.pea.ID), p.pea.epoch, partSigBytes)
	// broadcast msg
	err = p.pea.Broadcast(timeoutMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Warn("[PEA] Broadcast timeoutMsg failed.")
	}

	// vote self
	p.pea.MsgEntrance <- timeoutMsg
}

func (p *pacemakerImpl) handleTimeout(msg *pb.Msg) {
	timeoutMsg := msg.GetTimeout()
	senderId := int(timeoutMsg.Id)
	senderEpoch := int(timeoutMsg.Epoch)
	logger.WithFields(logrus.Fields{
		"senderId":    senderId,
		"senderEpoch": senderEpoch,
	}).Info("[p_" + strconv.Itoa(int(p.pea.ID)) + "] [r_" + strconv.Itoa(p.pea.epoch) + "] [PEA] Get timeout msg")

	partSig := &tcrsa.SigShare{}
	err := json.Unmarshal(timeoutMsg.PartialSig, partSig)
	if err != nil {
		logger.WithField("error", err.Error()).Error("[p_" + strconv.Itoa(int(p.pea.ID)) + "] [r_" + strconv.Itoa(p.pea.epoch) + "] [PEA] Unmarshal partSig failed.")
	}

	err = go_hotstuff.VerifyPartSig(partSig, p.documentHash, p.pea.Config.PublicKey)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"error":        err.Error(),
			"documentHash": hex.EncodeToString(p.documentHash),
		}).Warn("[p_" + strconv.Itoa(int(p.pea.ID)) + "] [r_" + strconv.Itoa(p.pea.epoch) + "] [PEA] Timeout: share signature not verified!")
		return
	}

	p.timeouts = append(p.timeouts, partSig)

	if len(p.timeouts) == 2*p.pea.Config.F+1 {
		signature, err := go_hotstuff.CreateFullSignature([]byte("epoch"+strconv.Itoa(p.pea.epoch)), p.timeouts, p.pea.Config.PublicKey)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error": err.Error(),
			}).Error("[p_" + strconv.Itoa(int(p.pea.ID)) + "] [r_" + strconv.Itoa(p.pea.epoch) + "] [PEA] Timeout: create signature failed!")
			return
		}
		p.pea.asyncProof = signature
		p.pea.maxProof = nil
		p.pea.asyncMode = true
		p.pea.startPessPath()
		p.timeouts = make([]*tcrsa.SigShare, 0)
	}
}

func (p *pacemakerImpl) Run(ctx context.Context) {
	p.timeouts = make([]*tcrsa.SigShare, 0)
	go p.startEpochTimeout(ctx)
	defer p.pea.TimeChan.Stop()
}

func (p *pacemakerImpl) startEpochTimeout(ctx context.Context) {
	for {
		select {
		case <-p.pea.TimeChan.Timeout():
			p.pea.TimeChan.Init()
			// send timeout msg
			p.broadcatTimeout()
		case <-ctx.Done():
			p.pea.TimeChan.Stop()
		}
	}
}
