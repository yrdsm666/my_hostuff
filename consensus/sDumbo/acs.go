package sDumbo

import (
	"bytes"
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

	// Sid        int
	round      int
	proposal   []byte
	// proposalHash []byte
	cancel     context.CancelFunc
	taskSignal chan string
	taskPhase  string

	// ACS components
	proBroadcast ProvableBroadcast
	mvba         SpeedMvba

	inputVectors []Vector

	valueVectors map[int]map[int][]byte
	msgCache     map[int]map[int][]*pb.Msg
	// startSid     int
}

const (
	PB_PHASE    string = "PB"
	SPB_PHASE_1 string = "SPB_1"
	SPB_PHASE_2 string = "SPB_2"
	CC_PHASE    string = "CC"
	COINSHARE   string = "COIN_SHARE_WITH_SID_"
)

// only variables with uppercase letters can be converted to JSON
type Vector struct {
	Id        int
	Round     int
	Sid       int
	Proposal  []byte
	Signature tcrsa.Signature
}

func NewCommonSubset(id int) *CommonSubsetImpl {
	logger.Debugf("[ACS] Start Common Subset.")
	ctx, cancel := context.WithCancel(context.Background())
	acs := &CommonSubsetImpl{
		round:    0,
		cancel: cancel,
	}

	acs.MsgEntrance = make(chan *pb.Msg)
	acs.taskSignal = make(chan string)
	acs.ID = uint32(id)

	// create txn cache
	acs.TxnSet = go_hotstuff.NewCmdSet()
	logger.WithField("replicaID", id).Debug("[ACS] Init command cache.")

	// read config
	acs.Config = config.HotStuffConfig{}
	acs.Config.ReadConfig()

	// read private key
	privateKey, err := go_hotstuff.ReadThresholdPrivateKeyFromFile(acs.GetSelfInfo().PrivateKey)
	if err != nil {
		logger.Fatal(err)
	}
	acs.Config.PrivateKey = privateKey

	// create ACS components
	acs.proBroadcast = NewProvableBroadcast(acs)
	acs.mvba = NewSpeedMvba(acs)

	// create sDumbo input value cache and msg cache
	// valueVectors: round -> id -> []byte
	acs.valueVectors = make(map[int]map[int][]byte)

	// msgCache: round -> sid -> []*pb.Ms
	acs.msgCache = make(map[int]map[int][]*pb.Msg)

	go acs.receiveTaskSignal(ctx)
	go acs.receiveMsg(ctx)

	// start ACS
	go acs.controller("start")

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
	case *pb.Msg_PbValue:
		pbValueMsg := msg.GetPbValue()
		invokePhase := pbValueMsg.InvokePhase
		if invokePhase == PB_PHASE || acs.taskPhase == PB_PHASE {
			acs.proBroadcast.handleProvableBroadcastMsg(msg)
		} else {
			acs.mvba.handleSpeedMvbaMsg(msg)
		}
	case *pb.Msg_PbEcho:
		if acs.taskPhase == PB_PHASE {
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
		// start new ACS instance
		go acs.startNewInstance()
	case "getPbValue_" + PB_PHASE:
		acs.broadcastPbFinal()
	case "getPbValue_1":
		go acs.mvba.controller(task)
	case "getPbValue_2":
		go acs.mvba.controller(task)
	case "getSpbValue":
		go acs.mvba.controller(task)
	case "getCoin":
		go acs.mvba.controller(task)
	case "pbFinal":
		if acs.taskPhase == PB_PHASE {
			proposal, _ := json.Marshal(acs.inputVectors)
			go acs.mvba.startSpeedMvba(proposal)
		}
	case "end":
		// commit
		fmt.Println("")
		fmt.Println("---------------- [END_1] -----------------")
		vector := acs.mvba.getLeaderVector()
		fmt.Println("副本：", acs.ID)
		fmt.Println("副本round: ", acs.round)
		fmt.Println("leader: ", vector.Id)
		fmt.Println("leaderRound: ", vector.Round)
		fmt.Println("leaderSid: ", vector.Sid)
		res := hex.EncodeToString(vector.Proposal)
		fmt.Println("partProposal: ", res[len(res)-100:len(res)-1])
		// fmt.Println("-----------------------------")
		var resVectors []Vector
		err := json.Unmarshal(vector.Proposal, &resVectors)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal res failed.")
		}
		fmt.Println("resVectorsLen:", len(resVectors))
		// valueVectors := acs.proBroadcast.getValueVectors()
		valueVectors := acs.valueVectors[acs.round]
		var resTxs []string
		// for _, v := range resVectors {
		// 	fmt.Printf("type(v): %T", v)
		// 	fmt.Println("")
		// 	fmt.Println("pnode: ", v.Id)
		// 	fmt.Println("pSid: ", v.Sid)
		// 	fmt.Println("proposalLen: ", len(v.Proposal))
		// 	fmt.Println("sigHashLen: ", len(v.Signature))
		// 	rv := hex.EncodeToString(v.Proposal)
		// 	fmt.Println("proposal: ", rv[len(rv)-100:len(rv)-1])
		// }
		for _, v := range resVectors {
			value, ok := valueVectors[v.Id]
			if ok {
				valueHash, _ := go_hotstuff.CreateDocumentHash(value, acs.Config.PublicKey)
				if !bytes.Equal(valueHash, v.Proposal) {
					logger.WithFields(logrus.Fields{
						"vId":       v.Id,
						"vSid":      v.Sid,
						"vProposal": hex.EncodeToString(v.Proposal),
						"valueHash": hex.EncodeToString(valueHash),
					}).Error("[r_" + strconv.Itoa(int(acs.ID)) + "] [r_" + strconv.Itoa(acs.round) + "] [ACS] The mvba result is not Hash of value proposal!")
					continue
				}
				var txs []string
				err := json.Unmarshal(value, &txs)
				if err != nil {
					logger.WithField("error", err.Error()).Error("Unmarshal value failed.")
				}
				fmt.Println("len(txs):", len(txs))
				resTxs = append(resTxs, txs...)
			} else {
				fmt.Println("len(valueVectors): ", len(valueVectors))
				fmt.Println("loss value from node: ", v.Id)
			}

		}
		fmt.Println("resTxs:", resTxs)
		err = acs.ProcessProposal(resTxs)
		if err != nil {
			logger.WithField("error", err.Error()).Error("echo client failed.")
		}
		fmt.Println(" GOOD WORK!.")
		fmt.Println("---------------- [END_2] -----------------")

		// if acs.round < 10 {
		// 	go acs.startNewInstance()
		// }
	// case "restartWithLeaderProposal":
	// 	fmt.Println("")
	// 	fmt.Println("replica: ", acs.ID)
	// 	fmt.Println("sid: ", acs.Sid)
	// 	fmt.Println("---------------- [NEXT_l_1] -----------------")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                 restart                   -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                 leader                    -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("---------------- [NEXT_l_2] -----------------")
	// 	vector := acs.mvba.getLeaderVector()
	// 	acs.Sid = acs.Sid + 1
	// 	go acs.mvba.startSpeedMvba(vector.Proposal)
	// case "restart":
	// 	fmt.Println("")
	// 	fmt.Println("replica: ", acs.ID)
	// 	fmt.Println("sid: ", acs.Sid)
	// 	fmt.Println("---------------- [NEXT_l_1] -----------------")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                 restart                   -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                  owner                    -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("-                                           -")
	// 	fmt.Println("---------------- [NEXT_l_2] -----------------")
	// 	proposal := acs.mvba.getProposal()
	// 	acs.Sid = acs.Sid + 1
	// 	//signature := acs.mvba.getSignature()
	// 	go acs.mvba.startSpeedMvba(proposal)
	default:
		logger.Warn("Receive unsupported task signal")
	}
}

