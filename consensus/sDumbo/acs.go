package sDumbo

import (
	//"bytes"
	"context"
	// "encoding/hex"
	"encoding/json"
	//"errors"
	//"github.com/golang/protobuf/proto"
	"github.com/niclabs/tcrsa"
	"github.com/sirupsen/logrus"
	//"github.com/syndtr/goleveldb/leveldb"
	go_hotstuff "github.com/wjbbig/go-hotstuff"
	"github.com/wjbbig/go-hotstuff/config"
	"github.com/wjbbig/go-hotstuff/consensus"
	"github.com/wjbbig/go-hotstuff/logging"
	pb "github.com/wjbbig/go-hotstuff/proto"
	//"os"
	"strconv"
	//"sync"
	"time"
	// "fmt"
	
)

var logger = logging.GetLogger()

type CommonSubsetImpl struct {
	consensus.AsynchronousImpl
	
	Sid                int
	proposalHash       []byte         
	cancel             context.CancelFunc
	taskSignal         chan string
	// restart            chan bool
	// taskEndFlag        bool
	// msgEndFlag         bool

	proBroadcast       ProvableBroadcast
	// mvbaInputVectors map[int][]MvbaInputVector
	vectors            []MvbaInputVector
	futureVectorsCache []MvbaInputVector
}

type MvbaInputVector struct{
	id            int
	sid           int
	proposalHash  []byte
	Signature     tcrsa.Signature
}

// sid: session id
func NewCommonSubset(id int) *CommonSubsetImpl {
	logger.Debugf("[ACS] Start Common Subset.")
	ctx, cancel := context.WithCancel(context.Background())
	acs := &CommonSubsetImpl{
		Sid:           0,
		cancel:        cancel,
	}

	msgEntrance := make(chan *pb.Msg)
	taskSignal := make(chan string)
	acs.MsgEntrance = msgEntrance
	acs.taskSignal = taskSignal
	acs.ID = uint32(id)
	logger.WithField("replicaID", id).Debug("[ACS] Init command cache.")

	// read config
	acs.Config = config.HotStuffConfig{}
	acs.Config.ReadConfig()

	privateKey, err := go_hotstuff.ReadThresholdPrivateKeyFromFile(acs.GetSelfInfo().PrivateKey)
	if err != nil {
		logger.Fatal(err)
	}
	acs.Config.PrivateKey = privateKey

	acs.TxnSet = go_hotstuff.NewCmdSet()

	acs.proBroadcast = NewProvableBroadcast(acs)

	acs.futureVectorsCache = make([]MvbaInputVector, 0)

	// acs.controller("start")
	go acs.receiveTaskSignal(ctx)
	go acs.receiveMsg(ctx)

	return acs
}

func (acs *CommonSubsetImpl) startNewInstance() {
	acs.Sid = acs.Sid + 1
	acs.vectors = make([]MvbaInputVector, 0)

	vectors := acs.futureVectorsCache[:]
	for i, v := range vectors {
		if v.sid == acs.Sid{
			acs.vectors = append(acs.vectors, v)
			acs.futureVectorsCache = append(acs.futureVectorsCache[:i], acs.futureVectorsCache[i+1:]...)
		}
	}

	logger.Info("[replica_"+strconv.Itoa(int(acs.ID))+"] [sid_"+strconv.Itoa(int(acs.Sid))+"] startNewInstance")
	var txs []string
	for {
		BatchSize := 2
		txs = acs.TxnSet.GetFirst(BatchSize) //int(ehs.Config.BatchSize)，取前两个元素
		if len(txs) == BatchSize { //如果txs为nil，会报错吗？
			break
		}
		time.Sleep(2000 * time.Millisecond)
	}

	proposal, _ := json.Marshal(txs)
	proposalHash, _ := go_hotstuff.CreateDocumentHash(proposal, acs.Config.PublicKey)
	acs.proposalHash = proposalHash
	go acs.proBroadcast.startProvableBroadcast()
	//acs.proBroadcast = *proBroadcast
}

func (acs *CommonSubsetImpl) receiveTaskSignal(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-acs.taskSignal:
			go acs.controller(task)
		}
	}
}

func (acs *CommonSubsetImpl) controller(task string) {
	switch task {
	case "init":
		go acs.startNewInstance()
	case "end":
		go acs.startNewInstance()
	case "restart":
		//完全结束和重新开始不一样
		go acs.startNewInstance()
	case "ProvableBroadcastFinal":
		go acs.startNewInstance()
	case "SPBFinal":
		//去mvba的控制器
		go acs.startNewInstance()
	case "coinFinal":
		//去mvba的控制器
		go acs.startNewInstance()
	case "voteFinal":
		//去mvba的控制器
		go acs.startNewInstance()
	default:
		logger.Warn("Receive unsupported task signal")
	}
}

func (acs *CommonSubsetImpl) receiveMsg(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-acs.MsgEntrance:
			go acs.handleMsg(msg)
		}
	}
}

func (acs *CommonSubsetImpl) handleMsg(msg *pb.Msg) {
	switch msg.Payload.(type) {
	case *pb.Msg_Request:
		request := msg.GetRequest()
		// logger.WithField("content", request.String()).Debug("[ACS] Get request msg.")
		// put the cmd into the cmdset
		acs.TxnSet.Add(request.Cmd)
		// send the request to the leader, if the replica is not the leader
		break
	case *pb.Msg_PbFinal:
		acs.proBroadcast.handleProvableBroadcastMsg(msg)
	case *pb.Msg_PbValue:
		acs.proBroadcast.handleProvableBroadcastMsg(msg)
	case *pb.Msg_PbEcho:
		acs.proBroadcast.handleProvableBroadcastMsg(msg)
	default:
		logger.Warn("Receive unsupported msg")
	}
}
