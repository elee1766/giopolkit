//go:build !linux || nox11

package place

import "gioui.org/io/event"

func Window(event.Event, int, int) bool { return false }
