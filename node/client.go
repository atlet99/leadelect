package node

import (
	"context"
	"fmt"
	"log"

	pb "github.com/atlet99/leadelect/grpc"
	"google.golang.org/grpc"
)

type client struct {
	node           *Node
	grpcClientOpts []grpc.DialOption
}

func newClient(node *Node, opts []grpc.DialOption) *client {
	return &client{
		node:           node,
		grpcClientOpts: opts,
	}
}

// heartBeat sends a heartbeat message to the target node and returns an error if unsuccessful
func (c *client) heartBeat(ctx context.Context, target string) error {
	ctx, cancel := context.WithTimeout(ctx, c.node.clientTimeout)
	defer cancel()

	conn, err := grpc.Dial(target, c.grpcClientOpts...)
	if err != nil {
		return fmt.Errorf("failed to connect to target %s for heartBeat: %w", target, err)
	}
	defer conn.Close()

	client := pb.NewNodeClient(conn)
	_, err = client.HeartBeat(ctx, &pb.HeartBeatRequest{NodeID: c.node.id, Round: c.node.getRound()})
	if err != nil {
		log.Printf("heartBeat request to target %s failed: %v", target, err)
	}
	return err
}

// vote sends a vote request to the target node and returns the response or an error if unsuccessful
func (c *client) vote(ctx context.Context, target string) (*pb.VoteResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.node.clientTimeout)
	defer cancel()

	conn, err := grpc.Dial(target, c.grpcClientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to target %s for vote: %w", target, err)
	}
	defer conn.Close()

	client := pb.NewNodeClient(conn)
	resp, err := client.Vote(ctx, &pb.VoteRequest{NodeID: c.node.id, Round: c.node.getRound()})
	if err != nil {
		log.Printf("vote request to target %s failed: %v", target, err)
		return nil, err
	}
	return resp, nil
}

// status retrieves the status from the target node and returns the response or an error if unsuccessful
func (c *client) status(ctx context.Context, target string) (*pb.NodeStatusResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.node.clientTimeout)
	defer cancel()

	conn, err := grpc.Dial(target, c.grpcClientOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to target %s for status: %w", target, err)
	}
	defer conn.Close()

	client := pb.NewNodeClient(conn)
	resp, err := client.Status(ctx, &pb.EmptyRequest{})
	if err != nil {
		log.Printf("status request to target %s failed: %v", target, err)
	}
	return resp, err
}
