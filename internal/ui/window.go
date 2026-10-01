package ui

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/elee1766/rootpls/internal/place"
)

// armDelay disables Approve after the window appears so a click or keypress meant for another
// window can't approve.
const armDelay = 1000 * time.Millisecond

var (
	colBg      = rgb(0x1e1f24)
	colSurface = rgb(0x2a2c33)
	colFg      = rgb(0xe6e6e6)
	colDim     = rgb(0x9a9ca5)
	colWarn    = rgb(0xf5a524)
	colErr     = rgb(0xe5484d)
	colDeny    = rgb(0x3a3d46)
	colApprove = rgb(0x2f7d4f)
	colRoot    = rgb(0xe5484d)
	colUser    = rgb(0x3d8bfd)
)

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xff}
}

func myUID() int { return os.Getuid() }

func show(ctx context.Context, s *session) {
	w := new(app.Window)
	w.Option(
		app.Title("Authentication required"),
		app.Size(unit.Dp(680), unit.Dp(520)),
		app.MinSize(unit.Dp(520), unit.Dp(400)),
	)
	s.update(func() { s.invalid = w.Invalidate })
	stop := context.AfterFunc(ctx, func() { w.Perform(system.ActionClose) })
	defer stop()

	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette = material.Palette{Fg: colFg, Bg: colBg, ContrastBg: colApprove, ContrastFg: colFg}
	th.TextSize = 14

	var (
		ops     op.Ops
		approve widget.Clickable
		deny    widget.Clickable
		submit  widget.Clickable
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
			armed := gtx.Now.Sub(opened) >= armDelay

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
			if st == stReview && approve.Clicked(gtx) && armed {
				decide(true)
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
			if submit.Clicked(gtx) && waiting {
				sendAnswer()
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

			paint.Fill(gtx.Ops, colBg)
			v := view_{th: th, s: s, st: st, armed: armed, prompt: prompt, waiting: waiting, info: info, errMsg: errMsg,
				approve: &approve, deny: &deny, submit: &submit, pw: &pw, scroll: &scroll}
			v.layout(gtx)

			if !armed {
				gtx.Execute(op.InvalidateCmd{At: opened.Add(armDelay)})
			}
			e.Frame(gtx.Ops)
		}
	}
}

type view_ struct {
	th                    *material.Theme
	s                     *session
	st                    stage
	armed, waiting        bool
	prompt, errMsg        string
	info                  []string
	approve, deny, submit *widget.Clickable
	pw                    *widget.Editor
	scroll                *widget.List
}

func (v *view_) label(s string, size unit.Sp, c color.NRGBA, bold bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(v.th, size, s)
		l.Color = c
		if bold {
			l.Font.Weight = font.Bold
		}
		return l.Layout(gtx)
	}
}

func (v *view_) mono(s string, c color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(v.th, 13, s)
		l.Font.Typeface = "Go Mono"
		l.Color = c
		return l.Layout(gtx)
	}
}

func spacer(dp unit.Dp) layout.Widget { return layout.Spacer{Height: dp}.Layout }

