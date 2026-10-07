package ui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"maps"
	"os/user"
	"slices"
	"strconv"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/elee1766/giopolkit/pkg/ui/place"
	"github.com/elee1766/giopolkit/pkg/ui/theme"
)

// armDelay disables Approve after the window appears so a click or keypress meant for another
// window can't approve. Flagged requests wait longer.
const (
	armDelay        = 1 * time.Second
	armDelayFlagged = 3 * time.Second
)

const (
	fontSize   unit.Sp = 13
	titleSize  unit.Sp = 18
	lineScale          = 1.6
	labelWidth unit.Dp = 64
)

func show(ctx context.Context, s *session, pal theme.Palette) {
	w := new(app.Window)
	w.Option(
		app.Title("Authentication required"),
		app.Size(unit.Dp(560), unit.Dp(600)),
		app.MinSize(unit.Dp(420), unit.Dp(360)),
	)
	s.update(func() { s.invalid = w.Invalidate })
	stop := context.AfterFunc(ctx, func() { w.Perform(system.ActionClose) })
	defer stop()

	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Face = "Go Mono"
	th.Palette = material.Palette{Fg: pal.Fg, Bg: pal.Bg, ContrastBg: pal.Fg, ContrastFg: pal.Bg}
	th.TextSize = fontSize

	delay := armDelay
	if len(s.report.Warnings) > 0 {
		delay = armDelayFlagged
	}

	var (
		ops     op.Ops
		approve widget.Clickable
		deny    widget.Clickable
		pw      = widget.Editor{SingleLine: true, Submit: true, Mask: '•'}
		scroll  = widget.List{List: layout.List{Axis: layout.Vertical}}
		view    event.Event
		placed  bool
		opened  = time.Now()
		decided bool
		focusPW bool
	)
	decide := func(ok bool) {
		if decided {
			return
		}
		decided = true
		s.decision <- ok
		if !ok {
			w.Perform(system.ActionClose)
		}
	}
	sendAnswer := func() {
		a := pw.Text()
		pw.SetText("")
		select {
		case s.answer <- a:
		default:
		}
	}

	for {
		switch e := w.Event().(type) {
		case app.X11ViewEvent:
			view = e
		case app.DestroyEvent:
			s.update(func() { s.invalid = nil })
			if !decided {
				decided = true
				s.decision <- false
			}
			pw.SetText("")
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			if !placed {
				placed = true
				if !place.Window(view, e.Size.X, e.Size.Y) {
					w.Perform(system.ActionCenter)
				}
				w.Perform(system.ActionRaise)
			}
			s.mu.Lock()
			st, prompt, secret, waiting, info, errMsg := s.stage, s.prompt, s.secret, s.waiting, append([]string(nil), s.info...), s.errMsg
			s.mu.Unlock()
			remaining := delay - gtx.Now.Sub(opened)
			armed := remaining <= 0

			for {
				ev, ok := gtx.Event(
					key.Filter{Name: key.NameEscape},
					key.Filter{Name: key.NameReturn, Required: key.ModShortcut},
					key.Filter{Name: key.NameEnter, Required: key.ModShortcut},
				)
				if !ok {
					break
				}
				ke, ok := ev.(key.Event)
				if !ok || ke.State != key.Press {
					continue
				}
				if ke.Name == key.NameEscape {
					if st == stReview {
						decide(false)
					} else {
						w.Perform(system.ActionClose)
					}
				} else if st == stReview && armed {
					decide(true)
				}
			}
			if deny.Clicked(gtx) {
				if st == stReview {
					decide(false)
				} else {
					w.Perform(system.ActionClose)
				}
			}
			if approve.Clicked(gtx) {
				switch {
				case st == stReview && armed:
					decide(true)
				case st == stAuth && waiting:
					sendAnswer()
				}
			}
			for {
				ev, ok := pw.Update(gtx)
				if !ok {
					break
				}
				if _, ok := ev.(widget.SubmitEvent); ok && waiting {
					sendAnswer()
				}
			}
			pw.Mask = 0
			if secret {
				pw.Mask = '•'
			}
			if st == stAuth && waiting && !focusPW {
				focusPW = true
				gtx.Execute(key.FocusCmd{Tag: &pw})
			}
			if !waiting {
				focusPW = false
			}

			v := view_{th: th, pal: pal, s: s, st: st, armed: armed, remaining: remaining,
				prompt: prompt, waiting: waiting, info: info, errMsg: errMsg,
				approve: &approve, deny: &deny, pw: &pw, scroll: &scroll}
			v.layout(gtx)

			if !armed {
				// Redraw when the countdown changes.
				gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(remaining % time.Second)})
			}
			e.Frame(gtx.Ops)
		}
	}
}