func (acs *CommonSubsetImpl) startNewInstance() {
	acs.round = acs.round + 1
	acs.taskPhase = PB_PHASE

	// Clear the value cache from old sids
	_, ok := acs.valueVectors[acs.round-1]
	if ok {
		delete(acs.valueVectors, acs.round-1)
	}

	logger.Info("[r_" + strconv.Itoa(int(acs.ID)) + "] [r_" + strconv.Itoa(acs.round) + "] startNewInstance")

	// Init mvba status
	acs.inputVectors = make([]Vector, 0)

	// Handle msg from msg cache
	msgList := acs.getMsgFromCache(acs.round, 0)
	if msgList != nil {
		logger.Warn("[p_" + strId + "] [r_" + strRound + "] [s_" + strSid + "] [MVBA] Handle msg from msg cache")
		for _, msg := range msgList {
			acs.handleMsg(msg)
		}
	}

	// Get txs from TxnSet
	var txs []string
	for {
		BatchSize := 2
		txs = acs.TxnSet.GetFirst(BatchSize) // Get the first BatchSize elements
		if len(txs) == BatchSize {           // 如果txs为nil，会报错吗？
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Start Provable Broadcast with proposal
	proposal, _ := json.Marshal(txs)
	acs.proposal = proposal
	go acs.proBroadcast.startProvableBroadcast(proposal, nil, 0, PB_PHASE, CheckValue)
}

func (acs *CommonSubsetImpl) handlePbFinal(msg *pb.Msg) {
	// Ignore messages if the number of messages is sufficient
	if len(acs.inputVectors) >= 2*acs.Config.F+1 {
		return
	}

	// Parse senderId and senderSid of message
	pbFinal := msg.GetPbFinal()
	senderId := int(pbFinal.Id)
	senderRound := int(pbFinal.Round)

	if senderRound < acs.round {
		// Ignore messages from old sid or round
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderRound":  senderRound,
		}).Warn("[r_" + strconv.Itoa(int(acs.ID)) + "] [r_" + strconv.Itoa(acs.round) + "] [ACS] Get old sid of Pbfinal msg")
		return
	} else if senderRound > acs.round {
		// Save messages from future sid or round
		acs.insertMsg(senderRound, 0, msg)
		logger.WithFields(logrus.Fields{
			"senderId":  senderId,
			"senderRound":  senderRound,
		}).Warn("[r_" + strconv.Itoa(int(acs.ID)) + "] [r_" + strconv.Itoa(acs.round) + "] [ACS] Get future sid of Pbfinal msg and save it")
		return
	}

	logger.WithFields(logrus.Fields{
		"senderId":  senderId,
		"senderRound":  senderRound,
	}).Info("[r_" + strconv.Itoa(int(acs.ID)) + "] [r_" + strconv.Itoa(acs.round) + "] [ACS] Get PbFinal msg")

	// Parse proposal and signature of message
	proposal := pbFinal.Proposal
	signature := &tcrsa.Signature{}
	err := json.Unmarshal(pbFinal.Signature, signature)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
	}

	// Verify the signature
	marshalData := getMsgdata(senderId, senderRound, 0, proposal)
	flag, err := go_hotstuff.TVerify(acs.Config.PublicKey, *signature, marshalData)
	if err != nil || !flag {
		logger.WithField("error", err.Error()).Error("[r_" + strconv.Itoa(int(acs.ID)) + "] [r_" + strconv.Itoa(acs.round) + "] [ACS] verfiy signature from PbFinal failed.")
		return
	}

	// Store the value of message as MVBA's input
	wVector := Vector{
		Id:        senderId,
		Round:     senderRound,
		Proposal:  proposal,
		Signature: *signature,
	}
	acs.inputVectors = append(acs.inputVectors, wVector)

	// Transmit a pbFinal signal to start MVBA
	if len(acs.inputVectors) == 2*acs.Config.F+1 {
		go acs.controller("pbFinal")
	}
}

