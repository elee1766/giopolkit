package ui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/user"
	"sort"
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

	"github.com/elee1766/giopolkit/pkg/place"
	"github.com/elee1766/giopolkit/pkg/theme"
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

func myUID() int { return os.Getuid() }

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
	left, right := "polkit", v.s.req.ActionID
	if len(rep.Argv) > 0 {
		left, right = "polkit · pkexec", ""
	}
	c := v.pal.Dim
	if v.flagged() {
		c = v.pal.Red
		left = "polkit · flagged"
		var rules []string
		seen := map[string]bool{}
		for _, w := range rep.Warnings {
			if !seen[w.Rule] {
				seen[w.Rule] = true
				rules = append(rules, w.Rule)
			}
		}
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
				l := v.label(Escape(right), c, false)
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
		var keys []string
		for k := range r.Details {
			if !strings.HasPrefix(k, "polkit.") {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
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

// argv lays out the quoted arguments inline, wrapping between arguments, with flagged ones drawn
// in inverse red.
func (v *view_) argv(argv []string, hl map[int]bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		maxW := gtx.Constraints.Max.X
		lc := gtx
		lc.Constraints.Min = image.Point{}
		space := func() int {
			m := op.Record(gtx.Ops)
			d := v.text("x", v.pal.Fg, true)(lc)
			m.Stop()
			return d.Size.X
		}()
		x, y, lineH, width := 0, 0, 0, 0
		for i, a := range argv {
			c := v.pal.Fg
			if hl[i] {
				c = v.pal.Bg
			}
			m := op.Record(gtx.Ops)
			d := v.text(Quote(a), c, true)(lc)
			call := m.Stop()
			if x > 0 && x+space+d.Size.X > maxW {
				y += lineH
				x, lineH = 0, 0
			} else if x > 0 {
				x += space
			}
			t := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
			if hl[i] {
				paint.FillShape(gtx.Ops, v.pal.Red, clip.Rect{Max: d.Size}.Op())
			}
			call.Add(gtx.Ops)
			t.Pop()
			x += d.Size.X
			lineH = max(lineH, d.Size.Y)
			width = max(width, x)
		}
		return layout.Dimensions{Size: image.Pt(width, y+lineH)}
	}
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
		btn := func(c *widget.Clickable, fill, edge, fg color.NRGBA, dashed bool, lbl, hint string, hintBold bool) layout.Widget {
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
								layout.Rigid(v.text(hint, fg, hintBold)),
							)
						})
					})
					return layout.Dimensions{Size: sz}
				})
			}
		}
		okEdge, okFg := v.pal.Fg, v.pal.Fg
		if danger {
			okEdge, okFg = v.pal.Red, v.pal.Red
		}
		if !okEnabled {
			okFg = v.pal.Dim
			if !danger {
				okEdge = v.pal.Dim
			}
		}
		return layout.Flex{}.Layout(gtx,
			layout.Flexed(1, btn(v.deny, v.pal.Fg, v.pal.Fg, v.pal.Bg, false, denyLbl, "esc", true)),
			layout.Rigid(layout.Spacer{Width: 12}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if !okEnabled {
					gtx = gtx.Disabled()
				}
				return btn(v.approve, color.NRGBA{}, okEdge, okFg, !okEnabled, okLbl, okHint, okEnabled)(gtx)
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
