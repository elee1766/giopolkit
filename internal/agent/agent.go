// Package agent implements a polkit authentication agent: it registers with polkitd for the
// current session, receives BeginAuthentication calls, and drives polkit-agent-helper-1 to
// check the user's credentials through PAM.
package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	authorityName  = "org.freedesktop.PolicyKit1"
	authorityPath  = "/org/freedesktop/PolicyKit1/Authority"
	authorityIface = "org.freedesktop.PolicyKit1.Authority"
	agentIface     = "org.freedesktop.PolicyKit1.AuthenticationAgent"
	ObjectPath     = dbus.ObjectPath("/io/github/elee1766/giopolkit/AuthenticationAgent")
)

// Identity is a user polkit will accept authentication for.
type Identity struct {
	UID  uint32
	Name string
}

// Request is one BeginAuthentication call.
type Request struct {
	ActionID   string
	Message    string
	IconName   string
	Details    map[string]string
	Cookie     string
	Identities []Identity
	ctx        context.Context
	cancel     context.CancelFunc
}

func (r *Request) Context() context.Context { return r.ctx }

// UI shows a request and drives authentication. It returns nil only after Authenticate succeeded.
// Returning an error (or ctx being cancelled) tells polkitd the user dismissed the dialog.
type UI interface {
	Handle(r *Request, auth Authenticator) error
}

// Authenticator runs one PAM conversation for an identity.
type Authenticator func(ctx context.Context, id Identity, conv Conversation) error

type Agent struct {
	conn    *dbus.Conn
	ui      UI
	subject subject

	mu       sync.Mutex
	inflight map[string]*Request
}

type subject struct {
	Kind    string
	Details map[string]dbus.Variant
}

type identityWire struct {
	Kind    string
	Details map[string]dbus.Variant
}

// SessionID finds the logind session to register for: XDG_SESSION_ID, or the session of this process.
func SessionID() (string, error) {
	if s := os.Getenv("XDG_SESSION_ID"); s != "" {
		return s, nil
	}
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return "", err
	}
	defer conn.Close()
	login := conn.Object("org.freedesktop.login1", "/org/freedesktop/login1")
	var path dbus.ObjectPath
	if err := login.Call("org.freedesktop.login1.Manager.GetSessionByPID", 0, uint32(os.Getpid())).Store(&path); err == nil {
		var v dbus.Variant
		if err := conn.Object("org.freedesktop.login1", path).Call("org.freedesktop.DBus.Properties.Get", 0, "org.freedesktop.login1.Session", "Id").Store(&v); err == nil {
			if s, ok := v.Value().(string); ok {
				return s, nil
			}
		}
	}
	// Fall back to the user's display session.
	var v dbus.Variant
	upath := dbus.ObjectPath("/org/freedesktop/login1/user/_" + strconv.Itoa(os.Getuid()))
	if err := conn.Object("org.freedesktop.login1", upath).Call("org.freedesktop.DBus.Properties.Get", 0, "org.freedesktop.login1.User", "Display").Store(&v); err != nil {
		return "", fmt.Errorf("find logind session: %w", err)
	}
	if st, ok := v.Value().([]any); ok && len(st) == 2 {
		if s, ok := st[0].(string); ok && s != "" {
			return s, nil
		}
	}
	return "", errors.New("no graphical logind session found; set XDG_SESSION_ID")
}

func New(ui UI, sessionID string) (*Agent, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	a := &Agent{
		conn: conn, ui: ui, inflight: map[string]*Request{},
		subject: subject{Kind: "unix-session", Details: map[string]dbus.Variant{"session-id": dbus.MakeVariant(sessionID)}},
	}
	if err := conn.ExportWithMap(a, map[string]string{"BeginAuthentication": "BeginAuthentication", "CancelAuthentication": "CancelAuthentication"}, ObjectPath, agentIface); err != nil {
		conn.Close()
		return nil, err
	}
	return a, nil
}

// Register makes this the session's authentication agent. It fails if another agent (lxpolkit)
// is already registered for the session.
func (a *Agent) Register() error {
	locale := os.Getenv("LANG")
	if locale == "" {
		locale = "C"
	}
	obj := a.conn.Object(authorityName, authorityPath)
	opts := map[string]dbus.Variant{"fallback": dbus.MakeVariant(false)}
	return obj.Call(authorityIface+".RegisterAuthenticationAgentWithOptions", 0, a.subject, locale, string(ObjectPath), opts).Err
}

func (a *Agent) Unregister() error {
	obj := a.conn.Object(authorityName, authorityPath)
	return obj.Call(authorityIface+".UnregisterAuthenticationAgent", 0, a.subject, string(ObjectPath)).Err
}

func (a *Agent) Close() { a.conn.Close() }

// BeginAuthentication is called by polkitd. It blocks until the user finishes or dismisses.
func (a *Agent) BeginAuthentication(sender dbus.Sender, actionID, message, iconName string, details map[string]string, cookie string, identities []identityWire) *dbus.Error {
	if !a.fromPolkitd(string(sender)) {
		return dbus.NewError("org.freedesktop.PolicyKit1.Error.Failed", []any{"caller is not polkitd"})
	}
	ids := make([]Identity, 0, len(identities))
	for _, w := range identities {
		if w.Kind != "unix-user" {
			continue
		}
		v, ok := w.Details["uid"]
		if !ok {
			continue
		}
		uid, ok := v.Value().(uint32)
		if !ok {
			continue
		}
		name := strconv.FormatUint(uint64(uid), 10)
		if u, err := user.LookupId(name); err == nil {
			name = u.Username
		}
		ids = append(ids, Identity{UID: uid, Name: name})
	}
	if len(ids) == 0 {
		return dbus.NewError("org.freedesktop.PolicyKit1.Error.Failed", []any{"no supported identities"})
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &Request{ActionID: actionID, Message: message, IconName: iconName, Details: details, Cookie: cookie, Identities: ids, ctx: ctx, cancel: cancel}
	a.mu.Lock()
	a.inflight[cookie] = r
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.inflight, cookie)
		a.mu.Unlock()
		cancel()
	}()

	err := a.ui.Handle(r, func(ctx context.Context, id Identity, conv Conversation) error {
		return runHelper(ctx, id, cookie, conv)
	})
	if err != nil {
		return dbus.NewError("org.freedesktop.PolicyKit1.Error.Cancelled", []any{err.Error()})
	}
	return nil
}

func (a *Agent) CancelAuthentication(sender dbus.Sender, cookie string) *dbus.Error {
	if !a.fromPolkitd(string(sender)) {
		return dbus.NewError("org.freedesktop.PolicyKit1.Error.Failed", []any{"caller is not polkitd"})
	}
	a.mu.Lock()
	r := a.inflight[cookie]
	a.mu.Unlock()
	if r != nil {
		r.cancel()
	}
	return nil
}

// fromPolkitd checks that a call came from the owner of org.freedesktop.PolicyKit1. Anyone on
// the system bus can call our object, so without this a local process could pop up fake requests.
func (a *Agent) fromPolkitd(sender string) bool {
	var owner string
	if err := a.conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, authorityName).Store(&owner); err != nil {
		return false
	}
	return owner == sender
}
