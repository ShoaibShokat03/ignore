//go:build windows

package clipboard

import (
	"encoding/binary"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	cfHDrop      = 15
	gmemMoveable = 0x0002
	gmemZeroInit = 0x0040
)

var (
	user32                      = windows.NewLazySystemDLL("user32.dll")
	shell32                     = windows.NewLazySystemDLL("shell32.dll")
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	procOpenClipboard           = user32.NewProc("OpenClipboard")
	procCloseClipboard          = user32.NewProc("CloseClipboard")
	procIsClipboardFormatAvail  = user32.NewProc("IsClipboardFormatAvailable")
	procGetClipboardData        = user32.NewProc("GetClipboardData")
	procEmptyClipboard          = user32.NewProc("EmptyClipboard")
	procSetClipboardData        = user32.NewProc("SetClipboardData")
	procGetClipboardSequenceNum = user32.NewProc("GetClipboardSequenceNumber")
	procDragQueryFile           = shell32.NewProc("DragQueryFileW")
	procGlobalAlloc             = kernel32.NewProc("GlobalAlloc")
	procGlobalLock              = kernel32.NewProc("GlobalLock")
	procGlobalUnlock            = kernel32.NewProc("GlobalUnlock")
	procGlobalFree              = kernel32.NewProc("GlobalFree")
	procRegisterClipboardFormat = user32.NewProc("RegisterClipboardFormatW")
)

type dropPoint struct {
	X int32
	Y int32
}

type dropFiles struct {
	PFiles uint32
	Pt     dropPoint
	FNC    int32
	FWide  int32
}

func readFiles() (FileList, error) {
	sequence := currentSequence()
	if sequence == 0 {
		return FileList{}, nil
	}
	if ok, _, _ := procIsClipboardFormatAvail.Call(cfHDrop); ok == 0 {
		return FileList{Sequence: sequence}, nil
	}
	if ok, _, err := procOpenClipboard.Call(0); ok == 0 {
		return FileList{}, err
	}
	defer procCloseClipboard.Call()
	handle, _, err := procGetClipboardData.Call(cfHDrop)
	if handle == 0 {
		return FileList{}, err
	}
	count, _, _ := procDragQueryFile.Call(handle, 0xFFFFFFFF, 0, 0)
	paths := make([]string, 0, count)
	for i := uintptr(0); i < count; i++ {
		length, _, _ := procDragQueryFile.Call(handle, i, 0, 0)
		if length == 0 {
			continue
		}
		buf := make([]uint16, length+1)
		procDragQueryFile.Call(handle, i, uintptr(unsafe.Pointer(&buf[0])), length+1)
		paths = append(paths, syscall.UTF16ToString(buf))
	}
	return FileList{Sequence: sequence, Paths: paths, DropEffect: readPreferredDropEffect()}, nil
}

func writeFiles(paths []string, dropEffect uint32) error {
	if len(paths) == 0 {
		if ok, _, err := procOpenClipboard.Call(0); ok == 0 {
			return err
		}
		defer procCloseClipboard.Call()
		if ok, _, err := procEmptyClipboard.Call(); ok == 0 {
			return err
		}
		return nil
	}
	encoded := make([]uint16, 0, 256)
	for _, path := range paths {
		encoded = append(encoded, syscall.StringToUTF16(path)...)
	}
	encoded = append(encoded, 0)
	headerSize := uintptr(unsafe.Sizeof(dropFiles{}))
	bytesSize := uintptr(len(encoded) * 2)
	totalSize := headerSize + bytesSize
	mem, _, err := procGlobalAlloc.Call(gmemMoveable|gmemZeroInit, totalSize)
	if mem == 0 {
		return err
	}
	ptr, _, err := procGlobalLock.Call(mem)
	if ptr == 0 {
		procGlobalFree.Call(mem)
		return err
	}
	header := (*dropFiles)(unsafe.Pointer(ptr))
	header.PFiles = uint32(headerSize)
	header.FWide = 1
	target := unsafe.Slice((*byte)(unsafe.Pointer(ptr+headerSize)), bytesSize)
	source := unsafe.Slice((*byte)(unsafe.Pointer(&encoded[0])), bytesSize)
	copy(target, source)
	procGlobalUnlock.Call(mem)

	if ok, _, err := procOpenClipboard.Call(0); ok == 0 {
		procGlobalFree.Call(mem)
		return err
	}
	defer procCloseClipboard.Call()
	if ok, _, err := procEmptyClipboard.Call(); ok == 0 {
		procGlobalFree.Call(mem)
		return err
	}
	if data, _, err := procSetClipboardData.Call(cfHDrop, mem); data == 0 {
		procGlobalFree.Call(mem)
		return err
	}
	if dropEffect != 0 {
		if err := setPreferredDropEffect(dropEffect); err != nil {
			return err
		}
	}
	return nil
}

func currentSequence() uint32 {
	seq, _, _ := procGetClipboardSequenceNum.Call()
	return uint32(seq)
}

func preferredDropEffectFormat() uintptr {
	return registeredClipboardFormat("Preferred DropEffect")
}

func performedDropEffectFormat() uintptr {
	return registeredClipboardFormat("Performed DropEffect")
}

func registeredClipboardFormat(formatName string) uintptr {
	name := syscall.StringToUTF16Ptr(formatName)
	format, _, _ := procRegisterClipboardFormat.Call(uintptr(unsafe.Pointer(name)))
	return format
}

func readPreferredDropEffect() uint32 {
	effect := readDropEffectUnlocked(preferredDropEffectFormat())
	if effect == 0 {
		return DropEffectCopy
	}
	return effect
}

func readPerformedDropEffect() uint32 {
	if ok, _, _ := procOpenClipboard.Call(0); ok == 0 {
		return 0
	}
	defer procCloseClipboard.Call()
	return readDropEffectUnlocked(performedDropEffectFormat())
}

func readDropEffectUnlocked(format uintptr) uint32 {
	if format == 0 {
		return 0
	}
	if ok, _, _ := procIsClipboardFormatAvail.Call(format); ok == 0 {
		return 0
	}
	handle, _, _ := procGetClipboardData.Call(format)
	if handle == 0 {
		return 0
	}
	ptr, _, _ := procGlobalLock.Call(handle)
	if ptr == 0 {
		return 0
	}
	defer procGlobalUnlock.Call(handle)
	raw := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), 4)
	return binary.LittleEndian.Uint32(raw)
}

func setPreferredDropEffect(dropEffect uint32) error {
	format := preferredDropEffectFormat()
	if format == 0 {
		return nil
	}
	mem, _, err := procGlobalAlloc.Call(gmemMoveable|gmemZeroInit, 4)
	if mem == 0 {
		return err
	}
	ptr, _, err := procGlobalLock.Call(mem)
	if ptr == 0 {
		procGlobalFree.Call(mem)
		return err
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), 4)
	binary.LittleEndian.PutUint32(raw, dropEffect)
	procGlobalUnlock.Call(mem)
	if data, _, err := procSetClipboardData.Call(format, mem); data == 0 {
		procGlobalFree.Call(mem)
		return err
	}
	return nil
}
