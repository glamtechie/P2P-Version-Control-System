package zing

import (
	"sync"
	"os"
	"fmt"
	"net"
	"net/rpc"
	"net/http"
)


type Server struct {
	// my index number
	id	 int

	// my ip address
	address  string

	// the prepare message queue
	preQueue []Version

	// the lock for changing prepare message queue
	lock	 *sync.Mutex

	// ready to serve or not
	ready	 bool
}

// global variable
var (
	GlobalBuffer []Push
	IndexList    []int
)


func InitializeServer(port string) *Server {
	server := Server{}
	if _, err := os.Stat(METADATA_FILE); os.IsNotExist(err) {
		panic("initialize the repository first")
	} else {
		server.id = getOwnIndex()
	}
	addrs, _ := net.InterfaceAddrs()
    for _, address := range addrs {
        if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
            if ipnet.IP.To4() != nil {
            	server.address = ipnet.IP.String() + port
            	break
            }
        }
    }

	server.preQueue = make([]Version, 0)
	server.lock     = &sync.Mutex{}
	server.ready    = false
	return &server
}

func StartServer(instance *Server) error {
	client := InitializeClient()
	if client.id == -1 {
		panic("initialize the repository first")
	}
	server := rpc.NewServer()
	server.Register(instance)

	l, e := net.Listen("tcp", instance.address)
	if e != nil {
		return e
	}

	go client.comeAlive()
	return http.Serve(l, server)
}


/*
 RPC function: ReceivePrepare
*/
func (self *Server) ReceivePrepare(prepare *Version, succ *bool) error {
	self.lock.Lock()
	defer self.lock.Unlock()

	fmt.Printf("Receive the prepare, Node: %d, Version: %d\n", prepare.NodeIndex, prepare.VersionIndex)

	// the server is not ready
	if !self.ready {
		return fmt.Errorf("Server not ready")
	}
	// it is a dummy prepare message
	if prepare.NodeAddress == INVALIDIP {
		return nil
	}

	if len(self.preQueue) == 0 {
		*succ = true
	} else {
		*succ = false
	}
	self.preQueue = append(self.preQueue, *prepare)
	return nil
}

func processChanges(push Push, index int) []Push {
	insertPoint := -1
	for key, value := range IndexList {
		if index < value {
			insertPoint = key
			break
		}
	}

	if insertPoint == -1 {
		insertPoint = len(IndexList)
	}
	IndexList    = append(IndexList[:insertPoint],    append([]int{index}, IndexList[insertPoint:]...)...)
	GlobalBuffer = append(GlobalBuffer[:insertPoint], append([]Push{push}, GlobalBuffer[insertPoint:]...)...)

	if IndexList[0] == 0 {
		cutPoint := 1
		for i := 1; i < len(IndexList); i++ {
			if IndexList[i] - IndexList[i - 1] != 1 {
				cutPoint = i
				break
			}
		}
		results := GlobalBuffer[:cutPoint]

		IndexList    = IndexList[cutPoint:]
		GlobalBuffer = GlobalBuffer[cutPoint:]
		return results
	} else {
		return make([]Push, 0)
	}
}


func commitChanges(pushes []Push, id int) error {
	// commit the pushes to the file system
	for _, push := range pushes {
		if push.Change.NodeIndex == -1 && push.Change.VersionIndex == -1 {
			// this is a new node join.
			ipList := getAddressList()
			ipList = append(ipList, push.Change.NodeAddress)
			setAddressList(ipList)
			continue
		}
		if len(push.Patch) == 0 {
			// this is an abort message or an come alive message.
			continue
		}

		var err error
		if push.Change.NodeIndex == id {
			err = zing_process_push_at_src("patch", push.Patch)
		} else {
			err = zing_process_push("patch", push.Patch)
		}

		if err != nil {
			panic("commit change error")
		}

		// write the push to the log
		writeLog(push)
	}
	return nil
}

/*
 RPC function: ReceivePush
*/
func (self *Server) ReceivePush(push *Push, succ *bool) error {
	var index int = -1
	var pushes []Push

	fmt.Printf("Receive the Push from Node: %d, Version: %d\n", push.Change.NodeIndex, push.Change.VersionIndex)
	fmt.Printf("Patch length: %d\n", len(push.Patch))

	self.lock.Lock()
	defer self.lock.Unlock()

	for i, prepare := range self.preQueue {
		if VersionEquality(prepare, push.Change) {
			index = i
			break
		}
	}
	if index == -1 {
		panic("No match prepare message")
	} else {
		pushes = processChanges(*push, index)
	}

	// commit the changes
	commitChanges(pushes, self.id)
	if len(pushes) > 0 {
		self.preQueue = self.preQueue[len(pushes):]
	}
	*succ = true
	return nil
}

/*
 RPC function: AsynchronousPush
*/
func (self *Server) AsynchronousPush(bundle *Asynchronous, succ *bool) error {
	// this function should only called by its own client
	if bundle.Object.server != self.address {
		panic("In AsynchronousPush: Not come from myself")
	}

	var group sync.WaitGroup
	// send push changes from last to first
	for i := len(bundle.Object.addressList) - 1; i >= 0; i-- {
		if bundle.LiveMap[i] {
			address := bundle.Object.addressList[i]
			succeed := false
			group.Add(1)
			go SendPush(address, bundle.Message, &succeed, &group)
		}
	}

	// here we don't wait the go routine to finish.
	*succ = true
	return nil
}

/*
 RPC function: ReceiveReady
*/
func (self *Server) ReceiveReady(address string, succ *bool) error {
	// this function should only called by its own client
	if address != self.address {
		panic("In receive ready: Not come from myself")
	}

	self.lock.Lock()
	defer self.lock.Unlock()

	self.ready = true
	*succ = true
	return nil
}

func (self *Server) ReturnAddressList(argList []string, resList *[]string) error {
	list := getAddressList()
	if len(list) < len(argList) {
		setAddressList(argList)
		*resList = argList
	} else {
		*resList = list
	}

	return nil
}

func (self *Server) PrepareQueueCheck(address string, result *bool) error {
	if address != self.address {
		panic("In check preQueue: Not come from my self")
	}

	self.lock.Lock()
	defer self.lock.Unlock()

	if len(self.preQueue) != 0 {
		*result = false
	} else {
		*result = true
	}
	return nil
}

func (self *Server) ReturnMissingData(ver Version, pushes *[]Push) error {
	if len(*pushes) > 0 {
		commitChanges(*pushes, self.id)
		return nil
	}

	tmpList  := getPushDiff(ver)
	localVer := getLastVer()
	fmt.Println("Missing data request: local version", localVer, "remote version", ver)

	if len(tmpList) == 0 && !VersionEquality(ver, localVer) {
		// I surpass the sender.
		tmpList = make([]Push, 1)
		tmpList[0].Change = localVer
		tmpList[0].Patch  = make([]byte, 0)
		*pushes = tmpList
	} else {
		*pushes = tmpList
	}
	return nil
}



