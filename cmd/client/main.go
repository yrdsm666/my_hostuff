package main

import (
	"context"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	go_hotstuff "github.com/wjbbig/go-hotstuff"
	"github.com/wjbbig/go-hotstuff/config"
	"github.com/wjbbig/go-hotstuff/logging"
	pb "github.com/wjbbig/go-hotstuff/proto"
	"google.golang.org/grpc"
)

var logger = logging.GetLogger()

const timeoutDuration = time.Second * 2

type command string

type HotStuffClient struct {
	consensusResult        map[command]reply
	hotStuffConfig         config.HotStuffConfig
	requestTimeout         *go_hotstuff.Timer
	requestTimeoutDuration time.Duration
	replyChan              chan *pb.Msg
	cancelFunc             context.CancelFunc
	mut                    sync.Mutex
}

type reply struct {
	result string
	count  int
}

func NewHotStuffClient() *HotStuffClient {
	client := &HotStuffClient{
		consensusResult:        make(map[command]reply),
		hotStuffConfig:         *config.NewHotStuffConfig(),
		requestTimeoutDuration: timeoutDuration,
		requestTimeout:         go_hotstuff.NewTimer(timeoutDuration),
		replyChan:              make(chan *pb.Msg),
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	client.cancelFunc = cancelFunc
	client.requestTimeout.Init()
	client.requestTimeout.Stop()
	go client.receiveReply(ctx)
	return client
}

func (client *HotStuffClient) getResults(cmd command) (re reply, b bool) {
	client.mut.Lock()
	defer client.mut.Unlock()
	re, b = client.consensusResult[cmd]
	return
}

func (client *HotStuffClient) setResult(cmd command, re reply) {
	client.mut.Lock()
	defer client.mut.Unlock()
	client.consensusResult[cmd] = re
}

func (client *HotStuffClient) receiveReply(ctx context.Context) {
	//如果client.replyChans有值则进行处理
	for {
		select {
		case msg := <-client.replyChan:
			replyMsg := msg.GetReply()
			cmd := command(replyMsg.Command)
			if re, ok := client.getResults(cmd); ok {
				if re.result == replyMsg.Result {
					re.count++
					//获得f+1节点的reply
					if re.count == client.hotStuffConfig.F+1 {
						logger.WithFields(logrus.Fields{
							"cmd":    cmd,
							"result": re.result,
						}).Info("Consensus success.")
					}
					client.setResult(cmd, re)
				}
			} else {
				re := reply{
					result: replyMsg.Result,
					count:  1,
				}
				client.setResult(cmd, re)
			}
		case <-ctx.Done():
			return
		}
	}
}

func startSend(index int, stuffClient *HotStuffClient){
	conn, err := grpc.Dial(stuffClient.hotStuffConfig.Cluster[index].Address, grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	client := pb.NewHotStuffServiceClient(conn) //grpc的客户端
	rand.Seed(time.Now().UnixNano())
	for {
		time.Sleep(time.Millisecond * 200)
		cmd := strconv.Itoa(rand.Intn(100)) + "," + strconv.Itoa(rand.Intn(100))
		//logger.WithField("content", cmd).Info("[CLIENT] Send request")
		_, err = client.SendRequest(context.Background(), &pb.Msg{Payload: &pb.Msg_Request{Request: &pb.Request{
			Cmd:           cmd,
			ClientAddress: "localhost:9999",
		}}})
	}
}

func main() {
	stuffClient := NewHotStuffClient()
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM,
		syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)
		

	// use goroutine send msg
	go func() {
		for i := 0; i < stuffClient.hotStuffConfig.N; i++{
			go startSend(i, stuffClient)
		} 
	}()

	// start client server
	clientServer := grpc.NewServer()
	hotStuffGRPCClient := &hotStuffGRPCClient{stuffClient}
	pb.RegisterHotStuffServiceServer(clientServer, hotStuffGRPCClient)
	listen, err := net.Listen("tcp", "localhost:9999")
	if err != nil {
		panic(err)
	}
	go func() {
		<-c
		// get signal, exit
		logger.Info("[CLIENT] Client exit...")
		stuffClient.cancelFunc() //释放结束信号来结束stuffClient
		os.Exit(1)
	}()
	clientServer.Serve(listen)

}
