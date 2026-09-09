//go:build windows

package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	dibRGBColors = 0
	biRGB        = 0
	diNormal     = 0x0003
)

var (
	iconShell32 = windows.NewLazySystemDLL("shell32.dll")
	iconUser32  = windows.NewLazySystemDLL("user32.dll")
	iconGDI32   = windows.NewLazySystemDLL("gdi32.dll")

	procExtractIconExW      = iconShell32.NewProc("ExtractIconExW")
	procDestroyIcon         = iconUser32.NewProc("DestroyIcon")
	procDrawIconEx          = iconUser32.NewProc("DrawIconEx")
	procCreateCompatibleDC  = iconGDI32.NewProc("CreateCompatibleDC")
	procDeleteDC            = iconGDI32.NewProc("DeleteDC")
	procCreateDIBSection    = iconGDI32.NewProc("CreateDIBSection")
	procSelectObject        = iconGDI32.NewProc("SelectObject")
	procDeleteObject        = iconGDI32.NewProc("DeleteObject")
	windowsEnvPattern       = regexp.MustCompile(`%([^%]+)%`)
	contextMenuIconPreviews sync.Map
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
}

// IconPreview returns a PNG data URL for the icon Explorer uses for a static verb.
// Explicit Icon registry data wins; when it is empty, the command executable is used.
func (s *ContextMenuService) IconPreview(icon, command string) string {
	source := strings.TrimSpace(icon)
	if source == "" {
		source = firstCommandToken(command)
	}
	if source == "" {
		return ""
	}

	cacheKey := source
	if cached, ok := contextMenuIconPreviews.Load(cacheKey); ok {
		return cached.(string)
	}

	preview, err := iconPreviewDataURL(source, 32)
	if err != nil {
		contextMenuIconPreviews.Store(cacheKey, "")
		return ""
	}
	contextMenuIconPreviews.Store(cacheKey, preview)
	return preview
}

func iconPreviewDataURL(source string, size int) (string, error) {
	path, index, err := parseIconLocation(source)
	if err != nil {
		return "", err
	}
	path = resolveIconPath(path)
	if path == "" {
		return "", fmt.Errorf("找不到图标资源: %s", source)
	}

	hicon, err := extractIcon(path, index)
	if err != nil {
		return "", err
	}
	defer procDestroyIcon.Call(hicon)

	data, err := iconToPNG(hicon, size)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data), nil
}

func parseIconLocation(source string) (string, int32, error) {
	value := strings.TrimSpace(source)
	value = strings.TrimSpace(strings.TrimPrefix(value, "@"))
	if value == "" {
		return "", 0, fmt.Errorf("图标路径为空")
	}

	var index int32
	if comma := strings.LastIndex(value, ","); comma >= 0 {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(value[comma+1:]), 10, 32); err == nil {
			index = int32(parsed)
			value = strings.TrimSpace(value[:comma])
		}
	}

	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	value = expandWindowsEnvironment(value)
	if value == "" {
		return "", 0, fmt.Errorf("图标路径为空")
	}
	return value, index, nil
}

func expandWindowsEnvironment(value string) string {
	value = os.ExpandEnv(value)
	return windowsEnvPattern.ReplaceAllStringFunc(value, func(match string) string {
		name := match[1 : len(match)-1]
		if expanded, ok := os.LookupEnv(name); ok {
			return expanded
		}
		return match
	})
}