type view_ struct {
	th             *material.Theme
	pal            theme.Palette
	s              *session
	st             stage
	armed, waiting bool
	remaining      time.Duration
	prompt, errMsg string
	info           []string
	approve, deny  *widget.Clickable
	pw             *widget.Editor
	scroll         *widget.List
}

func (v *view_) flagged() bool { return len(v.s.report.Warnings) > 0 }

// edge is the color of the window border and header: red when anything was flagged.
func (v *view_) edge() color.NRGBA {
	if v.flagged() {
		return v.pal.Red
	}
	return v.pal.Line
}

func (v *view_) label(s string, c color.NRGBA, bold bool) material.LabelStyle {
	l := material.Label(v.th, fontSize, s)
	l.Color = c
	l.LineHeightScale = lineScale
	if bold {
		l.Font.Weight = font.Bold
	}
	return l
}

func (v *view_) text(s string, c color.NRGBA, bold bool) layout.Widget {
	// Gio applies LineHeightScale only between wrapped lines, so pad single lines to match.
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 2, Bottom: 2}.Layout(gtx, v.label(s, c, bold).Layout)
	}
}

// row is one line of a two or three column grid: a fixed-width label, a value, and an optional
// right-aligned trailer.
func (v *view_) row(lbl string, lblColor color.NRGBA, value layout.Widget, trailer layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(labelWidth)
				gtx.Constraints.Max.X = gtx.Constraints.Min.X
				return v.text(lbl, lblColor, false)(gtx)
			}),
			layout.Flexed(1, value),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if trailer == nil {
					return layout.Dimensions{}
				}
				return layout.Inset{Left: 12}.Layout(gtx, trailer)
			}),
		)
	}
}

func (v *view_) layout(gtx layout.Context) layout.Dimensions {
	paint.Fill(gtx.Ops, v.pal.Bg)
	size := gtx.Constraints.Max
	border(gtx, image.Rectangle{Max: size}, v.edge(), false)
	return layout.UniformInset(1).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(v.header),
			layout.Flexed(1, v.body),
			layout.Rigid(v.footer),
		)
	})
}

func (v *view_) header(gtx layout.Context) layout.Dimensions {
	rep := v.s.report
	left, right := "polkit", Escape(v.s.req.ActionID)
	if len(rep.Argv) > 0 && len(rep.Chain) > 0 {
		to := "?"
		if rep.TargetUID >= 0 {
			to = strconv.Itoa(rep.TargetUID)
		}
		right = fmt.Sprintf("uid %d → %s", rep.Chain[0].UID, to)
	}
	c := v.pal.Dim
	if v.flagged() {
		c = v.pal.Red
		left = "polkit · flagged"
		var rules []string
		for _, w := range rep.Warnings {
			rules = append(rules, w.Rule)
		}
		rules = slices.Compact(rules) // findings from one rule are adjacent
		right = "rule " + rules[0]
		if len(rules) > 1 {
			right += fmt.Sprintf(" +%d", len(rules)-1)
		}
	}
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	dims := layout.Inset{Top: 8, Bottom: 8, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
			layout.Rigid(v.text(left, c, false)),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				l := v.label(right, c, false)
				l.Alignment = text.End
				l.MaxLines = 1
				return layout.Inset{Left: 16}.Layout(gtx, l.Layout)
			}),
		)
	})
	paint.FillShape(gtx.Ops, v.edge(), clip.Rect{Min: image.Pt(0, dims.Size.Y-gtx.Dp(1)), Max: dims.Size}.Op())
	return dims
}

