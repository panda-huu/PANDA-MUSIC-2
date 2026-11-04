package ntgcalls

import (
	"runtime/cgo"
	"unsafe"
)

type Client struct {
	ptr                         uintptr
	userData                    unsafe.Pointer
	handle                      cgo.Handle
	connectionChangeCallbacks   []ConnectionChangeCallback
	streamEndCallbacks          []StreamEndCallback
	upgradeCallbacks            []UpgradeCallback
	frameCallbacks              []FrameCallback
	remoteSourceCallbacks       []RemoteSourceCallback
	broadcastTimestampCallbacks []BroadcastTimestampCallback
	broadcastPartCallbacks      []BroadcastPartCallback
}
