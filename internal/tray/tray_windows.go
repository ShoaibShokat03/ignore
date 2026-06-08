//go:build windows

package tray

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmAppTray       = 0x8000 + 44
	wmCommand       = 0x0111
	wmDestroy       = 0x0002
	wmRButtonUp     = 0x0205
	wmLButtonDblClk = 0x0203

	nimAdd     = 0x00000000
	nimDelete  = 0x00000002
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	idiApplication = 32512
	imageIcon      = 1
	lrLoadFromFile = 0x00000010
	lrShared       = 0x00008000

	tpmRetCmd      = 0x0100
	tpmRightButton = 0x0002

	cmdOpen = 1001 + iota
	cmdEnable
	cmdDisable
	cmdReload
	cmdLogs
	cmdExit
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	shell32                 = windows.NewLazySystemDLL("shell32.dll")
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procDestroyIcon         = user32.NewProc("DestroyIcon")
	procLoadImage           = user32.NewProc("LoadImageW")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenu          = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostMessage         = user32.NewProc("PostMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
	procExtractIconEx       = shell32.NewProc("ExtractIconExW")
	procShellNotifyIcon     = shell32.NewProc("Shell_NotifyIconW")
	activeTray              atomic.Pointer[shellTray]
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type point struct {
	X int32
	Y int32
}

type msg struct {
	HWnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type notifyIconData struct {
	Size             uint32
	HWnd             windows.Handle
	ID               uint32
	Flags            uint32
	CallbackMessage  uint32
	Icon             windows.Handle
	Tip              [128]uint16
	State            uint32
	StateMask        uint32
	Info             [256]uint16
	TimeoutOrVersion uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	GuidItem         windows.GUID
	BalloonIcon      windows.Handle
}

type shellTray struct {
	log     *slog.Logger
	actions Actions
	hwnd    windows.Handle
	hicon   windows.Handle
	iconID  uint32
}

func newController(log *slog.Logger, actions Actions) Controller {
	return &shellTray{log: log, actions: actions, iconID: 1}
}

func (t *shellTray) Run(ctx context.Context) {
	activeTray.Store(t)
	hwnd, err := t.createWindow()
	if err != nil {
		t.log.Warn("tray window creation failed", "error", err)
		<-ctx.Done()
		return
	}
	t.hwnd = hwnd
	if err := t.addIcon(); err != nil {
		t.log.Warn("tray icon creation failed", "error", err)
	}
	go func() {
		<-ctx.Done()
		t.Stop()
	}()
	t.log.Info("tray controller started", "mode", "Shell_NotifyIcon")
	var m msg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (t *shellTray) Stop() {
	if t.hwnd != 0 {
		t.deleteIcon()
		procPostMessage.Call(uintptr(t.hwnd), wmDestroy, 0, 0)
		procDestroyWindow.Call(uintptr(t.hwnd))
		t.hwnd = 0
	}
}

func (t *shellTray) createWindow() (windows.Handle, error) {
	className, _ := windows.UTF16PtrFromString("IgnoreTrayWindow")
	hInstance, _, err := procGetModuleHandle.Call(0)
	if hInstance == 0 {
		return 0, err
	}
	wc := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   windows.NewCallback(wndProc),
		Instance:  windows.Handle(hInstance),
		ClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, err := procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, hInstance, 0)
	if hwnd == 0 {
		return 0, err
	}
	return windows.Handle(hwnd), nil
}

func (t *shellTray) addIcon() error {
	hicon := loadTrayIcon()
	t.hicon = windows.Handle(hicon)
	nid := notifyIconData{
		Size:            uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd:            t.hwnd,
		ID:              t.iconID,
		Flags:           nifMessage | nifIcon | nifTip,
		CallbackMessage: wmAppTray,
		Icon:            windows.Handle(hicon),
	}
	copy(nid.Tip[:], windows.StringToUTF16("Ignore"))
	ret, _, err := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	if ret == 0 {
		return err
	}
	return nil
}

func loadTrayIcon() uintptr {
	if hicon := extractAppIconFromExe(); hicon != 0 {
		return hicon
	}
	for _, path := range trayIconCandidates() {
		p, err := windows.UTF16PtrFromString(path)
		if err != nil {
			continue
		}
		hicon, _, _ := procLoadImage.Call(0, uintptr(unsafe.Pointer(p)), imageIcon, 0, 0, lrLoadFromFile)
		if hicon != 0 {
			return hicon
		}
	}
	hicon, _, _ := procLoadImage.Call(0, uintptr(idiApplication), imageIcon, 0, 0, lrShared)
	return hicon
}

func extractAppIconFromExe() uintptr {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return 0
	}
	p, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0
	}
	var small windows.Handle
	count, _, _ := procExtractIconEx.Call(
		uintptr(unsafe.Pointer(p)),
		0,
		0,
		uintptr(unsafe.Pointer(&small)),
		1,
	)
	if count == 0 || small == 0 {
		return 0
	}
	return uintptr(small)
}

func trayIconCandidates() []string {
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	cwd, _ := os.Getwd()
	return []string{
		filepath.Join(exeDir, "icon.ico"),
		filepath.Join(exeDir, "..", "windows", "icon.ico"),
		filepath.Join(cwd, "build", "windows", "icon.ico"),
		filepath.Join(cwd, "assets", "icon.ico"),
	}
}

func (t *shellTray) deleteIcon() {
	nid := notifyIconData{
		Size: uint32(unsafe.Sizeof(notifyIconData{})),
		HWnd: t.hwnd,
		ID:   t.iconID,
	}
	procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	if t.hicon != 0 {
		procDestroyIcon.Call(uintptr(t.hicon))
		t.hicon = 0
	}
}

func (t *shellTray) showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	appendMenu(menu, cmdOpen, "Open Ignore")
	appendMenu(menu, cmdEnable, "Enable Protection")
	appendMenu(menu, cmdDisable, "Disable Protection")
	appendMenu(menu, cmdReload, "Reload Rules")
	appendMenu(menu, cmdLogs, "Open Logs")
	appendMenu(menu, cmdExit, "Exit")
	var p point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	procSetForegroundWindow.Call(uintptr(t.hwnd))
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmRetCmd|tpmRightButton, uintptr(p.X), uintptr(p.Y), 0, uintptr(t.hwnd), 0)
	t.runCommand(uint32(cmd))
}

func (t *shellTray) runCommand(cmd uint32) {
	switch cmd {
	case cmdOpen:
		t.actions.Open()
	case cmdEnable:
		t.actions.Enable()
	case cmdDisable:
		t.actions.Disable()
	case cmdReload:
		t.actions.Reload()
	case cmdLogs:
		t.actions.Logs()
	case cmdExit:
		t.actions.Exit()
	}
}

func appendMenu(menu uintptr, id uint32, label string) {
	p, _ := windows.UTF16PtrFromString(label)
	procAppendMenu.Call(menu, 0, uintptr(id), uintptr(unsafe.Pointer(p)))
}

func wndProc(hwnd windows.Handle, msg uint32, wparam, lparam uintptr) uintptr {
	t := activeTray.Load()
	switch msg {
	case wmAppTray:
		if t != nil {
			switch uint32(lparam) {
			case wmRButtonUp:
				t.showMenu()
				return 0
			case wmLButtonDblClk:
				t.actions.Open()
				return 0
			}
		}
	case wmCommand:
		if t != nil {
			t.runCommand(uint32(wparam & 0xffff))
			return 0
		}
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wparam, lparam)
	return ret
}
