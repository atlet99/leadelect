package node

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	pb "github.com/NovikovRoman/leadelect/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type Node struct {
	sync.RWMutex
	id     string
	addr   string
	port   int
	status pb.NodeStatus

	round int64 // Раунд голосования
	votes int   // Голоса от нод
	voted bool  // Проголосовал

	nodes map[string]*Node // другие ноды

	heartBeatTime time.Time

	heartBeatTimeout     time.Duration
	checkElectionTimeout time.Duration

	grpcClient    *client
	clientTimeout time.Duration
	grpcServer    *grpc.Server
}

type NodeOpt func(n *Node)

func ClientTimeout(timeout time.Duration) NodeOpt {
	return func(n *Node) {
		n.clientTimeout = timeout
	}
}

func HeartBeatTimeout(timeout time.Duration) NodeOpt {
	return func(n *Node) {
		n.heartBeatTimeout = timeout
	}
}

func CheckElectionTimeout(timeout time.Duration) NodeOpt {
	return func(n *Node) {
		n.checkElectionTimeout = timeout
	}
}

func New(id, addr string, port int, opts ...NodeOpt) (n *Node) {
	n = &Node{
		id:                   id,
		addr:                 addr,
		port:                 port,
		nodes:                make(map[string]*Node),
		status:               pb.NodeStatus_Follower,
		clientTimeout:        time.Second * 10,
		heartBeatTimeout:     time.Second * 3,
		checkElectionTimeout: time.Second * 10,
	}

	for _, opt := range opts {
		opt(n)
	}

	n.grpcClient = newClient(n, []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	})
	return
}

func (n *Node) ClientTLS(caFile, serverHostOverride string) (err error) {
	caFile = asbPath(caFile)
	var creds credentials.TransportCredentials
	if creds, err = credentials.NewClientTLSFromFile(caFile, serverHostOverride); err != nil {
		return
	}
	n.grpcClient = newClient(n, []grpc.DialOption{grpc.WithTransportCredentials(creds)})
	return
}

func (n *Node) ServerTLS(certFile, keyFile string) (err error) {
	var creds credentials.TransportCredentials
	creds, err = credentials.NewServerTLSFromFile(asbPath(certFile), asbPath(keyFile))
	if err != nil {
		return
	}
	n.grpcServer = grpc.NewServer(grpc.Creds(creds))
	return
}

func (n *Node) ID() string {
	return n.id
}

func (n *Node) Addr() string {
	return n.addr
}

func (n *Node) Port() int {
	return n.port
}

func (n *Node) AddrPort() string {
	return fmt.Sprintf("%s:%d", n.addr, n.port)
}

func (n *Node) AddNode(node *Node) {
	n.Lock()
	n.nodes[node.id] = node
	n.Unlock()
}

func (n *Node) Nodes() map[string]*Node {
	n.RLock()
	defer n.RUnlock()
	return n.nodes
}

func (n *Node) NumNodes() int {
	n.RLock()
	defer n.RUnlock()
	return len(n.nodes)
}

func (n *Node) Status() pb.NodeStatus {
	n.RLock()
	defer n.RUnlock()
	return n.status
}

func (n *Node) addVote() {
	n.Lock()
	n.votes += 1
	n.Unlock()
}

func (n *Node) numVotes() int {
	n.RLock()
	defer n.RUnlock()
	return n.votes
}

func (n *Node) resetVotes() {
	n.Lock()
	n.votes = 0
	n.Unlock()
}

func (n *Node) isVoted() bool {
	n.RLock()
	defer n.RUnlock()
	return n.voted
}

func (n *Node) setVoted() {
	n.Lock()
	n.voted = true
	n.Unlock()
}

func (n *Node) resetVoted() {
	n.Lock()
	n.voted = false
	n.Unlock()
}

func (n *Node) getRound() int64 {
	n.RLock()
	defer n.RUnlock()
	return n.round
}

func (n *Node) setRound(round int64) {
	n.Lock()
	n.round = round
	n.Unlock()
}

func (n *Node) addRound() {
	n.Lock()
	n.round += 1
	n.Unlock()
}

func (n *Node) agreeVote(round int64) bool {
	n.Lock()
	defer n.Unlock()

	if n.status != pb.NodeStatus_Follower && n.round >= round || n.voted {
		return false
	}

	n.voted = true
	n.votes = 0
	n.status = pb.NodeStatus_Follower // снять свою кандидатуру
	return true
}

func (n *Node) heartBeatExpired() bool {
	n.RLock()
	defer n.RUnlock()
	return time.Since(n.heartBeatTime) > time.Second*5
}

func (n *Node) setHeartBeat() {
	n.Lock()
	n.heartBeatTime = time.Now()
	n.Unlock()
}

func (n *Node) setStatus(status pb.NodeStatus) {
	n.Lock()
	n.status = status
	n.Unlock()
}

func (n *Node) getStatus() pb.NodeStatus {
	n.RLock()
	defer n.RUnlock()
	return n.status
}

func (n *Node) isLeader() bool {
	return n.getStatus() == pb.NodeStatus_Leader
}

func (n *Node) isFollower() bool {
	return n.getStatus() == pb.NodeStatus_Follower
}

func (n *Node) newLeader(nodeID string) {
	n.Lock()
	defer n.Unlock()

	n.voted = false
	n.votes = 0

	if n.id == nodeID {
		n.status = pb.NodeStatus_Leader
	} else {
		n.status = pb.NodeStatus_Follower
	}

	for id, node := range n.nodes {
		if id == nodeID {
			node.status = pb.NodeStatus_Leader
		} else {
			node.status = pb.NodeStatus_Follower
		}
	}
}

func (n *Node) getLeader() *Node {
	n.RLock()
	defer n.RUnlock()

	if n.status == pb.NodeStatus_Leader {
		return n
	}

	for _, node := range n.nodes {
		if node.status == pb.NodeStatus_Leader {
			return node
		}
	}
	return nil
}

func (n *Node) heartBeat(ctx context.Context) error {
	n.setHeartBeat()

	var err error
	wg := &sync.WaitGroup{}
	for _, node := range n.Nodes() {
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			if e := n.grpcClient.heartBeat(ctx, node.AddrPort()); e != nil {
				err = errors.Join(err, fmt.Errorf("node %s: %w", node.ID(), e))
			}
		}(wg)
	}
	wg.Wait()

	return err
}
