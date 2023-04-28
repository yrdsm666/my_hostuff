package sDumbo

import (
	//"bytes"
	"context"
	//"encoding/hex"
	"encoding/json"
	//"errors"
	//"github.com/golang/protobuf/proto"
	"github.com/niclabs/tcrsa"
	//"github.com/sirupsen/logrus"
	//"github.com/syndtr/goleveldb/leveldb"
	go_hotstuff "github.com/wjbbig/go-hotstuff"
	"github.com/wjbbig/go-hotstuff/config"
	"github.com/wjbbig/go-hotstuff/consensus"
	"github.com/wjbbig/go-hotstuff/logging"
	pb "github.com/wjbbig/go-hotstuff/proto"
	//"os"
	//"strconv"
	//"sync"
	"time"
	"fmt"
	
)

var logger = logging.GetLogger()

type CommonSubsetImpl struct {
	consensus.AsynchronousImpl
	
	Sid              int
	cancel           context.CancelFunc

	proBroadcast     ProvableBroadcast
	mvbaInputVectors []MvbaInputVector
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
		Sid:           1,
		cancel:        cancel,
	}

	msgEntrance := make(chan *pb.Msg)
	acs.MsgEntrance = msgEntrance
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

	go acs.startNewInstance()
	go acs.receiveMsg(ctx)
	return acs
}

func (acs *CommonSubsetImpl) startNewInstance() {
	acs.mvbaInputVectors = make([]MvbaInputVector, 0)
	acs.Sid = acs.Sid + 1
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
	proBroadcast := NewProvableBroadcast(int(acs.ID), acs.Sid, proposalHash, acs.Config)
	acs.proBroadcast = *proBroadcast
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
		//logger.WithField("content", request.String()).Debug("[ACS] Get request msg.")
		// put the cmd into the cmdset
		acs.TxnSet.Add(request.Cmd)
		// send the request to the leader, if the replica is not the leader
		break
	case *pb.Msg_PbFinal:
		pbFinal := msg.GetPbFinal()
		senderId := int(pbFinal.Id)
		senderSid := int(pbFinal.Sid)
		senderProposal := pbFinal.Proposal
		marshalData := getMsgdata(senderId, senderSid, senderProposal)
		signature := tcrsa.Signature{}
		err := json.Unmarshal(pbFinal.Signature, signature)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Unmarshal signature failed.")
		}
		flag, err := go_hotstuff.TVerify(acs.Config.PublicKey, marshalData, signature)
		if ( err != nil || flag==false ) {
			logger.WithField("error", err.Error()).Error("verfiy signature failed.")
		}
		dataHash, err := go_hotstuff.CreateDocumentHash(marshalData, acs.Config.PublicKey)
		if err != nil {
			logger.WithField("error", err.Error()).Error("Create Document Hash failed.")
		}
		wVector := MvbaInputVector{
			id:            senderId,
			sid:           senderSid,
			proposalHash:  dataHash,
			Signature:     signature,
		}
		acs.mvbaInputVectors = append(acs.mvbaInputVectors, wVector)
		if len(acs.mvbaInputVectors) == 2*acs.Config.F+1 {
			fmt.Println("---------------- [ACS] -----------------")
			fmt.Println("副本：%d", acs.ID)
			for i := 0; i < 2*acs.Config.F+1; i++{
				fmt.Println(acs.mvbaInputVectors[i].proposalHash)
			}
			fmt.Println("[ACS] GOOD WORK!.")
			fmt.Println("---------------- [ACS] -----------------")
			go acs.startNewInstance()
		}
		break
	case *pb.Msg_PbValue:
		fmt.Println("---------------- [config_0] -----------------")
		fmt.Println(acs.Config.PublicKey)
		fmt.Println("---------------- [config_1] -----------------")
		fmt.Println(acs.proBroadcast.Config.PublicKey)
		fmt.Println("---------------- [config_2] -----------------")
		acs.proBroadcast.handleProvableBroadcastMsg(msg)
	case *pb.Msg_PbEcho:
		acs.proBroadcast.handleProvableBroadcastMsg(msg)
	default:
		logger.Warn("Receive unsupported msg")
	}
}