func (v *view_) body(gtx layout.Context) layout.Dimensions {
	r := v.s.req
	rep := v.s.report
	isExec := len(rep.Argv) > 0

	var groups []layout.Widget
	if isExec {
		target := rep.TargetUser
		if target == "" {
			target = "root"
		}
		tc := v.pal.Green
		if target == "root" {
			tc = v.pal.Red
		}
		groups = append(groups, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Baseline}.Layout(gtx,
				layout.Rigid(v.title("run as ", v.pal.Fg)),
				layout.Rigid(v.title(Escape(target), tc)),
			)
		})
	} else {
		groups = append(groups, v.title("authorize", v.pal.Fg))
	}

	if len(rep.Warnings) > 0 {
		var rows []layout.FlexChild
		for _, w := range rep.Warnings {
			rows = append(rows, layout.Rigid(v.row("!!", v.pal.Red, v.text(Escape(w.Text), v.pal.Red, false), nil)))
		}
		groups = append(groups, vstack(rows...))
	}

	var facts []layout.FlexChild
	if isExec {
		hl := map[int]bool{}
		for _, w := range rep.Warnings {
			if w.Arg >= 0 {
				hl[w.Arg] = true
			}
		}
		facts = append(facts, layout.Rigid(v.row("cmd", v.pal.Dim, v.argv(rep.Argv, hl), nil)))
		if rep.Cwd != "" {
			facts = append(facts, layout.Rigid(v.row("cwd", v.pal.Dim, v.text(Escape(rep.Cwd), v.pal.Fg, false), nil)))
		}
	} else {
		facts = append(facts,
			layout.Rigid(v.row("action", v.pal.Dim, v.text(Escape(r.ActionID), v.pal.Fg, true), nil)),
			layout.Rigid(v.row("msg", v.pal.Dim, v.text(Escape(r.Message), v.pal.Fg, false), nil)),
		)
		for _, k := range slices.Sorted(maps.Keys(r.Details)) {
			if strings.HasPrefix(k, "polkit.") {
				continue
			}
			facts = append(facts, layout.Rigid(v.row("detail", v.pal.Dim, v.text(Escape(k)+" = "+Escape(r.Details[k]), v.pal.Fg, false), nil)))
		}
	}
	if len(rep.Chain) > 0 {
		facts = append(facts, layout.Rigid(v.row("user", v.pal.Dim, v.text(userName(rep.Chain[0].UID), v.pal.Fg, false), nil)))
	}
	groups = append(groups, vstack(facts...))

	if len(rep.Chain) > 0 {
		var rows []layout.FlexChild
		for i, p := range rep.Chain {
			lbl := ""
			if i == 0 {
				lbl = "from"
			}
			cmd := Escape(p.Cmd)
			if len(cmd) > 140 {
				cmd = cmd[:140] + "..."
			}
			prefix := ""
			if i > 0 {
				prefix = strings.Repeat("   ", i-1) + "└─ "
			}
			value := func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Start}.Layout(gtx,
					layout.Rigid(v.text(prefix, v.pal.Dim, false)),
					layout.Flexed(1, v.text(cmd, v.pal.Fg, false)),
				)
			}
			pid := strconv.Itoa(p.PID)
			if p.UID != rep.Chain[0].UID {
				pid = userName(p.UID) + " " + pid
			}
			rows = append(rows, layout.Rigid(v.row(lbl, v.pal.Dim, value, v.text(pid, v.pal.Dim, false))))
		}
		groups = append(groups, vstack(rows...))
	}

	return layout.Inset{Top: 20, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(v.th, v.scroll).Layout(gtx, len(groups), func(gtx layout.Context, i int) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			in := layout.Inset{Bottom: 18}
			return in.Layout(gtx, groups[i])
		})
	})
}