func (acs *CommonSubsetImpl) broadcastPbFinal() {
	signature := acs.proBroadcast.getSignature()
	marshal, _ := json.Marshal(signature)
	proposalHash := acs.proBroadcast.getProposalHash()
	pbFinalMsg := acs.PbFinalMsg(int(acs.ID), acs.round, proposalHash, marshal)

	// broadcast msg
	err := acs.Broadcast(pbFinalMsg)
	if err != nil {
		logger.WithField("error", err.Error()).Warn("Broadcast pbFinalMsg failed.")
	}
	// send to self
	acs.MsgEntrance <- pbFinalMsg
}

func (acs *CommonSubsetImpl) insertValue(senderRound int, senderId int, senderProposal []byte) {
	_, ok := acs.valueVectors[senderRound]
	if !ok {
		roundValueVectors := make(map[int][]byte)
		acs.valueVectors[senderRound] = roundValueVectors
	}
	_, ok = acs.valueVectors[senderRound][senderId]
	if !ok {
		acs.valueVectors[senderRound][senderId] = senderProposal
	}
}

func (acs *CommonSubsetImpl) insertMsg(senderRound int, senderSid int, msg *pb.Msg) {
	_, ok := acs.msgCache[senderRound]
	if !ok {
		roundMsgCache := make(map[int][]*pb.Msg)
		acs.msgCache[senderRound] = roundMsgCache
	}
	msgList, ok := acs.msgCache[senderRound][senderSid]
	if !ok {
		initMsgList := []*pb.Msg{msg}
		acs.msgCache[senderRound][senderSid] = initMsgList
	} else {
		msgList = append(msgList, msg)
	}
}

func (acs *CommonSubsetImpl) getMsgFromCache(round int, sid int) []*pb.Msg {
	roundMsgCache, ok := acs.msgCache[round]
	if !ok {
		return nil
	}
	msgList, ok := roundMsgCache[sid]
	if !ok {
		return nil
	} else {
		delete(acs.msgCache[round], sid)
		return msgList
	}
}

func CheckValue(id int, round int, sid int, proposal []byte, proof []byte, publicKey *tcrsa.KeyMeta) bool {
	return true
}

func verfiyThld(id int, round int, sid int, proposal []byte, proof []byte, publicKey *tcrsa.KeyMeta) bool {
	// Parse signature
	signature := &tcrsa.Signature{}
	err := json.Unmarshal(proof, signature)
	if err != nil {
		logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
	}

	// Verify the signature
	marshalData := getMsgdata(id, round, sid, proposal)
	documentHash, _ := go_hotstuff.CreateDocumentHash(marshalData, publicKey)
	flag, err := go_hotstuff.TVerify(publicKey, *signature, documentHash)
	if err != nil || !flag {
		logger.WithField("error", err.Error()).Error("verfiy Thld failed.")
		return false
	}
	return true
}

// Combine these fields in []byte format
func getMsgdata(id int, round int, sid int, proposal []byte) []byte {
	type msgData struct {
		Id       int
		Round    int
		Sid      int
		Proposal []byte
	}
	data := &msgData{
		Id:       id,
		Round:    round,
		Sid:      sid,
		Proposal: proposal,
	}
	marshal, _ := json.Marshal(data)
	return marshal
}
