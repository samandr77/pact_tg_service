package session

import (
	"context"
	"sync"

	"github.com/gotd/td/telegram"
)

type MessageUpdate struct {
	MessageID int64
	From      string
	Text      string
	Timestamp int64
}

type Session struct {
	id         string
	client     *telegram.Client
	cancel     context.CancelFunc
	msgCh      chan MessageUpdate
	authorised bool
	mu         sync.RWMutex
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) MsgCh() <-chan MessageUpdate {
	return s.msgCh
}

func (s *Session) IsAuthorised() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.authorised
}

func (s *Session) Client() *telegram.Client {
	return s.client
}

func (s *Session) setAuthorised(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authorised = v
}
