// rootpls-agent is a polkit authentication agent with a readable confirmation window. It replaces
// lxpolkit (or any other agent) for the current session.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/elee1766/rootpls/internal/agent"
	"github.com/elee1766/rootpls/internal/ui"
)

func main() {
	session := flag.String("session", "", "logind session id (default: XDG_SESSION_ID or detected)")
	flag.Parse()

	sid := *session
	if sid == "" {
		var err error
		if sid, err = agent.SessionID(); err != nil {
			fmt.Fprintln(os.Stderr, "rootpls-agent:", err)
			os.Exit(1)
		}
	}
	a, err := agent.New(ui.New(), sid)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rootpls-agent:", err)
		os.Exit(1)
	}
	if err := a.Register(); err != nil {
		fmt.Fprintf(os.Stderr, "rootpls-agent: register for session %s: %v\n(is another agent such as lxpolkit running?)\n", sid, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "rootpls-agent: registered for session %s\n", sid)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		a.Unregister()
		a.Close()
		os.Exit(0)
	}()
	ui.Run()
}
