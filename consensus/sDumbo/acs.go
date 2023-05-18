package sDumbo

import (
	// "bytes"
	"context"
	"encoding/hex"
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
	"fmt"
	"time"
)

var logger = logging.GetLogger()

type CommonSubsetImpl struct {
	consensus.AsynchronousImpl

	Sid          int
	proposal     []byte
	proposalHash []byte
	cancel       context.CancelFunc
	taskSignal   chan string
	taskPhase    string
	// restart            chan bool
	// taskEndFlag        bool
	// msgEndFlag         bool

	proBroadcast ProvableBroadcast
	mvba         SpeedMvba
	// vectors map[int][]Vector
	vectors            []Vector
	futureVectorsCache []Vector
}

type Vector struct {
	id        int
	sid       int
	proposal  []byte
	Signature tcrsa.Signature
}

// sid: session id
func NewCommonSubset(id int) *CommonSubsetImpl {
	logger.Debugf("[ACS] Start Common Subset.")
	ctx, cancel := context.WithCancel(context.Background())
	acs := &CommonSubsetImpl{
		Sid:    0,
		cancel: cancel,
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
	acs.mvba = NewSpeedMvba(acs)

	acs.futureVectorsCache = make([]Vector, 0)

	// acs.controller("start")
	go acs.receiveTaskSignal(ctx)
	go acs.receiveMsg(ctx)

	acs.controller("start")

	return acs
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
	case *pb.Msg_PbValue:
		if acs.taskPhase == "PB" {
			acs.proBroadcast.handleProvableBroadcastMsg(msg)
		} else {
			acs.mvba.handleSpeedMvbaMsg(msg)
		}
	case *pb.Msg_PbEcho:
		if acs.taskPhase == "PB" {
			acs.proBroadcast.handleProvableBroadcastMsg(msg)
		} else {
			acs.mvba.handleSpeedMvbaMsg(msg)
		}
	case *pb.Msg_PbFinal:
		acs.handlePbFinal(msg)
	case *pb.Msg_CoinShare:
		acs.mvba.handleSpeedMvbaMsg(msg)
	case *pb.Msg_SpbFinal:
		acs.mvba.handleSpeedMvbaMsg(msg)
	case *pb.Msg_Done:
		acs.mvba.handleSpeedMvbaMsg(msg)
	case *pb.Msg_Halt:
		acs.mvba.handleSpeedMvbaMsg(msg)
	case *pb.Msg_PreVote:
		acs.mvba.handleSpeedMvbaMsg(msg)
	case *pb.Msg_Vote:
		acs.mvba.handleSpeedMvbaMsg(msg)
	default:
		logger.Warn("Receive unsupported msg")
	}
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
	case "start":
		go acs.startNewInstance()
	case "getPbValue":
		if acs.taskPhase == "PB" {
			acs.broadcastPbFinal()
		} else {
			go acs.mvba.controller(task)
		}
	case "getSpbValue":
		go acs.mvba.controller(task)
	case "getCoin":
		go acs.mvba.controller(task)
	case "pbFinal":
		if acs.taskPhase == "PB" {
			go acs.mvba.startSpeedMvba(acs.vectors)
		}
		// fmt.Println("")
		// fmt.Println("---------------- [PB_END_1] -----------------")
		// fmt.Println("replica: ", acs.ID)
		// for i := 0; i < 2*acs.Config.F+1; i++ {
		// 	fmt.Println("node: ", acs.vectors[i].id)
		// 	fmt.Println("Sid: ", acs.vectors[i].sid)
		// 	fmt.Println("proposalHashLen: ", len(acs.vectors[i].proposal))
		// }
		// fmt.Println(" GOOD WORK!.")
		// fmt.Println("---------------- [PB_END_2] -----------------")
	case "end":
		// commit
		fmt.Println("")
		fmt.Println("---------------- [END_1] -----------------")
		vector := acs.mvba.getLeaderVector()
		fmt.Println("副本：", acs.ID)
		fmt.Println("node: ", vector.id)
		fmt.Println("Sid: ", vector.sid)
		fmt.Println("proposal: ", hex.EncodeToString(vector.proposal))
		fmt.Println(" GOOD WORK!.")
		fmt.Println("---------------- [END_2] -----------------")
	case "restartWithLeaderProposal":
		fmt.Println("")
		fmt.Println("---------------- [NEXT_l_1] -----------------")
		vector := acs.mvba.getLeaderVector()
		fmt.Println("副本：", acs.ID)
		fmt.Println("node: ", vector.id)
		fmt.Println("Sid: ", vector.sid)
		fmt.Println("proposal: ", hex.EncodeToString(vector.proposal))
		fmt.Println(" GOOD WORK!.")
		fmt.Println("---------------- [NEXT_l_2] -----------------")
	case "restart":
		fmt.Println("")
		fmt.Println("---------------- [NEXT_i_1] -----------------")
		proposal := acs.mvba.getProposal()
		//signature := acs.mvba.getSignature()
		fmt.Println("副本：", acs.ID)
		fmt.Println("node: ", acs.ID)
		fmt.Println("Sid: ", acs.Sid)
		fmt.Println("proposal: ", hex.EncodeToString(proposal))
		fmt.Println(" GOOD WORK!.")
		fmt.Println("---------------- [NEXT_i_2] -----------------")
	default:
		logger.Warn("Receive unsupported task signal")
	}
}