func (v *view_) title(s string, c color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(v.th, titleSize, s)
		l.Color = c
		l.Font.Weight = font.Bold
		l.LineHeightScale = 1.3
		return l.Layout(gtx)
	}
}

// cmdLine is one logical line of a command: the argv indices on it and its indent in characters.
type cmdLine struct {
	indent int
	args   []int
}

// splitCommand breaks argv into lines the way a person would write it with backslashes: options
// each start a line, an option without "=" keeps the value after it, and a positional argument
// after a self-contained option starts a nested command, indented one level deeper.
func splitCommand(argv []string) []cmdLine {
	if len(argv) == 0 {
		return nil
	}
	lines := []cmdLine{{indent: 0, args: []int{0}}}
	level := 0
	takesValue := false // last arg was an option that may take the next arg as its value
	for i := 1; i < len(argv); i++ {
		a := argv[i]
		cur := &lines[len(lines)-1]
		isOpt := strings.HasPrefix(a, "-") && a != "-"
		switch {
		case isOpt && len(lines) == 1 && len(cur.args) == 1:
			cur.args = append(cur.args, i) // first option stays with the program
		case isOpt:
			lines = append(lines, cmdLine{indent: 2*level + 2, args: []int{i}})
		case takesValue || !strings.HasPrefix(argv[cur.args[len(cur.args)-1]], "-"):
			cur.args = append(cur.args, i)
		default:
			level++
			lines = append(lines, cmdLine{indent: 2 * level, args: []int{i}})
		}
		takesValue = isOpt && !strings.Contains(a, "=")
		if !isOpt {
			takesValue = false
		}
	}
	return lines
}

type argTok struct {
	text string
	hl   bool
}

// argv lays out the command. Short commands flow on wrapped lines. Commands that don't fit and
// split into several logical lines are shown numbered, one option per line. Flagged arguments are
// drawn in inverse red.
func (v *view_) argv(argv []string, hl map[int]bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		maxW := gtx.Constraints.Max.X
		lc := gtx
		lc.Constraints.Min = image.Point{}
		measure := func(s string) image.Point {
			m := op.Record(gtx.Ops)
			d := v.text(s, v.pal.Fg, true)(lc)
			m.Stop()
			return d.Size
		}
		ch := measure("x").X
		tok := func(i int) argTok { return argTok{text: Quote(argv[i]), hl: hl[i]} }

		lines := splitCommand(argv)
		all := make([]argTok, len(argv))
		for i := range argv {
			all[i] = tok(i)
		}
		if len(lines) < 2 || v.flowWidth(gtx, all, ch) <= maxW {
			h, w := v.flow(gtx, all, 0, 0, 0, maxW, ch)
			return layout.Dimensions{Size: image.Pt(w, h)}
		}

		// Numbered layout: a line-number gutter with a rule on its left, as in an editor.
		numW, gap := gtx.Dp(20), gtx.Dp(10)
		textX := numW + gap
		y, width := 0, 0
		for n, ln := range lines {
			toks := make([]argTok, 0, len(ln.args)+1)
			for _, i := range ln.args {
				toks = append(toks, tok(i))
			}
			if n < len(lines)-1 {
				toks = append(toks, argTok{text: "\\"})
			}
			num := strconv.Itoa(n + 1)
			m := op.Record(gtx.Ops)
			nd := v.text(num, v.pal.Dim, false)(lc)
			call := m.Stop()
			t := op.Offset(image.Pt(numW-nd.Size.X, y)).Push(gtx.Ops)
			call.Add(gtx.Ops)
			t.Pop()
			h, w := v.flow(gtx, toks, textX, y, ln.indent*ch, maxW-textX, ch)
			y += h
			width = max(width, textX+w)
		}
		rule := gtx.Dp(11)
		paint.FillShape(gtx.Ops, v.pal.Line, clip.Rect{Min: image.Pt(-rule, 0), Max: image.Pt(-rule+gtx.Dp(1), y)}.Op())
		return layout.Dimensions{Size: image.Pt(width, y)}
	}
}

