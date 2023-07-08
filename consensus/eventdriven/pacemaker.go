package eventdriven

import (
	"context"
	"strconv"

	"github.com/wjbbig/go-hotstuff/consensus"
	pb "github.com/wjbbig/go-hotstuff/proto"
)

type Pacemaker interface {
	UpdateHighQC(qcHigh *pb.QuorumCert)
	OnBeat()
	OnNextSyncView()
	OnReceiverNewView(qc *pb.QuorumCert)
	Run(ctx context.Context)
}

type pacemakerImpl struct {
	ehs    *EventDrivenHotStuffImpl
	notify chan Event
}

func NewPacemaker(e *EventDrivenHotStuffImpl) *pacemakerImpl {
	return &pacemakerImpl{e, e.GetEvents()}
}

func (p *pacemakerImpl) UpdateHighQC(qcHigh *pb.QuorumCert) {
	logger.Info("[replica_" + strconv.Itoa(int(p.ehs.ID)) + "] [view_" + strconv.Itoa(int(p.ehs.View.ViewNum)) + "] UpdateHighQC.")
	block, _ := p.ehs.expectBlock(qcHigh.BlockHash)
	if block == nil {
		logger.Warn("[replica_" + strconv.Itoa(int(p.ehs.ID)) + "] [view_" + strconv.Itoa(int(p.ehs.View.ViewNum)) + "] Could not find block of new QC.")
		return
	}
	oldQCHighBlock, _ := p.ehs.BlockStorage.BlockOf(p.ehs.qcHigh)
	if oldQCHighBlock == nil {
		logger.Error("[replica_" + strconv.Itoa(int(p.ehs.ID)) + "] [view_" + strconv.Itoa(int(p.ehs.View.ViewNum)) + "] Block from the old qcHigh missing from storage.")
		return
	}

	if block.Height > oldQCHighBlock.Height {
		p.ehs.qcHigh = qcHigh
		p.ehs.bLeaf = block
	}
}

func (p *pacemakerImpl) OnBeat() {
	go p.ehs.OnPropose()
}

func (p *pacemakerImpl) OnNextSyncView() {
	logger.Warn("[replica_" + strconv.Itoa(int(p.ehs.ID)) + "] [view_" + strconv.Itoa(int(p.ehs.View.ViewNum)) + "] NewViewTimeout triggered.")
	// view change
	mu.Lock()
	p.ehs.View.ViewNum++
	mu.Unlock()
	p.ehs.View.Primary = p.ehs.GetLeader()
	// create a dummyNode
	dummyBlock := p.ehs.CreateLeaf(p.ehs.GetLeaf().Hash, nil, nil)
	p.ehs.SetLeaf(dummyBlock)
	dummyBlock.Committed = true
	_ = p.ehs.BlockStorage.Put(dummyBlock)
	// create a new view msg
	newViewMsg := p.ehs.Msg(pb.MsgType_NEWVIEW, nil, p.ehs.GetHighQC())
	// send msg
	if p.ehs.ID != p.ehs.GetLeader() {
		_ = p.ehs.Unicast(p.ehs.GetNetworkInfo()[p.ehs.GetLeader()], newViewMsg)
	} else {
		//send to self
		p.ehs.MsgEntrance <- newViewMsg
	}
	// clean the current proposal
	p.ehs.CurExec = consensus.NewCurProposal()
	p.ehs.TimeChan.HardStartTimer()
}

func (p *pacemakerImpl) OnReceiverNewView(qc *pb.QuorumCert) {
	p.ehs.lock.Lock()
	defer p.ehs.lock.Unlock()
	logger.Info("[replica_" + strconv.Itoa(int(p.ehs.ID)) + "] [view_" + strconv.Itoa(int(p.ehs.View.ViewNum)) + "] OnReceiveNewView.")
	p.ehs.emitEvent(ReceiveNewView)
	p.UpdateHighQC(qc)
}

func (p *pacemakerImpl) Run(ctx context.Context) {
	if p.ehs.ID == p.ehs.GetLeader() {
		go p.OnBeat()
	}
	go p.startNewViewTimeout(ctx)
	defer p.ehs.TimeChan.Stop()
	defer p.ehs.BatchTimeChan.Stop()
	// get events
	n := <-p.notify

	for {
		switch n {
		case ReceiveProposal:
			p.ehs.TimeChan.HardStartTimer()
		case QCFinish:
			p.OnBeat()
		case ReceiveNewView:
			p.OnBeat()
		}

		var ok bool
		select {
		case n, ok = <-p.notify:
			if !ok {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (p *pacemakerImpl) startNewViewTimeout(ctx context.Context) {
	for {
		select {
		case <-p.ehs.TimeChan.Timeout():
			// To keep liveness, multiply the timeout duration by 2
			p.ehs.Config.Timeout *= 2
			// init timer
			p.ehs.TimeChan.Init()
			// send new view msg
			p.OnNextSyncView()
		case <-p.ehs.BatchTimeChan.Timeout():
			logger.Debug("[replica_" + strconv.Itoa(int(p.ehs.ID)) + "] [view_" + strconv.Itoa(int(p.ehs.View.ViewNum)) + "] BatchTimeout triggered")
			p.ehs.BatchTimeChan.Init()
			go p.ehs.OnPropose()
		case <-ctx.Done():
			p.ehs.TimeChan.Stop()
			p.ehs.BatchTimeChan.Stop()
		}
	}
}