func (v *view_) layout(gtx layout.Context) layout.Dimensions {
	r := v.s.req
	rep := v.s.report
	isExec := len(rep.Argv) > 0

	target := rep.TargetUser
	if target == "" {
		target = "root"
	}
	targetColor := colUser
	if target == "root" {
		targetColor = colRoot
	}

	// Body: scrollable details.
	var body []layout.Widget
	add := func(ws ...layout.Widget) { body = append(body, ws...) }
	if isExec {
		add(v.label("COMMAND", 11, colDim, true), spacer(4))
		add(func(gtx layout.Context) layout.Dimensions {
			return panel(gtx, targetColor, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(v.mono(QuoteArgv(rep.Argv), colFg)),
					layout.Rigid(layout.Spacer{Height: 8}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if rep.Cwd == "" {
							return layout.Dimensions{}
						}
						return v.mono("cwd  "+Escape(rep.Cwd), colDim)(gtx)
					}),
				)
			})
		}, spacer(12))
	} else {
		add(v.label("ACTION", 11, colDim, true), spacer(4))
		add(v.label(Escape(r.Message), 15, colFg, false), spacer(4))
		add(v.mono(Escape(r.ActionID), colDim), spacer(12))
		var keys []string
		for k := range r.Details {
			if !strings.HasPrefix(k, "polkit.") {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			add(v.label("DETAILS", 11, colDim, true), spacer(4))
			for _, k := range sorted(keys) {
				add(v.mono(Escape(k)+" = "+Escape(r.Details[k]), colFg))
			}
			add(spacer(12))
		}
	}
	if len(rep.Warnings) > 0 {
		add(v.label("WARNINGS", 11, colWarn, true), spacer(4))
		for _, w := range rep.Warnings {
			add(v.label("• "+Escape(w), 13, colWarn, false))
		}
		add(spacer(12))
	}
	if len(rep.Chain) > 0 {
		add(v.label("REQUESTED BY", 11, colDim, true), spacer(4))
		for i, p := range rep.Chain {
			cmd := Escape(p.Cmd)
			if len(cmd) > 140 {
				cmd = cmd[:140] + "…"
			}
			add(v.mono(fmt.Sprintf("%s%s  (pid %d, uid %d)", strings.Repeat("  ", i), cmd, p.PID, p.UID), colDim))
		}
	}

	header := func(gtx layout.Context) layout.Dimensions {
		if isExec {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(v.label("Run as ", 20, colFg, true)),
				layout.Rigid(v.label(Escape(target), 20, targetColor, true)),
			)
		}
		return v.label("Authorization required", 20, colFg, true)(gtx)
	}

	return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(header),
			layout.Rigid(spacer(14)),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.List(v.th, v.scroll).Layout(gtx, len(body), func(gtx layout.Context, i int) layout.Dimensions {
					return body[i](gtx)
				})
			}),
			layout.Rigid(spacer(12)),
			layout.Rigid(v.footer),
		)
	})
}

func (v *view_) footer(gtx layout.Context) layout.Dimensions {
	denyBtn := func(gtx layout.Context) layout.Dimensions {
		label := "Deny"
		if v.st != stReview {
			label = "Cancel"
		}
		b := material.Button(v.th, v.deny, label)
		b.Background = colDeny
		b.TextSize = 16
		b.Font.Weight = font.Bold
		b.CornerRadius = 6
		b.Inset = layout.Inset{Top: 14, Bottom: 14, Left: 40, Right: 40}
		gtx.Constraints.Min.X = gtx.Dp(150)
		return b.Layout(gtx)
	}
	switch v.st {
	case stReview:
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, v.label("Esc deny · Ctrl+Enter approve", 12, colDim, false)),
			layout.Rigid(denyBtn),
			layout.Rigid(layout.Spacer{Width: 12}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				b := material.Button(v.th, v.approve, "Approve")
				b.TextSize = 12
				b.Inset = layout.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}
				if !v.armed {
					gtx = gtx.Disabled()
				}
				return b.Layout(gtx)
			}),
		)
	case stDone:
		return v.label("Authenticated.", 14, colApprove, true)(gtx)
	}
	// stAuth
	status := ""
	statusColor := colDim
	switch {
	case v.errMsg != "":
		status, statusColor = v.errMsg, colErr
	case len(v.info) > 0:
		status, statusColor = strings.Join(v.info, "  "), colWarn
	case !v.waiting:
		status = "Checking…"
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if status == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Bottom: 8}.Layout(gtx, v.label(Escape(status), 13, statusColor, false))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if !v.waiting {
						return layout.Dimensions{}
					}
					return panel(gtx, colApprove, func(gtx layout.Context) layout.Dimensions {
						hint := strings.TrimSpace(v.prompt)
						if hint == "" {
							hint = "Password"
						}
						ed := material.Editor(v.th, v.pw, Escape(hint))
						ed.HintColor = colDim
						return ed.Layout(gtx)
					})
				}),
				layout.Rigid(layout.Spacer{Width: 12}.Layout),
				layout.Rigid(denyBtn),
				layout.Rigid(layout.Spacer{Width: 12}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					b := material.Button(v.th, v.submit, "OK")
					b.TextSize = 12
					b.Inset = layout.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}
					if !v.waiting {
						gtx = gtx.Disabled()
					}
					return b.Layout(gtx)
				}),
			)
		}),
	)
}

// panel draws a surface with a colored left edge, sized to its content.
func panel(gtx layout.Context, edge color.NRGBA, w layout.Widget) layout.Dimensions {
	m := op.Record(gtx.Ops)
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	dims := layout.UniformInset(unit.Dp(12)).Layout(gtx, w)
	call := m.Stop()
	paint.FillShape(gtx.Ops, colSurface, clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(6)).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, edge, clip.Rect{Max: image.Pt(gtx.Dp(3), dims.Size.Y)}.Op())
	call.Add(gtx.Ops)
	return dims
}

func sorted(xs []string) []string {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
	return xs
}
