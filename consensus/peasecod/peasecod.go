package peasecod

import (
	"context"

	//"errors"
	//"github.com/golang/protobuf/proto"

	//"github.com/syndtr/goleveldb/leveldb"

	go_hotstuff "github.com/wjbbig/go-hotstuff"
	"github.com/wjbbig/go-hotstuff/config"
	"github.com/wjbbig/go-hotstuff/consensus"
	"github.com/wjbbig/go-hotstuff/consensus/eventdriven"
	"github.com/wjbbig/go-hotstuff/logging"
	pb "github.com/wjbbig/go-hotstuff/proto"

	// "github.com/wjbbig/go-hotstuff/consensus/sDumbo"
	//"os"
	"strconv"
	"strings"
	//"sync"
	// "fmt"
	// "time"
)

var logger = logging.GetLogger()

type PeasecodImpl struct {
	consensus.ParallelImpl

	epoch    int
	proposal []byte

	hotstuff consensus.HotStuff

	cancel context.CancelFunc
}

func NewPeasecod(id int) *PeasecodImpl {
	logger.Debugf("[PEA] Start Peasecod.")
	ctx, cancel := context.WithCancel(context.Background())
	pea := &PeasecodImpl{
		epoch:  0,
		cancel: cancel,
	}

	pea.MsgEntrance = make(chan *pb.Msg)
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

	pea.hotstuff = eventdriven.NewEventDrivenHotStuff(id, handleMethod)

	go pea.receiveMsg(ctx)

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
	// case *pb.Msg_PbEcho:

	// case *pb.Msg_PbFinal:

	// case *pb.Msg_CoinShare:

	// case *pb.Msg_SpbFinal:

	// case *pb.Msg_Done:

	// case *pb.Msg_Halt:

	// case *pb.Msg_PreVote:

	// case *pb.Msg_Vote:

	default:
		logger.Warn("Receive unsupported msg")
	}
}

func handleMethod(arg string) string {
	split := strings.Split(arg, ",")
	arg1, _ := strconv.Atoi(split[0])
	arg2, _ := strconv.Atoi(split[1])
	return strconv.Itoa(arg1 + arg2)
}
