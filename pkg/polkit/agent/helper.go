package agent

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

var helperPaths = []string{
	"/usr/lib/polkit-1/polkit-agent-helper-1",
	"/usr/libexec/polkit-agent-helper-1",
	"/usr/lib/policykit-1/polkit-agent-helper-1",
	"/usr/libexec/polkit-1/polkit-agent-helper-1",
}

// PromptKind is a PAM conversation message type.
type PromptKind int

const (
	PromptSecret PromptKind = iota // PAM_PROMPT_ECHO_OFF: password, PIN
	PromptText                     // PAM_PROMPT_ECHO_ON
	InfoText                       // PAM_TEXT_INFO: e.g. "Please touch the device."
	ErrorText                      // PAM_ERROR_MSG
)

// Conversation receives PAM messages. For prompts it returns the user's answer. Returning an
// error aborts the attempt. Info and error messages return "" and nil.
type Conversation func(ctx context.Context, kind PromptKind, text string) (string, error)

var ErrAuthFailed = errors.New("authentication failed")

func helperPath() (string, error) {
	for _, p := range helperPaths {
		if st, err := os.Stat(p); err == nil && st.Mode()&os.ModeSetuid != 0 {
			return p, nil
		}
	}
	return "", errors.New("setuid polkit-agent-helper-1 not found")
}

// runHelper runs polkit-agent-helper-1 for one identity. The helper runs PAM as root and, on
// success, tells polkitd itself. The agent only relays the conversation.
func runHelper(ctx context.Context, id Identity, cookie string, conv Conversation) error {
	// polkit >= 124 ships a socket-activated helper and may drop the setuid bit. Prefer the
	// socket when present, like libpolkit-agent does.
	if _, err := os.Stat(helperSocket); err == nil {
		if conn, err := net.Dial("unix", helperSocket); err == nil {
			return runSocketHelper(ctx, conn, id, cookie, conv)
		}
	}
	path, err := helperPath()
	if err != nil {
		return err
	}
	cmd := exec.Command(path, id.Name)
	cmd.Env = []string{}
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var once sync.Once
	kill := func() { once.Do(func() { cmd.Process.Kill() }) }
	stop := context.AfterFunc(ctx, kill)
	defer stop()
	defer cmd.Wait()
	defer kill()

	// The cookie goes on stdin, not argv, so other processes can't read it.
	if _, err := io.WriteString(stdin, cookie+"\n"); err != nil {
		return err
	}
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		line := unescape(sc.Text())
		kind, text, ok := parseLine(line)
		switch {
		case line == "SUCCESS" || strings.HasPrefix(line, "SUCCESS "):
			stdin.Close()
			if err := cmd.Wait(); err != nil {
				return ErrAuthFailed
			}
			return nil
		case line == "FAILURE" || strings.HasPrefix(line, "FAILURE "):
			return ErrAuthFailed
		case !ok:
			continue
		}
		ans, err := conv(ctx, kind, text)
		if err != nil {
			return err
		}
		if kind == PromptSecret || kind == PromptText {
			if strings.ContainsAny(ans, "\n\x00") {
				return errors.New("answer contains a newline or NUL")
			}
			if _, err := io.WriteString(stdin, ans+"\n"); err != nil {
				return err
			}
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return ErrAuthFailed
}

const helperSocket = "/run/polkit/agent-helper.socket"

// runSocketHelper talks to the socket-activated polkit-agent-helper-1. The protocol matches the
// pipe one, except the user name is sent first on the socket instead of argv.
func runSocketHelper(ctx context.Context, conn net.Conn, id Identity, cookie string, conv Conversation) error {
	var once sync.Once
	close := func() { once.Do(func() { conn.Close() }) }
	stop := context.AfterFunc(ctx, close)
	defer stop()
	defer close()

	if _, err := io.WriteString(conn, id.Name+"\n"+cookie+"\n"); err != nil {
		return err
	}
	rd := bufio.NewReader(conn)
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return ErrAuthFailed
		}
		line = unescape(strings.TrimSuffix(line, "\n"))
		kind, text, ok := parseLine(line)
		switch {
		case line == "SUCCESS" || strings.HasPrefix(line, "SUCCESS "):
			return nil
		case line == "FAILURE" || strings.HasPrefix(line, "FAILURE "):
			return ErrAuthFailed
		case !ok:
			continue
		}
		ans, err := conv(ctx, kind, text)
		if err != nil {
			return err
		}
		if kind == PromptSecret || kind == PromptText {
			if strings.ContainsAny(ans, "\n\x00") {
				return errors.New("answer contains a newline or NUL")
			}
			if _, err := io.WriteString(conn, ans+"\n"); err != nil {
				return err
			}
		}
	}
}

func parseLine(line string) (PromptKind, string, bool) {
	tag, text, _ := strings.Cut(line, " ")
	switch tag {
	case "PAM_PROMPT_ECHO_OFF":
		return PromptSecret, text, true
	case "PAM_PROMPT_ECHO_ON":
		return PromptText, text, true
	case "PAM_TEXT_INFO":
		return InfoText, text, true
	case "PAM_ERROR_MSG":
		return ErrorText, text, true
	}
	return 0, "", false
}

// unescape reverses g_strescape, which the helper applies to each message.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'v':
			b.WriteByte('\v')
		case '"', '\\':
			b.WriteByte(s[i])
		case '0', '1', '2', '3', '4', '5', '6', '7':
			j := i
			for j < len(s) && j < i+3 && s[j] >= '0' && s[j] <= '7' {
				j++
			}
			v, _ := strconv.ParseUint(s[i:j], 8, 8)
			b.WriteByte(byte(v))
			i = j - 1
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
