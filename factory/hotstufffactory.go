package factory

import (
	"strconv"
	"strings"

	"github.com/wjbbig/go-hotstuff/consensus"
	"github.com/wjbbig/go-hotstuff/consensus/basic"
	"github.com/wjbbig/go-hotstuff/consensus/chained"
	"github.com/wjbbig/go-hotstuff/consensus/eventdriven"
	"github.com/wjbbig/go-hotstuff/consensus/peasecod"
	"github.com/wjbbig/go-hotstuff/consensus/sDumbo"
)

func HotStuffFactory(networkType string, id int) consensus.HotStuff {
	switch networkType {
	case "basic":
		return basic.NewBasicHotStuff(id, handleMethod)
	case "chained":
		return chained.NewChainedHotStuff(id, handleMethod)
	case "event-driven":
		return eventdriven.NewEventDrivenHotStuff(id, handleMethod, nil, nil)
	}
	return nil
}

func ACSFactory(networkType string, id int) consensus.Asynchronous {
	return sDumbo.NewCommonSubset(id, nil, nil)
}

func PeaFactory(networkType string, id int) consensus.Parallel {
	return peasecod.NewPeasecod(id)
}

func handleMethod(arg string) string {
	split := strings.Split(arg, ",")
	arg1, _ := strconv.Atoi(split[0])
	arg2, _ := strconv.Atoi(split[1])
	return strconv.Itoa(arg1 + arg2)
}
