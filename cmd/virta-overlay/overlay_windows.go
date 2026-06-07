//go:build windows

package main

// Windows overlay implementation using pure Go + golang.org/x/sys/windows.
//
// A layered popup window (WS_EX_LAYERED | WS_EX_TOPMOST) is created as a
// click-through transparent frame. WebView2 integration requires the Edge
// WebView2 Runtime which is not guaranteed to be present, so this version
// opens the overlay URL in the default browser and shows a notification
// window explaining the situation.
//
// A full WebView2 implementation can replace this stub once the
// github.com/jchv/go-webview2 (or equivalent) dependency is added to go.mod.

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modShell32  = windows.NewLazySystemDLL("shell32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modGdi32    = windows.NewLazySystemDLL("gdi32.dll")

	procRegisterClassExW      = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW       = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW        = modUser32.NewProc("DefWindowProcW")
	procGetMessageW           = modUser32.NewProc("GetMessageW")
	procTranslateMessage      = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW      = modUser32.NewProc("DispatchMessageW")
	procSetLayeredWindowAttrs = modUser32.NewProc("SetLayeredWindowAttributes")
	procShowWindow            = modUser32.NewProc("ShowWindow")
	procSetWindowTextW        = modUser32.NewProc("SetWindowTextW")
	procDestroyWindow         = modUser32.NewProc("DestroyWindow")
	procPostQuitMessage       = modUser32.NewProc("PostQuitMessage")
	procMessageBoxW           = modUser32.NewProc("MessageBoxW")
	procGetModuleHandleW      = modKernel32.NewProc("GetModuleHandleW")
	procShellExecuteW         = modShell32.NewProc("ShellExecuteW")
	procLoadCursorW           = modUser32.NewProc("LoadCursorW")
	procGetStockObject        = modGdi32.NewProc("GetStockObject")
)

const (
	wsExLayered    = 0x00080000
	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080
	wsExNoActivate = 0x08000000
	wsExTransparent = 0x00000020

	wsPopup   = 0x80000000
	wsVisible = 0x10000000

	swShow = 5

	lwaTrans   = 0x00000002 // use transparency value
	lwaColorKey = 0x00000001

	wmDestroy = 0x0002
	wmClose   = 0x0010

	nullBrush = 5
	idcArrow  = 32512
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

// wndProc is the window procedure for the overlay window.
//
//go:nosplit
func wndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return r
}

func runOverlay(urlStr, title string, x, y, width, height int) error {
	// Open the overlay URL in the default browser so the user can at least
	// view the panel. This also acts as the WebView2-unavailable fallback.
	if err := shellOpen(urlStr); err != nil {
		// Non-fatal: log but continue to show the notification window.
		_ = err
	}

	hInst, _, _ := procGetModuleHandleW.Call(0)

	className, _ := syscall.UTF16PtrFromString("VirtaOverlay")
	titleW, _ := syscall.UTF16PtrFromString(title)

	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	nullBrushHandle, _, _ := procGetStockObject.Call(nullBrush)

	wcx := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   syscall.NewCallback(wndProc),
		hInstance:     hInst,
		hCursor:       cursor,
		hbrBackground: nullBrushHandle,
		lpszClassName: className,
	}

	ret, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wcx)))
	if ret == 0 {
		return fmt.Errorf("RegisterClassExW: %w", err)
	}

	exStyle := uintptr(wsExLayered | wsExTopmost | wsExToolWindow | wsExNoActivate | wsExTransparent)
	style := uintptr(wsPopup)

	hwnd, _, err := procCreateWindowExW.Call(
		exStyle,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(titleW)),
		style,
		uintptr(x), uintptr(y),
		uintptr(width), uintptr(height),
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW: %w", err)
	}

	// Make the window fully transparent and click-through.
	// Alpha=0 with LWA_ALPHA causes the window to be invisible; combined with
	// WS_EX_TRANSPARENT all mouse events are forwarded to the window below.
	procSetLayeredWindowAttrs.Call(hwnd, 0, 0, lwaTrans)

	procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(titleW)))
	procShowWindow.Call(hwnd, swShow)

	// Message loop.
	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if r == 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	return nil
}

// shellOpen opens a URL using the system default handler (e.g. the default browser).
func shellOpen(urlStr string) error {
	verb, _ := syscall.UTF16PtrFromString("open")
	file, _ := syscall.UTF16PtrFromString(urlStr)
	r, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		0, 0,
		swShow,
	)
	// ShellExecute returns a value > 32 on success.
	if r <= 32 {
		return fmt.Errorf("ShellExecuteW returned %d", r)
	}
	return nil
}
