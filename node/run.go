package node

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"net"
	"sync"
	"time"

	pb "github.com/NovikovRoman/leadelect/grpc"
)

func (n *Node) Run(ctx context.Context, logger *slog.Logger) {
	go n.serverRun(ctx, logger)

	// And who is the leader here?
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

		if n.isLeader() && t.After(nextHeartBeat) {
			n.setHeartBeat()
			nextHeartBeat = time.Now().Add(n.heartBeatTimeout)
			if err := n.heartBeat(ctx); err != nil {
				logger.Error("Run heartBeat", "err", err.Error())
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
	lis, err := net.Listen("tcp", n.AddrPort())
	if err != nil {
		logger.Error("serverRun net.Listen", "id", n.ID(), "addr", n.AddrPort(), "err", err.Error())
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
		logger.Error("serverRun Serve", "id", n.ID(), "addr", n.AddrPort(), "err", err.Error())
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
			go func(wg *sync.WaitGroup, ch chan *data) {
				defer wg.Done()

				resp, err := n.grpcClient.status(ctx, node.AddrPort())
				if err != nil {
					logger.Info("initialization status", "nodeID", node.id, "err", err.Error())
					ch <- nil
					return
				}

				logger.Info("initialization", "nodeID", node.id, "status", resp.Status, "round", resp.Round)
				ch <- &data{
					nodeID: node.ID(),
					resp:   resp,
				}
			}(wg, ch)
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

	time.Sleep(time.Millisecond * time.Duration(rand.Int32N(150)+150))

	n.addRound()
	n.setVoted()
	n.addVote()
	n.setStatus(pb.NodeStatus_Candidate)

	ch := make(chan string)
	defer close(ch)

	wg := &sync.WaitGroup{}

	maxRound := int64(0)
	for _, node := range n.Nodes() {
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()

			if n.isFollower() {
				return
			}

			resp, err := n.grpcClient.vote(ctx, node.AddrPort())
			if err != nil {
				logger.Error("election vote", "nodeID", node.id, "err", err.Error())
				return
			}

			logger.Info("election", "nodeID", node.id, "vote", resp.Vote, "round", resp.Round)
			if maxRound < resp.Round {
				maxRound = resp.Round
			}
			if resp.Vote {
				n.addVote()
			}
		}(wg)
	}
	wg.Wait()

	logger.Info("election", "votes", n.numVotes())
	if n.numVotes() > n.NumNodes()/2 {
		n.newLeader(n.id)
		if err := n.heartBeat(ctx); err != nil {
			logger.Error("election heartBeat", "err", err.Error())
		}
		return

	} else if maxRound > n.getRound() {
		n.setRound(maxRound)
	}

	n.resetVotes()
	n.setStatus(pb.NodeStatus_Follower)
	n.resetVoted()
}
