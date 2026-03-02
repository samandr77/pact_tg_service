package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
)

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	rootCtx  context.Context
	appID    int
	appHash  string
}

func NewSessionManager(ctx context.Context, appID int, appHash string) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*Session),
		rootCtx:  ctx,
		appID:    appID,
		appHash:  appHash,
	}
}

func (m *SessionManager) Create(ctx context.Context) (string, string, error) {
	id, err := generateID()
	if err != nil {
		return "", "", fmt.Errorf("create session: %w", err)
	}

	sess := &Session{
		id:       id,
		msgCh:    make(chan MessageUpdate, 100),
		logoutCh: make(chan struct{}),
	}

	dispatcher := tg.NewUpdateDispatcher()
	loggedIn := qrlogin.OnLoginToken(dispatcher)
	dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		msg, ok := u.Message.(*tg.Message)
		if !ok {
			return nil
		}
		update := MessageUpdate{
			MessageID: int64(msg.ID),
			From:      extractFrom(msg.FromID),
			Text:      msg.Message,
			Timestamp: int64(msg.Date),
		}
		select {
		case sess.msgCh <- update:
		default:
			slog.Warn("msgCh переполнен, сообщение отброшено", "session_id", id)
		}
		return nil
	})

	client := telegram.NewClient(m.appID, m.appHash, telegram.Options{
		UpdateHandler: dispatcher,
	})

	sessCtx, cancel := context.WithCancel(m.rootCtx)
	sess.client = client
	sess.cancel = cancel

	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()

	qrURLCh := make(chan string, 1)

	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.sessions, id)
			m.mu.Unlock()
			cancel()
		}()

		runErr := client.Run(sessCtx, func(ctx context.Context) error {
			qr := client.QR()
			_, authErr := qr.Auth(ctx, loggedIn,
				func(ctx context.Context, token qrlogin.Token) error {
					select {
					case qrURLCh <- token.URL():
					default:
					}
					return nil
				},
			)
			if authErr != nil && !errors.Is(authErr, context.Canceled) {
				return fmt.Errorf("qr auth: %w", authErr)
			}

			sess.setAuthorised(true)
			slog.Info("сессия авторизована", "session_id", id)

			select {
			case <-sess.logoutCh:
				if _, logoutErr := client.API().AuthLogOut(ctx); logoutErr != nil {
					slog.Error("auth logout failed", "session_id", id, "err", logoutErr)
				}
			case <-ctx.Done():
			}
			return nil
		})

		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			slog.Error("сессия завершилась с ошибкой", "session_id", id, "err", runErr)
		}
	}()

	select {
	case url := <-qrURLCh:
		return id, url, nil
	case <-ctx.Done():
		return "", "", fmt.Errorf("create session: %w", ctx.Err())
	}
}

func (m *SessionManager) Get(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}

func (m *SessionManager) Delete(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return ErrNotFound
	}
	delete(m.sessions, id)
	m.mu.Unlock()

	if s.IsAuthorised() {
		close(s.logoutCh)
	} else {
		s.cancel()
	}
	return nil
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func extractFrom(peer tg.PeerClass) string {
	switch v := peer.(type) {
	case *tg.PeerUser:
		return strconv.FormatInt(v.UserID, 10)
	case *tg.PeerChat:
		return strconv.FormatInt(v.ChatID, 10)
	case *tg.PeerChannel:
		return strconv.FormatInt(v.ChannelID, 10)
	default:
		return ""
	}
}
