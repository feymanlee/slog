package modules

import (
	"path/filepath"
	"runtime"
	"strconv"
)

// Frame 获取调用帧。
func Frame(pc uintptr) runtime.Frame {
	fs := runtime.CallersFrames([]uintptr{pc})
	f, _ := fs.Next()
	return f
}

// SourceLabel 将 frame 转为短路径标签。
func SourceLabel(f runtime.Frame) string {
	return filepath.Base(f.File) + ":" + strconv.Itoa(f.Line)
}
