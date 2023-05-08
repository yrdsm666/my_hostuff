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
	"fmt"
	
)

var logger = logging.GetLogger()

type CommonSubsetImpl struct {
	consensus.AsynchronousImpl
	
	Sid                int
	proposalHash       []byte         
	cancel             context.CancelFunc
	restart            chan bool
	taskEndFlag        bool
	msgEndFlag         bool

	proBroadcast       ProvableBroadcast
	// mvbaInputVectors map[int][]MvbaInputVector
	mvbaInputVectors   []MvbaInputVector
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
	restart := make(chan bool)
	acs.MsgEntrance = msgEntrance
	acs.restart =restart
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

	go acs.start(ctx)
	go acs.receiveMsg(ctx)

	acs.restart <- true
	return acs
}

func (acs *CommonSubsetImpl) startNewInstance() {
	acs.Sid = acs.Sid + 1
	acs.mvbaInputVectors = make([]MvbaInputVector, 0)

	vectors := acs.futureVectorsCache[:]
	for i, v := range vectors {
		if v.sid == acs.Sid{
			acs.mvbaInputVectors = append(acs.mvbaInputVectors, v)
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

func (acs *CommonSubsetImpl) start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <- acs.restart:
			acs.taskEndFlag = false
			acs.msgEndFlag = false
			go acs.startNewInstance()
		}
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
		pbFinal := msg.GetPbFinal()
		senderId := int(pbFinal.Id)
		senderSid := int(pbFinal.Sid)
		senderProposalHash := pbFinal.Proposal
		// Ignore messages from old sid
		if senderSid < acs.Sid{
			logger.WithFields(logrus.Fields{
				"senderId":  senderId,
				"senderSid":  senderSid,
			}).Warn("[replica_"+strconv.Itoa(int(acs.ID))+"] [sid_"+strconv.Itoa(acs.Sid)+"] [PB] Get mismatch Pbfinal msg")
			break
		}
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid":  senderSid,
		}).Info("[replica_"+strconv.Itoa(int(acs.ID))+"] [sid_"+strconv.Itoa(int(acs.Sid))+"] [PB] Get PbFinal msg")
		signature := &tcrsa.Signature{}
		err := json.Unmarshal(pbFinal.Signature, signature)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
		}
		marshalData := getMsgdata(senderId, senderSid, senderProposalHash)
		flag, err := go_hotstuff.TVerify(acs.Config.PublicKey, *signature, marshalData)
		if ( err != nil || flag==false ) {
			logger.WithField("error", err.Error()).Error("[replica_"+strconv.Itoa(int(acs.ID))+"] [sid_"+strconv.Itoa(int(acs.Sid))+"] verfiy signature failed.")
		}
		wVector := MvbaInputVector{
			id:            senderId,
			sid:           senderSid,
			proposalHash:  senderProposalHash,
			Signature:     *signature,
		}
		if senderSid > acs.Sid {
			acs.futureVectorsCache = append(acs.mvbaInputVectors, wVector)
		} else {
			acs.mvbaInputVectors = append(acs.mvbaInputVectors, wVector)
		}
		if len(acs.mvbaInputVectors) == 2*acs.Config.F+1 {
			fmt.Println("")
			fmt.Println("---------------- [ACS] -----------------")
			fmt.Println("副本：", acs.ID)
			for i := 0; i < 2*acs.Config.F+1; i++{
				fmt.Println("node: ", acs.mvbaInputVectors[i].id)
				fmt.Println("Sid: ", acs.mvbaInputVectors[i].sid)
				fmt.Println("proposalHashLen: ",len(acs.mvbaInputVectors[i].proposalHash))
			}
			fmt.Println("[ACS] GOOD WORK!.")
			fmt.Println("---------------- [ACS] -----------------")
			// 需要保证旧实例结束再启动新实例
			// 不能同时执行新旧实例，代码不允许
			// 达到新实例的启动条件时，仍然要执行旧实例，不然其他节点可能无法在上一个实例中结束
			// 也就是说，当节点广播完上一个实例的的pbfinal消息后，才可启动新实例
			//go acs.startNewInstance()
			// start a new instance if the conditions are met
			acs.msgEndFlag = true
			if acs.taskEndFlag == true {
				acs.restart <- true
			}
		}
		break
	case *pb.Msg_PbValue:
		acs.proBroadcast.handleProvableBroadcastMsg(msg)
	case *pb.Msg_PbEcho:
		acs.proBroadcast.handleProvableBroadcastMsg(msg)
	default:
		logger.Warn("Receive unsupported msg")
	}
}
