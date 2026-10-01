//go:build linux && !nox11

package place

/*
#cgo LDFLAGS: -lX11 -lXrandr
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <X11/extensions/Xrandr.h>
#include <string.h>

// Moves win to the center of the primary RandR monitor and marks it as an always-on-top dialog.
static void rootpls_place(Display *d, Window win, int w, int h) {
	int n = 0, mx = 0, my = 0, mw = 0, mh = 0;
	XRRMonitorInfo *mons = XRRGetMonitors(d, DefaultRootWindow(d), True, &n);
	for (int i = 0; i < n; i++) {
		if (i == 0 || mons[i].primary) {
			mx = mons[i].x; my = mons[i].y; mw = mons[i].width; mh = mons[i].height;
			if (mons[i].primary) break;
		}
	}
	if (mons) XRRFreeMonitors(mons);
	if (mw > 0 && mh > 0) {
		XSizeHints *hints = XAllocSizeHints();
		long supplied;
		XGetWMNormalHints(d, win, hints, &supplied);
		hints->flags |= USPosition | PPosition;
		hints->x = mx + (mw - w) / 2;
		hints->y = my + (mh - h) / 2;
		XSetWMNormalHints(d, win, hints);
		XFree(hints);
		XMoveWindow(d, win, mx + (mw - w) / 2, my + (mh - h) / 2);
	}

	Atom type = XInternAtom(d, "_NET_WM_WINDOW_TYPE", False);
	Atom dialog = XInternAtom(d, "_NET_WM_WINDOW_TYPE_DIALOG", False);
	XChangeProperty(d, win, type, XA_ATOM, 32, PropModeReplace, (unsigned char *)&dialog, 1);

	XEvent ev;
	memset(&ev, 0, sizeof(ev));
	ev.xclient.type = ClientMessage;
	ev.xclient.window = win;
	ev.xclient.message_type = XInternAtom(d, "_NET_WM_STATE", False);
	ev.xclient.format = 32;
	ev.xclient.data.l[0] = 1; // _NET_WM_STATE_ADD
	ev.xclient.data.l[1] = XInternAtom(d, "_NET_WM_STATE_ABOVE", False);
	ev.xclient.data.l[3] = 1;
	XSendEvent(d, DefaultRootWindow(d), False, SubstructureRedirectMask | SubstructureNotifyMask, &ev);
	XFlush(d);
}
*/
import "C"

import (
	"unsafe"

	"gioui.org/app"
	"gioui.org/io/event"
)

// placeWindow centers an X11 window on the primary monitor. Other platforms are left to the window manager.
func Window(e event.Event, w, h int) bool {
	v, ok := e.(app.X11ViewEvent)
	if !ok || v.Display == nil || v.Window == 0 {
		return false
	}
	C.rootpls_place((*C.Display)(unsafe.Pointer(v.Display)), C.Window(v.Window), C.int(w), C.int(h))
	return true
}
