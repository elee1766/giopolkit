package main

import (
	"fmt"
	"image"
	"image/color"
	"os"
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
)

// armDelay blocks approval right after the window appears so a stray click or keypress can't approve.
const armDelay = 800 * time.Millisecond

var (
	colBg      = rgb(0x1e1f24)
	colSurface = rgb(0x2a2c33)
	colFg      = rgb(0xe6e6e6)
	colDim     = rgb(0x9a9ca5)
	colDeny    = rgb(0x3a3d46)
	colApprove = rgb(0x2f7d4f)
)

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xff}
}

func riskStyle(risk string) (string, color.NRGBA) {
	switch risk {
	case "low":
		return "LOW RISK", rgb(0x3d8bfd)
	case "high":
		return "HIGH RISK", rgb(0xe5484d)
	default:
		return "MEDIUM RISK", rgb(0xf5a524)
	}
}

// confirm shows the dialog and blocks until the user decides. Any non-approval is a denial.
func confirm(req request) bool {
	result := make(chan bool, 1)
	go func() {
		result <- dialog(req)
		os.Stdout.Sync()
	}()
	go app.Main()
	return <-result
}

func dialog(req request) bool {
	w := new(app.Window)
	w.Option(
		app.Title("rootpls: root access request"),
		app.Size(unit.Dp(560), unit.Dp(400)),
		app.MinSize(unit.Dp(420), unit.Dp(320)),
	)

	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette = material.Palette{Fg: colFg, Bg: colBg, ContrastBg: colApprove, ContrastFg: colFg}
	th.TextSize = 14

	var (
		ops      op.Ops
		approve  widget.Clickable
		deny     widget.Clickable
		cmdText  widget.Selectable
		scroll   = widget.List{List: layout.List{Axis: layout.Vertical}}
		opened   = time.Now()
		deadline = opened.Add(req.Timeout)
		decided  *bool
		view     event.Event
		placed   bool
	)
	cmdText.SetText(req.Display())
	riskLabel, riskColor := riskStyle(req.Risk)
	decide := func(v bool) {
		if decided == nil {
			decided = &v
			w.Perform(system.ActionClose)
		}
	}

	for {
		switch e := w.Event().(type) {
		case app.X11ViewEvent:
			view = e
		case app.DestroyEvent:
			if e.Err != nil {
				fmt.Fprintln(os.Stderr, "rootpls: dialog error:", e.Err)
			}
			return decided != nil && *decided
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			if !placed {
				placed = true
				if !placeWindow(view, e.Size.X, e.Size.Y) {
					w.Perform(system.ActionCenter)
				}
				w.Perform(system.ActionRaise)
			}
			now := gtx.Now
			armed := now.Sub(opened) >= armDelay
			if now.After(deadline) {
				decide(false)
			}

			for {
				ev, ok := gtx.Event(
					key.Filter{Name: key.NameEscape},
					key.Filter{Name: key.NameReturn, Required: key.ModShortcut},
					key.Filter{Name: key.NameEnter, Required: key.ModShortcut},
				)
				if !ok {
					break
				}
				if ke, ok := ev.(key.Event); ok && ke.State == key.Press {
					if ke.Name == key.NameEscape {
						decide(false)
					} else if armed {
						decide(true)
					}
				}
			}
			if deny.Clicked(gtx) {
				decide(false)
			}
			if approve.Clicked(gtx) && armed {
				decide(true)
			}

			paint.Fill(gtx.Ops, colBg)

			layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								l := material.Label(th, 18, "An agent wants to run a command as root")
								l.Font.Weight = font.Bold
								return l.Layout(gtx)
							}),
							layout.Flexed(1, layout.Spacer{}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return pill(gtx, th, riskLabel, riskColor)
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(caption(th, "REASON")),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(material.Body1(th, req.Reason).Layout),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(caption(th, "COMMAND")),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return panel(gtx, riskColor, func(gtx layout.Context) layout.Dimensions {
							return material.List(th, &scroll).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
								return cmdText.Layout(gtx, th.Shaper, font.Font{Typeface: "Go Mono"}, 14,
									colorOp(colFg), colorOp(rgb(0x44475a)))
							})
						})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Caption(th, "in "+req.Dir)
						l.Color = colDim
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						left := deadline.Sub(now).Round(time.Second)
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								l := material.Caption(th, fmt.Sprintf("Esc deny · Ctrl+Enter approve · auto-deny in %s", left))
								l.Color = colDim
								return l.Layout(gtx)
							}),
							layout.Flexed(1, layout.Spacer{}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								b := material.Button(th, &deny, "Deny")
								b.Background = colDeny
								b.TextSize = 16
								b.Font.Weight = font.Bold
								b.CornerRadius = 6
								b.Inset = layout.Inset{Top: 14, Bottom: 14, Left: 40, Right: 40}
								gtx.Constraints.Min.X = gtx.Dp(150)
								return b.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								b := material.Button(th, &approve, "Approve")
								b.TextSize = 12
								b.Inset = layout.Inset{Top: 6, Bottom: 6, Left: 10, Right: 10}
								if !armed {
									gtx = gtx.Disabled()
								}
								return b.Layout(gtx)
							}),
						)
					}),
				)
			})

			// Redraw for the countdown and to enable Approve once armed.
			next := now.Add(time.Second)
			if !armed {
				next = opened.Add(armDelay)
			}
			gtx.Execute(op.InvalidateCmd{At: next})
			e.Frame(gtx.Ops)
		}
	}
}

func colorOp(c color.NRGBA) op.CallOp {
	var m op.Ops
	rec := op.Record(&m)
	paint.ColorOp{Color: c}.Add(&m)
	return rec.Stop()
}

func caption(th *material.Theme, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Caption(th, s)
		l.Color = colDim
		l.Font.Weight = font.Bold
		return l.Layout(gtx)
	}
}

func pill(gtx layout.Context, th *material.Theme, s string, c color.NRGBA) layout.Dimensions {
	m := op.Record(gtx.Ops)
	dims := layout.Inset{Top: 4, Bottom: 4, Left: 10, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		l := material.Caption(th, s)
		l.Color = rgb(0x111111)
		l.Font.Weight = font.Bold
		return l.Layout(gtx)
	})
	call := m.Stop()
	r := gtx.Dp(12)
	paint.FillShape(gtx.Ops, c, clip.UniformRRect(image.Rectangle{Max: dims.Size}, r).Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

// panel draws a surface with a colored left edge in the risk color.
func panel(gtx layout.Context, edge color.NRGBA, w layout.Widget) layout.Dimensions {
	size := gtx.Constraints.Max
	r := gtx.Dp(6)
	paint.FillShape(gtx.Ops, colSurface, clip.UniformRRect(image.Rectangle{Max: size}, r).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, edge, clip.Rect{Max: image.Pt(gtx.Dp(3), size.Y)}.Op())
	gtx.Constraints.Min = size
	layout.UniformInset(unit.Dp(12)).Layout(gtx, w)
	return layout.Dimensions{Size: size}
}