func (acs *CommonSubsetImpl) startNewInstance() {
	acs.Sid = acs.Sid + 1
	acs.vectors = make([]Vector, 0)

	vectors := acs.futureVectorsCache[:]
	for i, v := range vectors {
		if v.sid == acs.Sid {
			acs.vectors = append(acs.vectors, v)
			acs.futureVectorsCache = append(acs.futureVectorsCache[:i], acs.futureVectorsCache[i+1:]...)
		}
	}

	logger.Info("[replica_" + strconv.Itoa(int(acs.ID)) + "] [sid_" + strconv.Itoa(int(acs.Sid)) + "] startNewInstance")
	var txs []string
	for {
		BatchSize := 2
		txs = acs.TxnSet.GetFirst(BatchSize) //int(ehs.Config.BatchSize)，取前两个元素
		if len(txs) == BatchSize {           //如果txs为nil，会报错吗？
			break
		}
		time.Sleep(2000 * time.Millisecond)
	}

	proposal, _ := json.Marshal(txs)
	acs.proposal = proposal
	proposalHash, _ := go_hotstuff.CreateDocumentHash(proposal, acs.Config.PublicKey)
	acs.proposalHash = proposalHash
	acs.taskPhase = "PB"
	go acs.proBroadcast.startProvableBroadcast(proposalHash, nil, "", CheckValue)
}

func (acs *CommonSubsetImpl) handlePbFinal(msg *pb.Msg) {
	pbFinal := msg.GetPbFinal()
	senderId := int(pbFinal.Id)
	senderSid := int(pbFinal.Sid)
	senderProposal := pbFinal.Proposal
	// Ignore messages from old sid
	if senderSid < acs.Sid {
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderSid": senderSid,
		}).Warn("[replica_" + strconv.Itoa(int(acs.ID)) + "] [sid_" + strconv.Itoa(acs.Sid) + "] [ACS] Get mismatch Pbfinal msg")
		return
	}
	logger.WithFields(logrus.Fields{
		"senderId":  senderId,
		"senderSid": senderSid,
	}).Info("[replica_" + strconv.Itoa(int(acs.ID)) + "] [sid_" + strconv.Itoa(int(acs.Sid)) + "] [ACS] Get PbFinal msg")
	signature := &tcrsa.Signature{}
	err := json.Unmarshal(pbFinal.Signature, signature)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
	}
	marshalData := getMsgdata(senderId, senderSid, senderProposal)
	// documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, acs.Config.PublicKey)
	flag, err := go_hotstuff.TVerify(acs.Config.PublicKey, *signature, marshalData)
	if err != nil || flag == false {
		logger.WithField("error", err.Error()).Error("[replica_" + strconv.Itoa(int(acs.ID)) + "] [sid_" + strconv.Itoa(int(acs.Sid)) + "] [ACS] verfiy signature from PbFinal failed.")
	}
	wVector := Vector{
		id:        senderId,
		sid:       senderSid,
		proposal:  senderProposal,
		Signature: *signature,
	}
	if senderSid > acs.Sid {
		acs.futureVectorsCache = append(acs.vectors, wVector)
	} else {
		acs.vectors = append(acs.vectors, wVector)
	}
	if len(acs.vectors) == 2*acs.Config.F+1 {
		// fmt.Println("")
		// fmt.Println("---------------- [ACS] -----------------")
		// fmt.Println("副本：", acs.ID)
		// for i := 0; i < 2*acs.Config.F+1; i++ {
		// 	fmt.Println("node: ", acs.vectors[i].id)
		// 	fmt.Println("Sid: ", acs.vectors[i].sid)
		// 	fmt.Println("proposalHashLen: ", len(acs.vectors[i].proposal))
		// }
		// fmt.Println("[ACS] GOOD WORK!.")
		// fmt.Println("---------------- [ACS] -----------------")
		// 需要保证旧实例结束再启动新实例
		// 不能同时执行新旧实例，代码不允许
		// 达到新实例的启动条件时，仍然要执行旧实例，不然其他节点可能无法在上一个实例中结束
		// 也就是说，当节点广播完上一个实例的的pbfinal消息后，才可启动新实例
		//go acs.startNewInstance()
		// start a new instance if the conditions are met
		// acs.msgEndFlag = true
		// if acs.taskEndFlag == true {
		// 	acs.restart <- true
		// }
		acs.taskSignal <- "pbFinal"
	}
}

func (acs *CommonSubsetImpl) broadcastPbFinal() {
	signature := acs.proBroadcast.getSignature()
	marshal, _ := json.Marshal(signature)
	proposal := acs.proBroadcast.getProposal()
	pbFinalMsg := acs.PbFinalMsg(int(acs.ID), acs.Sid, proposal, marshal)
	// broadcast msg
	err := acs.Broadcast(pbFinalMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Broadcast failed.")
	}
	// send to self
	acs.MsgEntrance <- pbFinalMsg
}

func CheckValue(id int, sid int, proposal []byte, proof []byte, publicKey *tcrsa.KeyMeta) bool {
	return true
}

func verfiyThld(id int, sid int, proposal []byte, proof []byte, publicKey *tcrsa.KeyMeta) bool {
	signature := &tcrsa.Signature{}
	err := json.Unmarshal(proof, signature)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
	}
	// strj := []byte(string(j))
	// newProposal := append(proposal[:], strj[0])
	marshalData := getMsgdata(id, sid, proposal)
	documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, publicKey)

	flag, err := go_hotstuff.TVerify(publicKey, *signature, documentHash)
	if err != nil || flag == false {
		logger.WithField("error", err.Error()).Error("verfiy Thld failed.")
		return false
	}
	return true

}
