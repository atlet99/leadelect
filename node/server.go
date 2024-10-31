package node

import (
	"context"
	"log/slog"

	pb "github.com/NovikovRoman/leadelect/grpc"
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

func (s *server) Status(ctx context.Context, in *pb.EmptyRequest) (*pb.NodeStatusResponse, error) {
	return &pb.NodeStatusResponse{
		Status: s.node.Status(),
		Round:  s.node.getRound(),
	}, nil
}

func (s *server) Vote(ctx context.Context, in *pb.VoteRequest) (*pb.VoteResponse, error) {
	return &pb.VoteResponse{
		Vote:  s.node.agreeVote(in.Round),
		Round: in.Round,
	}, nil
}

func (s *server) HeartBeat(ctx context.Context, in *pb.HeartBeatRequest) (*pb.EmptyResponse, error) {
	s.node.setHeartBeat()

	leader := s.node.getLeader()
	if leader == nil || leader.getRound() < in.Round {
		s.logger.Info("HeartBeat", "Leader", in.NodeID)
		s.node.newLeader(in.NodeID)
		s.node.setRound(in.Round)
	}
	return &pb.EmptyResponse{}, nil
}
