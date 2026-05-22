// Mock gRPC backend — provides OrderService with dummy data
package main

import (
	"context"
	"log"
	"net"
	"os"

	pb "mcp-gateway-go-demo/dev/mock-grpc-backend/orders"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

func main() {
	log.SetFlags(0)
	port := "50051"
	if p := os.Getenv("GRPC_PORT"); p != "" {
		port = p
	}
	log.Printf("=== Mock gRPC Backend (port=%s) ===", port)

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterOrderServiceServer(s, &orderServer{})
	reflection.Register(s)

	log.Println("Listening on :" + port)
	log.Println("Service: orders.OrderService (GetOrder, CreateOrder)")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

type orderServer struct {
	pb.UnimplementedOrderServiceServer
}

func (s *orderServer) GetOrder(ctx context.Context, req *pb.GetOrderRequest) (*pb.Order, error) {
	log.Printf("[gRPC] GetOrder order_id=%s", req.OrderId)
	if req.OrderId == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	return &pb.Order{
		OrderId:  req.OrderId,
		Customer: "张三",
		Amount:   299.99,
		Status:   "paid",
		Items:    3,
	}, nil
}

func (s *orderServer) CreateOrder(ctx context.Context, req *pb.CreateOrderRequest) (*pb.Order, error) {
	log.Printf("[gRPC] CreateOrder customer=%s amount=%.2f", req.Customer, req.Amount)
	return &pb.Order{
		OrderId:  "ORD-NEW-001",
		Customer: req.Customer,
		Amount:   req.Amount,
		Status:   "pending",
		Items:    0,
	}, nil
}
