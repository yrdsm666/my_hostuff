package peasecod

import (
	"bytes"
	"context"
	"sync"

	//"errors"
	//"github.com/golang/protobuf/proto"

	//"github.com/syndtr/goleveldb/leveldb"

	"github.com/niclabs/tcrsa"
	go_hotstuff "github.com/wjbbig/go-hotstuff"
	"github.com/wjbbig/go-hotstuff/config"
	"github.com/wjbbig/go-hotstuff/consensus"
	"github.com/wjbbig/go-hotstuff/consensus/eventdriven"
	"github.com/wjbbig/go-hotstuff/consensus/sDumbo"
	"github.com/wjbbig/go-hotstuff/logging"
	pb "github.com/wjbbig/go-hotstuff/proto"

	//"os"
	"strconv"
	"strings"

	//"sync"
	"fmt"
	// "time"
)

var logger = logging.GetLogger()

// type PeasecodParameter struct {
// 	TxnSet      go_hotstuff.CmdSet
// 	ResEntrance chan *pb.Msg
// }

type PeasecodImpl struct {
	consensus.ParallelImpl

	epoch       int
	latestBlock *consensus.FastResult
	asyncMode   bool
	asyncProof  tcrsa.Signature
	maxProof    *pb.QuorumCert
	pcBlocks    *pb.Block

	pacemaker Pacemaker
	hotstuff  consensus.HotStuff
	acs       consensus.Asynchronous
	// acs sDumbo.CommonSubsetImpl

	cancel  context.CancelFunc
	lock    sync.Mutex
	waitAsc *sync.Cond
}

func NewPeasecod(id int) *PeasecodImpl {
	logger.Debugf("[PEA] Start Peasecod.")
	ctx, cancel := context.WithCancel(context.Background())
	pea := &PeasecodImpl{
		epoch:  0,
		cancel: cancel,
	}

	pea.MsgEntrance = make(chan *pb.Msg)
	pea.FastPathRes = make(chan *consensus.FastResult)
	pea.PessPathRes = make(chan []consensus.PessResult)
	pea.ID = uint32(id)

	// create txn cache
	pea.TxnSet = go_hotstuff.NewCmdSet()
	logger.WithField("replicaID", id).Debug("[PEA] Init command cache.")

	// read config
	pea.Config = config.HotStuffConfig{}
	pea.Config.ReadConfig()

	// read private key
	privateKey, err := go_hotstuff.ReadThresholdPrivateKeyFromFile(pea.GetSelfInfo().PrivateKey)
	if err != nil {
		logger.Fatal(err)
	}
	pea.Config.PrivateKey = privateKey

	// init timer and stop it
	peaTimeout := pea.Config.Timeout * 5
	pea.TimeChan = go_hotstuff.NewTimer(peaTimeout)
	pea.TimeChan.Init()

	pea.asyncMode = false
	pea.hotstuff = eventdriven.NewEventDrivenHotStuff(id, handleMethod, pea.TxnSet, pea.FastPathRes)
	pea.acs = sDumbo.NewCommonSubset(int(pea.ID), pea.TxnSet, pea.PessPathRes)

	pea.pacemaker = NewPacemaker(pea)
	pea.waitAsc = sync.NewCond(&pea.lock)

	go pea.receiveMsg(ctx)
	go pea.receiveRes(ctx)
	go pea.pacemaker.Run(ctx)

	return pea
}

func (pea *PeasecodImpl) receiveMsg(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-pea.MsgEntrance:
			go pea.handleMsg(msg)
		}
	}
}

func (pea *PeasecodImpl) receiveRes(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case res := <-pea.FastPathRes:
			go pea.handleFastRes(res)
		case res := <-pea.PessPathRes:
			logger.Info("Good gooooooood work")
			go pea.handlePessRes(res)
		}
	}
}

func (pea *PeasecodImpl) handleMsg(msg *pb.Msg) {
	switch msg.Payload.(type) {
	case *pb.Msg_Request:
		// request := msg.GetRequest()
		// // logger.WithField("content", request.String()).Debug("[pea] Get request msg.")
		// // put the cmd into the cmdset
		// pea.TxnSet.Add(request.Cmd)
		pea.hotstuff.GetMsgEntrance() <- msg
	case *pb.Msg_Prepare:
		pea.hotstuff.GetMsgEntrance() <- msg
	case *pb.Msg_PrepareVote:
		pea.hotstuff.GetMsgEntrance() <- msg
	case *pb.Msg_NewView:
		pea.hotstuff.GetMsgEntrance() <- msg
	// case *pb.Msg_PbValue:
	// 	pea.acs.GetMsgEntrance() <- msg
	// case *pb.Msg_PbEcho:
	// 	pea.acs.GetMsgEntrance() <- msg
	// case *pb.Msg_PbFinal:
	// 	pea.acs.GetMsgEntrance() <- msg
	// case *pb.Msg_CoinShare:
	// 	pea.acs.GetMsgEntrance() <- msg
	// case *pb.Msg_SpbFinal:
	// 	pea.acs.GetMsgEntrance() <- msg
	// case *pb.Msg_Done:
	// 	pea.acs.GetMsgEntrance() <- msg
	// case *pb.Msg_Halt:
	// 	pea.acs.GetMsgEntrance() <- msg
	// case *pb.Msg_PreVote:
	// 	pea.acs.GetMsgEntrance() <- msg
	// case *pb.Msg_Vote:
	// 	pea.acs.GetMsgEntrance() <- msg
	case *pb.Msg_Timeout:
		pea.pacemaker.handleTimeout(msg)
	default:
		// logger.Warn("Receive unsupported msg")
		// pea.lock.Lock()
		// if pea.acs.start == false {
		// 	logger.Warn("[p_" + strconv.Itoa(int(cc.acs.ID)) + "] [r_" + strconv.Itoa(cc.acs.round) + "] [s_" + strconv.Itoa(cc.sid) + "] [CC] wait common coin start")
		// 	pea.waitAsc.Wait()
		// }
		// pea.lock.Unlock()

		pea.acs.GetMsgEntrance() <- msg
	}
}

func (pea *PeasecodImpl) handleFastRes(res *consensus.FastResult) {
	pea.latestBlock = res
	pea.ProcessProposal(res.Block.Commands)
	pea.TimeChan.HardStartTimer()
	logger.Info("Good work")
}

func (pea *PeasecodImpl) handlePessRes(res []consensus.PessResult) {
	fmt.Println("len(res):", len(res))
	var flag string = "continue"
	for _, v := range res {
		if v.Flag == "stop" {
			flag = v.Flag
		}
	}
	fmt.Println("flag:", flag)
	logger.Info("Good work")
}

func (pea *PeasecodImpl) startPessPath() {
	logger.Info("start ACS in PEA")
	var flag string
	if pea.maxProof == nil || (pea.maxProof != nil && bytes.Equal(pea.maxProof.BlockHash, pea.latestBlock.Proof.BlockHash)) {
		flag = "continue"
	} else {
		flag = "stop"
	}
	pessInput := &consensus.PessResult{
		Txn:   nil,
		Proof: pea.latestBlock.Proof,
		Flag:  flag,
	}
	pea.acs.SetPeaInput(pessInput)
	pea.acs.GetTaskSignal() <- "start"
}

func handleMethod(arg string) string {
	split := strings.Split(arg, ",")
	arg1, _ := strconv.Atoi(split[0])
	arg2, _ := strconv.Atoi(split[1])
	return strconv.Itoa(arg1 + arg2)
}