func (v *view_) flowWidth(gtx layout.Context, toks []argTok, ch int) int {
	w := 0
	for i, t := range toks {
		if i > 0 {
			w += ch
		}
		w += ch * len([]rune(t.text))
	}
	return w
}

// flow draws tokens separated by spaces starting at (x0+indent, y0), wrapping at maxW. Wrapped
// lines are indented two characters past indent. Tokens wider than a line are broken anywhere.
// It returns the height used and the widest line.
func (v *view_) flow(gtx layout.Context, toks []argTok, x0, y0, indent, maxW, ch int) (int, int) {
	lc := gtx
	lc.Constraints.Min = image.Point{}
	lc.Constraints.Max.X = max(maxW-indent, ch*8)
	x, y, lineH, width := indent, 0, 0, 0
	for i, t := range toks {
		fg := v.pal.Fg
		if t.hl {
			fg = v.pal.Bg
		}
		lw := lc
		if x > indent {
			lw.Constraints.Max.X = max(maxW-indent-2*ch, ch*8)
		}
		m := op.Record(gtx.Ops)
		d := v.label(t.text, fg, true).Layout(lw)
		call := m.Stop()
		if i > 0 {
			if x+ch+d.Size.X > maxW {
				y += lineH
				x, lineH = indent+2*ch, 0
			} else {
				x += ch
			}
		}
		tr := op.Offset(image.Pt(x0+x, y0+y+gtx.Dp(2))).Push(gtx.Ops)
		if t.hl {
			paint.FillShape(gtx.Ops, v.pal.Red, clip.Rect{Max: d.Size}.Op())
		}
		call.Add(gtx.Ops)
		tr.Pop()
		x += d.Size.X
		lineH = max(lineH, d.Size.Y+gtx.Dp(4))
		width = max(width, x)
	}
	return y + lineH, width
}

func (v *view_) footer(gtx layout.Context) layout.Dimensions {
	in := layout.Inset{Left: 16, Right: 16, Bottom: 16}
	switch v.st {
	case stDone:
		return in.Layout(gtx, v.text("authenticated", v.pal.Green, true))
	case stAuth:
		return in.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return vstack(
				layout.Rigid(v.authPrompt),
				layout.Rigid(layout.Spacer{Height: 12}.Layout),
				layout.Rigid(v.buttons("cancel", "submit", "enter", v.waiting, false)),
			)(gtx)
		})
	}
	lbl, hint := "approve", "ctrl+enter"
	if v.flagged() {
		lbl = "approve anyway"
	}
	if !v.armed {
		hint = fmt.Sprintf("wait %d", int((v.remaining+time.Second-1)/time.Second))
	}
	return in.Layout(gtx, v.buttons("deny", lbl, hint, v.armed, v.flagged()))
}

func (v *view_) authPrompt(gtx layout.Context) layout.Dimensions {
	var status string
	sc := v.pal.Dim
	switch {
	case v.errMsg != "":
		status, sc = v.errMsg, v.pal.Red
	case len(v.info) > 0:
		status, sc = strings.Join(v.info, "  "), v.pal.Fg
	case !v.waiting:
		status = "checking..."
	}
	return vstack(
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if status == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Bottom: 8}.Layout(gtx, v.text(Escape(status), sc, false))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !v.waiting {
				return layout.Dimensions{}
			}
			hint := strings.TrimSpace(v.prompt)
			if hint == "" {
				hint = "password"
			}
			bc := v.pal.Fg
			if v.errMsg != "" {
				bc = v.pal.Red
			}
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			m := op.Record(gtx.Ops)
			dims := layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(v.text("> ", v.pal.Dim, false)),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						ed := material.Editor(v.th, v.pw, Escape(hint))
						ed.HintColor = v.pal.Dim
						ed.SelectionColor = withAlpha(v.pal.Fg, 0x40)
						ed.LineHeightScale = lineScale
						return ed.Layout(gtx)
					}),
				)
			})
			call := m.Stop()
			border(gtx, image.Rectangle{Max: dims.Size}, bc, false)
			call.Add(gtx.Ops)
			return dims
		}),
	)(gtx)
}

