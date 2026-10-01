// Package ui is the Gio window for polkit requests. Each request moves through:
// review (read what will run) -> authenticate (password / key touch) -> done.
package ui

import (
	"context"
	"errors"
	"sync"
	"time"

	"gioui.org/app"

	"github.com/elee1766/giopolkit/internal/agent"
	"github.com/elee1766/giopolkit/internal/inspect"
)

var ErrDenied = errors.New("denied by user")

// stage of a request.
type stage int

const (
	stReview stage = iota
	stAuth
	stDone
)

// session is the state shared between the agent goroutine (running PAM) and the window.
type session struct {
	req    *agent.Request
	report inspect.Report

	mu       sync.Mutex
	stage    stage
	prompt   string // current PAM prompt text
	secret   bool   // prompt is echo-off
	waiting  bool   // a prompt is open and needs an answer
	info     []string
	errMsg   string
	answer   chan string
	decision chan bool // review: true approve, false deny
	invalid  func()
}

func (s *session) update(f func()) {
	s.mu.Lock()
	f()
	inv := s.invalid
	s.mu.Unlock()
	if inv != nil {
		inv()
	}
}

// UI implements agent.UI. Requests are shown one at a time in the order they arrive.
type UI struct {
	queue sync.Mutex
}

func New() *UI { return &UI{} }

// Run starts the Gio main loop. It must be called from main and never returns.
func Run() { app.Main() }

const maxAttempts = 3

func (u *UI) Handle(r *agent.Request, auth agent.Authenticator) error {
	// One window at a time. polkitd can send several requests; they wait their turn.
	u.queue.Lock()
	defer u.queue.Unlock()
	if r.Context().Err() != nil {
		return r.Context().Err()
	}
	s := &session{
		req:      r,
		report:   inspect.Build(r.ActionID, r.Details),
		answer:   make(chan string),
		decision: make(chan bool, 1),
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	closed := make(chan struct{})
	go func() {
		show(ctx, s)
		close(closed)
		cancel()
	}()
	defer func() { cancel(); <-closed }()

	select {
	case ok := <-s.decision:
		if !ok {
			return ErrDenied
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	id := pickIdentity(r.Identities)
	s.update(func() { s.stage = stAuth })
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err = auth(ctx, id, func(ctx context.Context, kind agent.PromptKind, text string) (string, error) {
			switch kind {
			case agent.InfoText:
				s.update(func() { s.info = append(s.info, text) })
				return "", nil
			case agent.ErrorText:
				s.update(func() { s.errMsg = text })
				return "", nil
			}
			s.update(func() {
				s.prompt, s.secret, s.waiting = text, kind == agent.PromptSecret, true
			})
			select {
			case a := <-s.answer:
				s.update(func() { s.waiting, s.errMsg, s.info = false, "", nil })
				return a, nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})
		if err == nil {
			s.update(func() { s.stage = stDone })
			time.Sleep(250 * time.Millisecond)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.update(func() { s.errMsg, s.waiting, s.info = "Authentication failed. Try again.", false, nil })
	}
	return err
}

// pickIdentity prefers the current user, then any non-root user, then root.
func pickIdentity(ids []agent.Identity) agent.Identity {
	me := uint32(myUID())
	for _, id := range ids {
		if id.UID == me {
			return id
		}
	}
	for _, id := range ids {
		if id.UID != 0 {
			return id
		}
	}
	return ids[0]
}