func resolveIconPath(value string) string {
	candidate := filepath.Clean(strings.TrimSpace(value))
	if candidate == "." || candidate == "" {
		return ""
	}
	if fileExists(candidate) {
		if abs, err := filepath.Abs(candidate); err == nil {
			return abs
		}
		return candidate
	}
	if path, err := exec.LookPath(candidate); err == nil && fileExists(path) {
		if abs, absErr := filepath.Abs(path); absErr == nil {
			return abs
		}
		return path
	}
	if filepath.IsAbs(candidate) {
		return ""
	}

	systemRoot := os.Getenv("SystemRoot")
	for _, dir := range []string{filepath.Join(systemRoot, "System32"), systemRoot} {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, candidate)
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func extractIcon(path string, index int32) (uintptr, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}

	var large uintptr
	var small uintptr
	count, _, _ := procExtractIconExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(uint32(index)),
		uintptr(unsafe.Pointer(&large)),
		uintptr(unsafe.Pointer(&small)),
		1,
	)
	if count == 0 {
		return 0, fmt.Errorf("无法提取图标: %s,%d", path, index)
	}

	if large != 0 {
		if small != 0 && small != large {
			procDestroyIcon.Call(small)
		}
		return large, nil
	}
	if small != 0 {
		return small, nil
	}
	return 0, fmt.Errorf("图标资源为空: %s,%d", path, index)
}

func iconToPNG(hicon uintptr, size int) ([]byte, error) {
	black, err := drawIconOnBackground(hicon, size, 0)
	if err != nil {
		return nil, err
	}
	white, err := drawIconOnBackground(hicon, size, 255)
	if err != nil {
		return nil, err
	}

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for pixel := 0; pixel < size*size; pixel++ {
		src := pixel * 4
		db := positiveDifference(white[src], black[src])
		dg := positiveDifference(white[src+1], black[src+1])
		dr := positiveDifference(white[src+2], black[src+2])
		alpha := 255 - max3(db, dg, dr)

		dst := pixel * 4
		if alpha <= 0 {
			img.Pix[dst+3] = 0
			continue
		}
		img.Pix[dst] = unpremultiply(black[src+2], alpha)
		img.Pix[dst+1] = unpremultiply(black[src+1], alpha)
		img.Pix[dst+2] = unpremultiply(black[src], alpha)
		img.Pix[dst+3] = uint8(alpha)
	}

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func drawIconOnBackground(hicon uintptr, size int, background byte) ([]byte, error) {
	hdc, _, _ := procCreateCompatibleDC.Call(0)
	if hdc == 0 {
		return nil, fmt.Errorf("创建图标绘制 DC 失败")
	}
	defer procDeleteDC.Call(hdc)

	info := bitmapInfo{Header: bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:       int32(size),
		Height:      -int32(size),
		Planes:      1,
		BitCount:    32,
		Compression: biRGB,
	}}
	var bits unsafe.Pointer
	hbitmap, _, _ := procCreateDIBSection.Call(
		hdc,
		uintptr(unsafe.Pointer(&info)),
		dibRGBColors,
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if hbitmap == 0 || bits == nil {
		return nil, fmt.Errorf("创建图标位图失败")
	}
	defer procDeleteObject.Call(hbitmap)

	previous, _, _ := procSelectObject.Call(hdc, hbitmap)
	if previous == 0 {
		return nil, fmt.Errorf("选择图标位图失败")
	}
	defer procSelectObject.Call(hdc, previous)

	pixels := unsafe.Slice((*byte)(bits), size*size*4)
	for offset := 0; offset < len(pixels); offset += 4 {
		pixels[offset] = background
		pixels[offset+1] = background
		pixels[offset+2] = background
		pixels[offset+3] = 255
	}

	drawn, _, _ := procDrawIconEx.Call(
		hdc,
		0,
		0,
		hicon,
		uintptr(size),
		uintptr(size),
		0,
		0,
		diNormal,
	)
	if drawn == 0 {
		return nil, fmt.Errorf("绘制图标失败")
	}

	result := make([]byte, len(pixels))
	copy(result, pixels)
	return result, nil
}

func positiveDifference(a, b byte) int {
	if a <= b {
		return 0
	}
	return int(a) - int(b)
}

func max3(a, b, c int) int {
	if a < b {
		a = b
	}
	if a < c {
		a = c
	}
	return a
}

func unpremultiply(value byte, alpha int) uint8 {
	if alpha <= 0 {
		return 0
	}
	channel := int(value) * 255 / alpha
	if channel > 255 {
		channel = 255
	}
	return uint8(channel)
}
