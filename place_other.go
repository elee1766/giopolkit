//go:build !linux || nox11

package main

import "gioui.org/io/event"

func placeWindow(event.Event, int, int) bool { return false }
