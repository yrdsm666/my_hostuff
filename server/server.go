package main

import (
	"flag"
	"github.com/wjbbig/go-hotstuff/consensus"
	"github.com/wjbbig/go-hotstuff/factory"
	"github.com/wjbbig/go-hotstuff/logging"
	"github.com/wjbbig/go-hotstuff/proto"
	"github.com/wjbbig/go-hotstuff/config"
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
	flag.StringVar(&networkType, "type", "", "which type of network you want to create.  basic/chained/event-driven/acs")
}

func start(networkType string, id int) {
	// create grpc server
	rpcServer := grpc.NewServer()

	if networkType=="acs" {
		hotStuffService := consensus.NewAsynchronousService(factory.ACSFactory(networkType, id))
		// register service
		proto.RegisterHotStuffServiceServer(rpcServer, hotStuffService)
		// get node port
		info := hotStuffService.GetImpl().GetSelfInfo()
		port := info.Address[strings.Index(info.Address, ":"):]
		logger.Infof("[CONSENSUS] Server type: %v", networkType)
		logger.Infof("[CONSENSUS] Server start at port%s", port)
		// listen the port
		listen, err := net.Listen("tcp", port)
		if err != nil {
			panic(err)
		}
		// close goroutine,db connection and delete db file safe when exiting
		go func() {
			<-sigChan
			logger.Info("[CONSENSUS] Exit...")
			hotStuffService.GetImpl().SafeExit()
			wg.Done()
			os.Exit(0)
		}()
		// start server
		rpcServer.Serve(listen)	
	} else{
		hotStuffService := consensus.NewHotStuffService(factory.HotStuffFactory(networkType, id))
		// register service
		proto.RegisterHotStuffServiceServer(rpcServer, hotStuffService)
		// get node port
		info := hotStuffService.GetImpl().GetSelfInfo()
		port := info.Address[strings.Index(info.Address, ":"):]
		logger.Infof("[CONSENSUS] Server type: %v", networkType)
		logger.Infof("[CONSENSUS] Server start at port%s", port)
		// listen the port
		listen, err := net.Listen("tcp", port)
		if err != nil {
			panic(err)
		}
		// close goroutine,db connection and delete db file safe when exiting
		go func() {
			<-sigChan
			logger.Info("[CONSENSUS] Exit...")
			hotStuffService.GetImpl().SafeExit()
			wg.Done()
			os.Exit(0)
		}()
		// start server
		rpcServer.Serve(listen)
	}
}

var wg sync.WaitGroup

func main(){
	flag.Parse()
	
	config := config.NewHotStuffConfig()

	if networkType==""{
		networkType = config.NetworkType
	}

	logger.Infof("[CONSENSUS] START!")
	for i:=1; i<config.N; i++{
		wg.Add(1)
		go start(networkType, i)
	}
	wg.Wait()
	logger.Infof("[CONSENSUS] END!")
}
