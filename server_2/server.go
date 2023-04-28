package main

import (
	//"flag"
	"github.com/wjbbig/go-hotstuff/consensus"
	"github.com/wjbbig/go-hotstuff/factory"
	"github.com/wjbbig/go-hotstuff/logging"
	"github.com/wjbbig/go-hotstuff/proto"
	"google.golang.org/grpc"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"sync"
)

var (
	id          int
	networkType string
	logger      = logging.GetLogger()
	sigChan     chan os.Signal
)

func init() {
	sigChan = make(chan os.Signal)
	signal.Notify(sigChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM,
		syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGUSR2)
}

func start(networkType string, id int) {
	// create grpc server
	rpcServer := grpc.NewServer()

	//hotStuffService := consensus.NewHotStuffService(factory.HotStuffFactory(networkType, id))
	hotStuffService := consensus.NewAsynchronousService(factory.ACSFactory(networkType, id))
	// register service
	proto.RegisterHotStuffServiceServer(rpcServer, hotStuffService)
	// get node port
	info := hotStuffService.GetImpl().GetSelfInfo()
	port := info.Address[strings.Index(info.Address, ":"):]
	logger.Infof("[HOTSTUFF] Server type: %v", networkType)
	logger.Infof("[HOTSTUFF] Server start at port%s", port)
	// listen the port
	listen, err := net.Listen("tcp", port)
	if err != nil {
		panic(err)
	}
	// close goroutine,db connection and delete db file safe when exiting
	go func() {
		<-sigChan
		logger.Info("[HOTSTUFF] Exit...")
		hotStuffService.GetImpl().SafeExit()
		wg.Done()
		os.Exit(1)
	}()
	// start server
	rpcServer.Serve(listen)
}

var wg sync.WaitGroup

func main(){
	logger.Infof("[ACS] START!")
	for i:=1;i<5;i++{
		wg.Add(1)
		go start("acs", i)
	}
	wg.Wait()
	logger.Infof("[ACS] END!")
}
