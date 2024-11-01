package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	pb "github.com/atlet99/leadelect/grpc"
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

	round int64 // Voting round
	votes int   // Votes from nodes
	voted bool  // Has voted

	nodes map[string]*Node // Other nodes

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

func New(id, addr string, port int, opts ...NodeOpt) *Node {
	n := &Node{
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
	return n
}

func (n *Node) ClientTLS(caFile, serverHostOverride string) error {
	caFile = absPath(caFile)
	creds, err := credentials.NewClientTLSFromFile(caFile, serverHostOverride)
	if err != nil {
		return fmt.Errorf("failed to load client TLS credentials: %w", err)
	}
	n.grpcClient = newClient(n, []grpc.DialOption{grpc.WithTransportCredentials(creds)})
	return nil
}

func (n *Node) ServerTLS(certFile, keyFile string) error {
	creds, err := credentials.NewServerTLSFromFile(absPath(certFile), absPath(keyFile))
	if err != nil {
		return fmt.Errorf("failed to load server TLS credentials: %w", err)
	}
	n.grpcServer = grpc.NewServer(grpc.Creds(creds))
	return nil
}

func (n *Node) heartBeat(ctx context.Context) error {
	n.setHeartBeat()

	var mu sync.Mutex // Mutex for concurrent error access
	var err error
	wg := &sync.WaitGroup{}
	for _, node := range n.Nodes() {
		wg.Add(1)
		go func(node *Node) {
			defer wg.Done()
			if e := n.grpcClient.heartBeat(ctx, node.AddrPort()); e != nil {
				mu.Lock()
				err = fmt.Errorf("node %s: %w", node.ID(), e)
				mu.Unlock()
			}
		}(node)
	}
	wg.Wait()

	return err
}
