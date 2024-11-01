package node

import (
	"context"
	"log/slog"

	pb "github.com/atlet99/leadelect/grpc"
)

type server struct {
	pb.UnimplementedNodeServer
	node   *Node
	logger *slog.Logger
}

func newServer(node *Node, logger *slog.Logger) *server {
	return &server{
		node:   node,
		logger: logger,
	}
}

// Status returns the status and round of the node.
func (s *server) Status(ctx context.Context, in *pb.EmptyRequest) (*pb.NodeStatusResponse, error) {
	s.logger.Info("Received Status request", "nodeID", s.node.ID())
	return &pb.NodeStatusResponse{
		Status: s.node.Status(),
		Round:  s.node.getRound(),
	}, nil
}

// Vote handles a voting request from another node.
func (s *server) Vote(ctx context.Context, in *pb.VoteRequest) (*pb.VoteResponse, error) {
	s.logger.Info("Received Vote request", "nodeID", s.node.ID(), "round", in.Round)
	vote := s.node.agreeVote(in.Round)
	return &pb.VoteResponse{
		Vote:  vote,
		Round: s.node.getRound(),
	}, nil
}

// HeartBeat processes a heartbeat from the leader or candidate.
func (s *server) HeartBeat(ctx context.Context, in *pb.HeartBeatRequest) (*pb.EmptyResponse, error) {
	s.logger.Info("Received HeartBeat", "nodeID", s.node.ID(), "leaderID", in.NodeID, "round", in.Round)

	// Update heartbeat time and check for leadership change
	s.node.setHeartBeat()

	leader := s.node.getLeader()
	if leader == nil || leader.getRound() < in.Round {
		s.logger.Info("Updating Leader", "newLeaderID", in.NodeID)
		s.node.newLeader(in.NodeID)
		s.node.setRound(in.Round)
	}
	return &pb.EmptyResponse{}, nil
}
