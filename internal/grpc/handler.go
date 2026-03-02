package handler

import (
	"context"
	"errors"
	"log/slog"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/samandr77/pact_tg_service/gen"
	"github.com/samandr77/pact_tg_service/internal/session"
)

type Handler struct {
	pb.UnimplementedTelegramServiceServer
	sessions *session.SessionManager
}

func NewHandler(sessions *session.SessionManager) *Handler {
	return &Handler{sessions: sessions}
}

func (h *Handler) CreateSession(ctx context.Context, _ *pb.CreateSessionRequest) (*pb.CreateSessionResponse, error) {
	id, qrURL, err := h.sessions.Create(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create session: %v", err)
	}
	return &pb.CreateSessionResponse{
		SessionId: &id,
		QrCode:    &qrURL,
	}, nil
}

func (h *Handler) DeleteSession(_ context.Context, req *pb.DeleteSessionRequest) (*pb.DeleteSessionResponse, error) {
	if err := h.sessions.Delete(req.GetSessionId()); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "session %s not found", req.GetSessionId())
		}
		return nil, status.Errorf(codes.Internal, "delete session: %v", err)
	}
	return &pb.DeleteSessionResponse{}, nil
}

func (h *Handler) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	if req.GetPeer() == "" {
		return nil, status.Error(codes.InvalidArgument, "peer is required")
	}

	sess, err := h.sessions.Get(req.GetSessionId())
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "session %s not found", req.GetSessionId())
		}
		return nil, status.Errorf(codes.Internal, "get session: %v", err)
	}

	if !sess.IsAuthorised() {
		return nil, status.Errorf(codes.FailedPrecondition, "session %s is not authorised", req.GetSessionId())
	}

	sender := message.NewSender(sess.Client().API())
	updates, err := sender.Resolve(req.GetPeer()).Text(ctx, req.GetText())
	if err != nil {
		slog.Error("failed to send message", "session_id", req.GetSessionId(), "err", err)
		return nil, status.Errorf(codes.Internal, "send message: %v", err)
	}

	msgID := extractMessageID(updates)
	return &pb.SendMessageResponse{MessageId: &msgID}, nil
}

func (h *Handler) SubscribeMessages(req *pb.SubscribeMessagesRequest, stream grpc.ServerStreamingServer[pb.MessageUpdate]) error {
	sess, err := h.sessions.Get(req.GetSessionId())
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return status.Errorf(codes.NotFound, "session %s not found", req.GetSessionId())
		}
		return status.Errorf(codes.Internal, "get session: %v", err)
	}

	if !sess.IsAuthorised() {
		return status.Errorf(codes.FailedPrecondition, "session %s is not authorised", req.GetSessionId())
	}

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case update := <-sess.MsgCh():
			msgID := update.MessageID
			if err := stream.Send(&pb.MessageUpdate{
				MessageId: &msgID,
				From:      &update.From,
				Text:      &update.Text,
				Timestamp: &update.Timestamp,
			}); err != nil {
				return err
			}
		}
	}
}

func extractMessageID(updates tg.UpdatesClass) int64 {
	switch u := updates.(type) {
	case *tg.UpdateShortSentMessage:
		return int64(u.ID)
	case *tg.Updates:
		for _, upd := range u.Updates {
			if m, ok := upd.(*tg.UpdateMessageID); ok {
				return int64(m.ID)
			}
		}
	}
	return 0
}
