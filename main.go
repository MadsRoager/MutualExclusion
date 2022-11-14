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
	"google.golang.org/grpc/credentials/insecure"
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
	ownPort := int32(arg1)

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

	// Dialing peers
	n := 2
	for n < len(os.Args) {
		arg, _ := strconv.ParseInt(os.Args[n], 10, 32)
		port := int32(arg)

		if port == ownPort {
			continue
		}

		var conn *grpc.ClientConn
		log.Printf("Trying to dial: %v\n", port)
		fmt.Printf("Trying to dial: %v\n", port)
		conn, err := grpc.Dial(fmt.Sprintf(":%v", port), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("Could not connect: %s", err)
		}
		defer conn.Close()
		c := request.NewRequestClient(conn)
		p.clients[port] = c
		n++
	}

	waitingForSignToEnterCriticalSection(p)
}

func waitingForSignToEnterCriticalSection(p *peer) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		log.Printf("Peer with id %d wants to enter critical section", p.id)
		fmt.Printf("Peer with id %d wants to enter critical section\n", p.id)
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
	fmt.Printf("Peer with id %d changed its state to %s\n", p.id, state)
}

func (p *peer) sendRequestToAll() {
	p.updateTimestamp(p.lamportTimestamp, &m)
	lastSentLamportTimestamp = p.lamportTimestamp
	request := &request.Priority{Id: p.id, LamportTimestamp: p.lamportTimestamp} // TODO
	for id, client := range p.clients {
		WaitGroup.Add(1)
		go p.requestAndWaitForReply(client, request, id)
	}
	WaitGroup.Wait()

}

func (p *peer) requestAndWaitForReply(client request.RequestClient, request *request.Priority, id int32) {
	_, err := client.Request(p.ctx, request)
	if err != nil {
		log.Fatalln("Something went wrong")
	}
	defer WaitGroup.Done()
}

func (p *peer) enterCriticalSection() {
	log.Printf("Peer with id %d entered the critical section", p.id)
	fmt.Printf("Peer with id %d entered the critical section\n", p.id)
	time.Sleep(6 * time.Second)
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

func (p *peer) Request(ctx context.Context, req *request.Priority) (*request.Reply, error) {
	p.updateTimestamp(req.LamportTimestamp, &m)
	// waiting to reply until its state is RELEASED
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