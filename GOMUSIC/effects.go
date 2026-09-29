package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type effectState struct {
	Speed   float64
	Bass    int
	Enabled bool
}

var (
	fxCache = map[int64]effectState{}
	fxMu    sync.Mutex
	seekPos = map[int64]int{}
)

func getEffects(chatID int64) effectState {
	fxMu.Lock()
	defer fxMu.Unlock()
	if s, ok := fxCache[chatID]; ok {
		return s
	}
	sp, b, en := loadChatEffects(chatID)
	s := effectState{Speed: sp, Bass: b, Enabled: en}
	if s.Speed == 0 {
		s.Speed = 1
	}
	fxCache[chatID] = s
	return s
}

func setSpeed(chatID int64, speed float64) {
	s := getEffects(chatID)
	s.Speed = speed
	fxMu.Lock()
	fxCache[chatID] = s
	fxMu.Unlock()
	saveChatEffects(chatID, s.Speed, s.Bass, s.Enabled)
}

func setBass(chatID int64, bass int) {
	s := getEffects(chatID)
	s.Bass = bass
	fxMu.Lock()
	fxCache[chatID] = s
	fxMu.Unlock()
	saveChatEffects(chatID, s.Speed, s.Bass, s.Enabled)
}

func setEffectsEnabled(chatID int64, val bool) {
	s := getEffects(chatID)
	s.Enabled = val
	fxMu.Lock()
	fxCache[chatID] = s
	fxMu.Unlock()
	saveChatEffects(chatID, s.Speed, s.Bass, s.Enabled)
}

func setSeekState(chatID int64, sec int) {
	if sec < 0 {
		sec = 0
	}
	fxMu.Lock()
	seekPos[chatID] = sec
	fxMu.Unlock()
}

func getSeekState(chatID int64) int {
	fxMu.Lock()
	defer fxMu.Unlock()
	return seekPos[chatID]
}

func currentPlaybackSec(chatID int64) int {
	base := getSeekState(chatID)
	connectMu.Lock()
	started := streamAt[chatID]
	connectMu.Unlock()
	if !started.IsZero() {
		base += int(time.Since(started).Seconds())
	}
	if base < 0 {
		return 0
	}
	return base
}

func nextSeekSec(chatID int64, deltaSec int) int {
	next := currentPlaybackSec(chatID) + deltaSec
	if next < 0 {
		next = 0
	}
	if s := peekCurrent(chatID); s != nil {
		if dur := parseDur(s.Duration); dur > 3 && next > dur-2 {
			next = dur - 2
		}
	}
	return next
}

func maybeApplyEffects(chatID int64, path string) (string, error) {
	s := getEffects(chatID)
	if !s.Enabled && s.Speed == 1 && s.Bass == 0 {
		return path, nil
	}
	var filters []string
	if s.Speed != 0 && s.Speed != 1 {
		filters = append(filters, fmt.Sprintf("atempo=%.2f", s.Speed))
	}
	if s.Bass > 0 {
		filters = append(filters, fmt.Sprintf("bass=g=%d", s.Bass))
	}
	if len(filters) == 0 {
		return path, nil
	}
	out := strings.TrimSuffix(path, filepath.Ext(path)) + "_fx.mp3"
	args := []string{"-y", "-i", path, "-af", strings.Join(filters, ","), out}
	cmd := exec.Command("ffmpeg", args...)
	if b, err := cmd.CombinedOutput(); err != nil {
		return path, fmt.Errorf("%s", strings.TrimSpace(string(b)))
	}
	return out, nil
}

func parseFloatArg(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}