// buttons lays out the two equal-width footer buttons: a filled deny/cancel and an outlined
// approve/submit. The outline is dashed while the button is disabled.
func (v *view_) buttons(denyLbl, okLbl, okHint string, okEnabled, danger bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		h := gtx.Dp(44)
		btn := func(c *widget.Clickable, fill, edge, fg, hintFg color.NRGBA, dashed bool, lbl, hint string, hintBold bool) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, h)
				gtx.Constraints.Max.Y = h
				return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					sz := gtx.Constraints.Min
					rect := image.Rectangle{Max: sz}
					if fill.A != 0 {
						paint.FillShape(gtx.Ops, fill, clip.Rect(rect).Op())
					}
					border(gtx, rect, edge, dashed)
					if gtx.Enabled() {
						pointer.CursorPointer.Add(gtx.Ops)
					}
					layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							return layout.Flex{Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(v.text(lbl, fg, true)),
								layout.Rigid(v.text(hint, hintFg, hintBold)),
							)
						})
					})
					return layout.Dimensions{Size: sz}
				})
			}
		}
		// Approve: green on a quiet border, red when flagged, dim with a dashed border while disabled.
		okEdge, okFg, hintFg, hintBold := v.pal.Line, v.pal.Green, v.pal.Dim, false
		switch {
		case danger && okEnabled:
			okEdge, okFg, hintFg, hintBold = v.pal.Red, v.pal.Red, v.pal.Red, true
		case danger:
			okEdge, okFg, hintFg = v.pal.Red, v.pal.Dim, v.pal.Dim
		case !okEnabled:
			okEdge, okFg, hintFg = v.pal.Dim, v.pal.Dim, v.pal.Dim
		}
		return layout.Flex{}.Layout(gtx,
			layout.Flexed(1, btn(v.deny, v.pal.Fg, v.pal.Fg, v.pal.Bg, v.pal.Bg, false, denyLbl, "esc", true)),
			layout.Rigid(layout.Spacer{Width: 12}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if !okEnabled {
					gtx = gtx.Disabled()
				}
				return btn(v.approve, color.NRGBA{}, okEdge, okFg, hintFg, !okEnabled, okLbl, okHint, hintBold)(gtx)
			}),
		)
	}
}

// border strokes a 1dp rectangle inside r, solid or dashed.
func border(gtx layout.Context, r image.Rectangle, c color.NRGBA, dashed bool) {
	t := gtx.Dp(1)
	edges := []image.Rectangle{
		{Min: r.Min, Max: image.Pt(r.Max.X, r.Min.Y+t)},
		{Min: image.Pt(r.Min.X, r.Max.Y-t), Max: r.Max},
		{Min: r.Min, Max: image.Pt(r.Min.X+t, r.Max.Y)},
		{Min: image.Pt(r.Max.X-t, r.Min.Y), Max: r.Max},
	}
	if !dashed {
		for _, e := range edges {
			paint.FillShape(gtx.Ops, c, clip.Rect(e).Op())
		}
		return
	}
	dash, gap := gtx.Dp(3), gtx.Dp(3)
	for i, e := range edges {
		horiz := i < 2
		start, end := e.Min.Y, e.Max.Y
		if horiz {
			start, end = e.Min.X, e.Max.X
		}
		for p := start; p < end; p += dash + gap {
			q := min(p+dash, end)
			d := e
			if horiz {
				d.Min.X, d.Max.X = p, q
			} else {
				d.Min.Y, d.Max.Y = p, q
			}
			paint.FillShape(gtx.Ops, c, clip.Rect(d).Op())
		}
	}
}

func vstack(children ...layout.FlexChild) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

func userName(uid int) string {
	if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
		return Escape(u.Username)
	}
	return "uid " + strconv.Itoa(uid)
}
