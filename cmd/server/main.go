package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	pb "github.com/samandr77/pact_tg_service/gen"
	"github.com/samandr77/pact_tg_service/internal/config"
	handler "github.com/samandr77/pact_tg_service/internal/grpc"
	"github.com/samandr77/pact_tg_service/internal/session"
)

func main() {
	cfg := config.Load()

	if cfg.IsProduction() {
		slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	} else {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sessions := session.NewSessionManager(ctx, cfg.AppID, cfg.AppHash)

	lis, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatalf("listen %s: %v", cfg.GRPCAddr, err)
	}

	srv := grpc.NewServer()
	pb.RegisterTelegramServiceServer(srv, handler.NewHandler(sessions))

	slog.Info("gRPC server starting", "addr", cfg.GRPCAddr)

	go func() {
		if serveErr := srv.Serve(lis); serveErr != nil {
			slog.Error("gRPC serve error", "err", serveErr)
		}
	}()

	<-ctx.Done()

	slog.Info("shutting down")
	srv.GracefulStop()
}
