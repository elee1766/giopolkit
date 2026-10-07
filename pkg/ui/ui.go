// Package ui is the Gio window for polkit requests. Each request moves through:
// review (read what will run) -> authenticate (password / key touch) -> done.
package ui

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"gioui.org/app"

	"github.com/elee1766/giopolkit/pkg/polkit/agent"
	"github.com/elee1766/giopolkit/pkg/polkit/pkexec"
	"github.com/elee1766/giopolkit/pkg/rules"
	"github.com/elee1766/giopolkit/pkg/ui/theme"
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
	report pkexec.Report

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
	opts  Options
}

// Options configure the window.
type Options struct {
	Palette theme.Palette
	Rules   []rules.Rule // checks run on pkexec commands
}

func New(opts Options) *UI { return &UI{opts: opts} }

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
		report:   pkexec.Build(r.ActionID, r.Details, u.opts.Rules),
		answer:   make(chan string),
		decision: make(chan bool, 1),
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	closed := make(chan struct{})
	go func() {
		show(ctx, s, u.opts.Palette)
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

	id := agent.PickIdentity(r.Identities, uint32(os.Getuid()))
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
