package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	request "github.com/MadsRoager/MutualExclusion/grpc"
	"google.golang.org/grpc"
)

var WaitGroup sync.WaitGroup
var m sync.Mutex
var lastSentLamportTimestamp int32

type peer struct {
	request.UnimplementedRequestServer
	id               int32
	amountOfPings    map[int32]int32
	clients          map[int32]request.RequestClient
	ctx              context.Context
	lamportTimestamp int32
	state            string
}

func main() {
	logfile, err := os.OpenFile("logfile", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	log.SetOutput(logfile)
	log.SetFlags(2)
	arg1, _ := strconv.ParseInt(os.Args[1], 10, 32)
	ownPort := int32(arg1) + 8000

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &peer{
		id:               ownPort,
		amountOfPings:    make(map[int32]int32),
		clients:          make(map[int32]request.RequestClient),
		ctx:              ctx,
		lamportTimestamp: 0,
		state:            "RELEASED",
	}

	// Create listener tcp on port ownPort
	list, err := net.Listen("tcp", fmt.Sprintf(":%v", ownPort))
	if err != nil {
		log.Fatalf("Failed to listen on port: %v", err)
	}
	grpcServer := grpc.NewServer()
	request.RegisterRequestServer(grpcServer, p)

	go func() {
		if err := grpcServer.Serve(list); err != nil {
			log.Fatalf("Failed to server %v", err)
		}
	}()

	for i := 0; i < 3; i++ {
		port := int32(8000) + int32(i)

		if port == ownPort {
			continue
		}

		var conn *grpc.ClientConn
		log.Printf("Trying to dial: %v\n", port)
		conn, err := grpc.Dial(fmt.Sprintf(":%v", port), grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			log.Fatalf("Could not connect: %s", err)
		}
		defer conn.Close()
		c := request.NewRequestClient(conn)
		p.clients[port] = c
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		log.Printf("%d wants to enter critical section", p.id)
		fmt.Printf("%d wants to enter critical section", p.id)
		p.wantToEnterCriticalSection()
	}
}

func (p *peer) wantToEnterCriticalSection() {
	p.changeState("WANTED")
	p.sendRequestToAll()
	p.changeState("HELD")
	p.enterCriticalSection()
	p.changeState("RELEASED")
	// it replies to queued requests implicitly
}

func (p *peer) changeState(state string) {
	p.state = state
	log.Printf("Peer with id %d changed its state to %s", p.id, state)
}

func (p *peer) enterCriticalSection() {
	log.Printf("Peer with id %d entered the critical section", p.id)
	fmt.Printf("Peer with id %d entered the critical section", p.id)
	time.Sleep(6 * time.Second)
}

func (p *peer) Request(ctx context.Context, req *request.Priority) (*request.Reply, error) {
	p.updateTimestamp(req.LamportTimestamp, &m)
	for p.state == "HELD" || p.state == "WANTED" && p.isFirstPriority(req) {
		// queue request
		time.Sleep(1 * time.Second)
	}
	rep := &request.Reply{Amount: 1}
	return rep, nil

}

func (p *peer) isFirstPriority(req *request.Priority) bool {
	if req.LamportTimestamp > lastSentLamportTimestamp {
		return true
	}
	return req.Id > p.id
}

func (p *peer) sendRequestToAll() {
	p.updateTimestamp(p.lamportTimestamp, &m)
	lastSentLamportTimestamp = p.lamportTimestamp
	request := &request.Priority{Id: p.id, LamportTimestamp: p.lamportTimestamp} // TODO
	for id, client := range p.clients {
		WaitGroup.Add(1)
		go p.RequestAndWaitForReply(client, request, id)
	}
	WaitGroup.Wait()

}

func (p *peer) RequestAndWaitForReply(client request.RequestClient, request *request.Priority, id int32) {
	_, err := client.Request(p.ctx, request)
	if err != nil {
		log.Fatalln("Something went wrong")
	}
	defer WaitGroup.Done()

}

func (p *peer) updateTimestamp(newTimestamp int32, m *sync.Mutex) {
	m.Lock()
	p.lamportTimestamp = maxValue(int32(p.lamportTimestamp), newTimestamp)
	p.lamportTimestamp++
	m.Unlock()
}

func maxValue(new int32, old int32) int32 {
	if new < old {
		return old
	}
	return new
}
