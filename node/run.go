package node

import (
	"context"
	"log/slog"
	"math/rand"
	"net"
	"sync"
	"time"

	pb "github.com/atlet99/leadelect/grpc"
)

// handleError processes errors and logs critical cases
func handleError(logger *slog.Logger, err error, msg string) {
	if err != nil {
		logger.Error(msg, "error", err.Error())
	}
}

func (n *Node) Run(ctx context.Context, logger *slog.Logger) {
	go n.serverRun(ctx, logger)

	// Initialization and leader identification
	n.initialization(ctx, logger)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	nextCheck := time.Now()
	nextHeartBeat := time.Now()
	for t := range ticker.C {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Leader status check
		if n.isLeader() && t.After(nextHeartBeat) {
			n.setHeartBeat()
			nextHeartBeat = time.Now().Add(n.heartBeatTimeout)
			if err := n.heartBeat(ctx); err != nil {
				handleError(logger, err, "Run heartBeat failed")
			}

		} else if n.getLeader() != nil && n.heartBeatExpired() {
			n.newLeader("")
		}

		if n.getLeader() != nil && t.Before(nextCheck) {
			continue
		}

		logger.Info("Run", "status", n.getStatus())

		if n.getLeader() == nil {
			n.election(ctx, logger)
			time.Sleep(time.Second)
		} else {
			nextCheck = time.Now().Add(n.checkElectionTimeout)
		}
	}
}

func (n *Node) serverRun(ctx context.Context, logger *slog.Logger) {
	var lis net.Listener
	var err error

	// Retry logic for server startup
	for i := 0; i < 3; i++ {
		lis, err = net.Listen("tcp", n.AddrPort())
		if err == nil {
			break
		}
		logger.Error("serverRun net.Listen retrying...", "attempt", i+1, "error", err.Error())
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		handleError(logger, err, "serverRun failed to bind to address")
		return
	}

	pb.RegisterNodeServer(n.grpcServer, newServer(n, logger))

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		<-ctx.Done()
		n.grpcServer.GracefulStop()
		logger.Info("serverRun Graceful stop", "id", n.ID(), "addr", n.AddrPort())
		wg.Done()
	}()

	logger.Info("serverRun", "id", n.ID(), "addr", n.AddrPort())
	if err := n.grpcServer.Serve(lis); err != nil {
		logger.Error("serverRun Serve failed", "id", n.ID(), "addr", n.AddrPort(), "err", err.Error())
	}
	wg.Wait()
}

func (n *Node) initialization(ctx context.Context, logger *slog.Logger) {
	type data struct {
		nodeID string
		resp   *pb.NodeStatusResponse
	}

	ch := make(chan *data)
	defer close(ch)

	go func() {
		wg := &sync.WaitGroup{}
		for _, node := range n.Nodes() {
			logger.Info("initialization", "nodeID", node.id)

			wg.Add(1)
			go func(wg *sync.WaitGroup, ch chan *data, node *Node) {
				defer wg.Done()

				resp, err := n.grpcClient.status(ctx, node.AddrPort())
				if err != nil {
					logger.Info("initialization status failed", "nodeID", node.id, "err", err.Error())
					ch <- nil
					return
				}

				logger.Info("initialization", "nodeID", node.id, "status", resp.Status, "round", resp.Round)
				ch <- &data{
					nodeID: node.ID(),
					resp:   resp,
				}
			}(wg, ch, node)
		}
		wg.Wait()
	}()

	maxRound := int64(0)
	hasLeader := false
	for i := 0; i < len(n.Nodes()); i++ {
		select {
		case <-ctx.Done():
			return
		case d := <-ch:
			if d == nil {
				continue
			}

			if d.resp.Status == pb.NodeStatus_Leader {
				hasLeader = true
				n.newLeader(d.nodeID)
				n.setHeartBeat()
				n.setRound(d.resp.Round)
			} else if d.resp.Round > maxRound {
				maxRound = d.resp.Round
			}
		}
	}

	if !hasLeader && maxRound > n.getRound() {
		n.setRound(maxRound)
	}
}

func (n *Node) election(ctx context.Context, logger *slog.Logger) {
	n.resetVotes()
	if n.isVoted() {
		n.setStatus(pb.NodeStatus_Follower)
		return
	}

	time.Sleep(time.Millisecond * time.Duration(rand.Intn(150)+150))

	n.addRound()
	n.setVoted()
	n.addVote()
	n.setStatus(pb.NodeStatus_Candidate)

	wg := &sync.WaitGroup{}
	maxRound := int64(0)
	for _, node := range n.Nodes() {
		wg.Add(1)
		go func(node *Node) {
			defer wg.Done()

			if n.isFollower() {
				return
			}

			resp, err := n.grpcClient.vote(ctx, node.AddrPort())
			if err != nil {
				logger.Error("election vote failed", "nodeID", node.id, "err", err.Error())
				return
			}

			logger.Info("election", "nodeID", node.id, "vote", resp.Vote, "round", resp.Round)
			if maxRound < resp.Round {
				maxRound = resp.Round
			}
			if resp.Vote {
				n.addVote()
			}
		}(node)
	}
	wg.Wait()

	logger.Info("election result", "votes", n.numVotes())
	if n.numVotes() > n.NumNodes()/2 {
		n.newLeader(n.id)
		if err := n.heartBeat(ctx); err != nil {
			logger.Error("election heartBeat failed", "err", err.Error())
		}
		return
	} else if maxRound > n.getRound() {
		n.setRound(maxRound)
	}

	n.resetVotes()
	n.setStatus(pb.NodeStatus_Follower)
	n.resetVoted()
}
