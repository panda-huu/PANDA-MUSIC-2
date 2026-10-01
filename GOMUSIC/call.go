package main

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/nikhil390u8o/GOMUSICV2/ntgcalls"
)

var (
	callIsVideo  = map[int64]bool{}
	connectWait  = map[int64]chan error{}
	connectMu    sync.Mutex
	streamEndMu  sync.Mutex
	switching    sync.Map
	liveSession  sync.Map
	switchHold   = map[int64]int{}
	currentPaths = map[int64]string{}
	streamGen    = map[int64]int{}
	streamAt     = map[int64]time.Time{}
	joinParams   = map[int64]string{}
	chatPlayMu   sync.Map
)

func lockChatPlay(chatID int64) func() {
	v, _ := chatPlayMu.LoadOrStore(chatID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func markLiveSession(chatID int64) { liveSession.Store(chatID, true) }
func isLiveSession(chatID int64) bool {
	_, ok := liveSession.Load(chatID)
	return ok
}
func clearLiveSession(chatID int64) { liveSession.Delete(chatID) }

func bumpStream(chatID int64) int {
	connectMu.Lock()
	defer connectMu.Unlock()
	streamGen[chatID]++
	streamAt[chatID] = time.Now()
	return streamGen[chatID]
}

func currentGen(chatID int64) int {
	connectMu.Lock()
	defer connectMu.Unlock()
	return streamGen[chatID]
}

func holdSwitch(chatID int64) {
	connectMu.Lock()
	switchHold[chatID]++
	connectMu.Unlock()
	switching.Store(chatID, time.Now())
	bumpStream(chatID)
}

func releaseSwitch(chatID int64) {
	connectMu.Lock()
	if switchHold[chatID] > 0 {
		switchHold[chatID]--
	}
	left := switchHold[chatID]
	connectMu.Unlock()
	switching.Store(chatID, time.Now())
	if left <= 0 {
		bumpStream(chatID)
	}
}

func beginSwitch(chatID int64) { holdSwitch(chatID) }
func endSwitch(chatID int64)   { releaseSwitch(chatID) }

func isSwitching(chatID int64) bool {
	connectMu.Lock()
	held := switchHold[chatID] > 0
	connectMu.Unlock()
	if held {
		return true
	}
	v, ok := switching.Load(chatID)
	if !ok {
		return false
	}
	t, _ := v.(time.Time)
	return time.Since(t) < 20*time.Second
}

func setCurrentPath(chatID int64, path string) {
	connectMu.Lock()
	currentPaths[chatID] = path
	connectMu.Unlock()
	queueMu.Lock()
	q := chatQueues[chatID]
	if len(q) > 0 {
		q[0].FilePath = path
		chatQueues[chatID] = q
	}
	queueMu.Unlock()
}

func currentPath(chatID int64) string {
	connectMu.Lock()
	defer connectMu.Unlock()
	return currentPaths[chatID]
}

func hasLocalCall(chatID int64) bool {
	return Calls != nil && Calls.Calls()[chatID] != nil
}

func clearCallCache(chatID int64) {
	delete(activeCalls, chatID)
}

func leaveVCNow(chatID int64) {
	connectMu.Lock()
	switchHold[chatID] = 0
	delete(joinParams, chatID)
	connectMu.Unlock()
	switching.Delete(chatID)
	clearLiveSession(chatID)
	bumpStream(chatID)
	stopAutoplay(chatID)
	for _, song := range clearQueue(chatID) {
		deleteFile(song.FilePath)
	}
	if Calls != nil {
		_ = Calls.Stop(chatID)
	}
	if call, ok := activeCalls[chatID]; ok {
		_, _ = Assistant.PhoneLeaveGroupCall(call, 0)
		delete(activeCalls, chatID)
	}
	delete(callIsVideo, chatID)
}

func leaveVC(chatID int64) {
	if isSwitching(chatID) || isLiveSession(chatID) {
		return
	}
	leaveVCNow(chatID)
}

func changeStream(chatID int64) error {
	return RoomChangeStream(chatID)
}

func handleStreamEnd(chatID int64) {
	if isSwitching(chatID) {
		log.Println("stream-end ignored (switch hold)", chatID)
		return
	}
	streamEndMu.Lock()
	defer streamEndMu.Unlock()
	if isSwitching(chatID) {
		return
	}
	connectMu.Lock()
	started := streamAt[chatID]
	connectMu.Unlock()
	if started.IsZero() || time.Since(started) < 12*time.Second {
		log.Println("stream-end ignored (too soon)", chatID)
		return
	}
	if peekCurrent(chatID) == nil && peekNext(chatID) == nil {
		if !isLiveSession(chatID) {
			leaveVCNow(chatID)
		}
		return
	}
	_ = RoomChangeStream(chatID)
}

func ensureVC(chatID int64) error {
	if _, err := fetchLiveCall(chatID); err == nil {
		return nil
	}
	peer, err := Assistant.ResolvePeer(chatID)
	if err != nil {
		return err
	}
	_, err = Assistant.PhoneCreateGroupCall(&telegram.PhoneCreateGroupCallParams{
		Peer:     peer,
		RandomID: int32(rand.Intn(90000) + 10000),
	})
	if err != nil {
		low := strings.ToLower(err.Error())
		if strings.Contains(low, "already") || strings.Contains(low, "groupcall_already_started") {
			return nil
		}
		return err
	}
	return nil
}

func fetchLiveCall(chatID int64) (telegram.InputGroupCall, error) {
	peer, err := Assistant.ResolvePeer(chatID)
	if err != nil {
		return nil, err
	}
	switch p := peer.(type) {
	case *telegram.InputPeerChannel:
		full, err := Assistant.ChannelsGetFullChannel(&telegram.InputChannelObj{ChannelID: p.ChannelID, AccessHash: p.AccessHash})
		if err != nil {
			return nil, err
		}
		cf, ok := full.FullChat.(*telegram.ChannelFull)
		if !ok || cf.Call == nil {
			return nil, errors.New("no active group call")
		}
		return cf.Call, nil
	case *telegram.InputPeerChat:
		full, err := Assistant.MessagesGetFullChat(p.ChatID)
		if err != nil {
			return nil, err
		}
		cf, ok := full.FullChat.(*telegram.ChatFullObj)
		if !ok || cf.Call == nil {
			return nil, errors.New("no active group call")
		}
		return cf.Call, nil
	default:
		return nil, fmt.Errorf("unsupported peer type %T", peer)
	}
}

func resolveActiveCall(chatID int64) (telegram.InputGroupCall, error) {
	return fetchLiveCall(chatID)
}

func invalidCallErr(err error) bool {
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "groupcall_invalid") ||
		strings.Contains(low, "groupcall_forbidden") ||
		strings.Contains(low, "groupcall_not_modified") ||
		strings.Contains(low, "join_as_peer_invalid")
}

func shouldRetryJoin(err error) bool {
	if err == nil {
		return false
	}
	if invalidCallErr(err) {
		return true
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "interdc") ||
		strings.Contains(low, "timed out") ||
		strings.Contains(low, "timeout") ||
		strings.Contains(low, "flood") ||
		strings.Contains(low, "500") ||
		strings.Contains(low, "503")
}

func alreadyJoinedError(err error) bool {
	if err == nil {
		return false
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "already") ||
		strings.Contains(low, "participant_join") ||
		strings.Contains(low, "joined")
}

func startNTGStream(chatID int64, path string, video bool, seekSec int) error {
	if Calls == nil {
		return errors.New("ntgcalls not ready")
	}
	media := buildMedia(path, video, seekSec)
	return startNTGStreamWithMedia(chatID, media, video)
}

func ntgPlay(chatID int64, media ntgcalls.MediaDescription, video bool) error {
	if Calls == nil {
		return errors.New("ntgcalls not ready")
	}
	if hasLocalCall(chatID) && isLiveSession(chatID) {
		err := Calls.SetStreamSources(chatID, ntgcalls.CaptureStream, media)
		if err == nil {
			callIsVideo[chatID] = video || media.Camera != nil
			return nil
		}
		low := strings.ToLower(err.Error())
		if strings.Contains(low, "not found") || strings.Contains(low, "already") || strings.Contains(low, "remove") {
			log.Println("setstream missing call, rejoining", chatID, err)
			_ = Calls.Stop(chatID)
			connectMu.Lock()
			delete(joinParams, chatID)
			connectMu.Unlock()
			clearLiveSession(chatID)
			return joinExistingCall(chatID, media, video || media.Camera != nil)
		}
		return err
	}
	return joinExistingCall(chatID, media, video || media.Camera != nil)
}

func resetLocalCall(chatID int64) {
	if Calls != nil {
		_ = Calls.Stop(chatID)
	}
	connectMu.Lock()
	delete(joinParams, chatID)
	connectMu.Unlock()
	clearLiveSession(chatID)
}

func joinExistingCall(chatID int64, media ntgcalls.MediaDescription, video bool) error {
	if Calls == nil {
		return errors.New("ntgcalls not ready")
	}
	if hasLocalCall(chatID) && isLiveSession(chatID) {
		return Calls.SetStreamSources(chatID, ntgcalls.CaptureStream, media)
	}

	var last error
	for attempt := 1; attempt <= 5; attempt++ {
		var local string
		if hasLocalCall(chatID) {
			connectMu.Lock()
			local = joinParams[chatID]
			connectMu.Unlock()
		}
		if local == "" {
			if hasLocalCall(chatID) {
				_ = Calls.Stop(chatID)
			}
			created, err := Calls.CreateCall(chatID)
			if err != nil {
				low := strings.ToLower(err.Error())
				if strings.Contains(low, "already") && hasLocalCall(chatID) {
					connectMu.Lock()
					local = joinParams[chatID]
					connectMu.Unlock()
					if local == "" {
						return Calls.SetStreamSources(chatID, ntgcalls.CaptureStream, media)
					}
				} else {
					last = err
					time.Sleep(time.Duration(attempt) * 400 * time.Millisecond)
					continue
				}
			} else {
				local = created
			}
			connectMu.Lock()
			joinParams[chatID] = local
			connectMu.Unlock()
		}

		joinMedia := media
		if video && media.Camera != nil && media.Microphone != nil {
			joinMedia = ntgcalls.MediaDescription{Microphone: media.Microphone}
		}
		if err := Calls.SetStreamSources(chatID, ntgcalls.CaptureStream, joinMedia); err != nil {
			last = err
			resetLocalCall(chatID)
			time.Sleep(time.Duration(attempt) * 400 * time.Millisecond)
			continue
		}

		me, err := assistantSelf()
		if err != nil {
			return err
		}

		inputCall, err := fetchLiveCall(chatID)
		if err != nil {
			if e := ensureVC(chatID); e != nil {
				last = e
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			inputCall, err = fetchLiveCall(chatID)
			if err != nil {
				last = err
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
		}

		updates, err := Assistant.PhoneJoinGroupCall(&telegram.PhoneJoinGroupCallParams{
			Call:         inputCall,
			JoinAs:       &telegram.InputPeerUser{UserID: me.ID, AccessHash: me.AccessHash},
			VideoStopped: !video,
			Muted:        false,
			Params:       &telegram.DataJson{Data: local},
		})
		if err != nil {
			last = err
			if alreadyJoinedError(err) {
				activeCalls[chatID] = inputCall
				callIsVideo[chatID] = video
				if video && media.Camera != nil {
					_ = Calls.SetStreamSources(chatID, ntgcalls.CaptureStream, media)
				}
				return nil
			}
			if invalidCallErr(err) || shouldRetryJoin(err) {
				log.Println("join retry", attempt, chatID, err)
				clearCallCache(chatID)
				resetLocalCall(chatID)
				time.Sleep(time.Duration(attempt) * 800 * time.Millisecond)
				continue
			}
			return err
		}

		remote := "{\"transport\": null}"
		if u, ok := updates.(*telegram.UpdatesObj); ok {
			for _, upd := range u.Updates {
				if conn, ok := upd.(*telegram.UpdateGroupCallConnection); ok && conn.Params != nil {
					remote = conn.Params.Data
				}
			}
		}
		if err := Calls.Connect(chatID, remote, false); err != nil {
			last = err
			log.Println("ntg connect timeout/retry", attempt, chatID, err)
			resetLocalCall(chatID)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		if video && media.Camera != nil {
			time.Sleep(400 * time.Millisecond)
			if err := Calls.SetStreamSources(chatID, ntgcalls.CaptureStream, media); err != nil {
				log.Println("video upgrade after connect:", err)
			}
		}
		activeCalls[chatID] = inputCall
		callIsVideo[chatID] = video
		return nil
	}
	if last == nil {
		last = fmt.Errorf("join call failed")
	}
	return last
}

var cachedAssistant *telegram.UserObj

// assistantSelf returns the cached assistant UserObj, fetching it once via GetMe.
// Previously this called itself recursively causing a stack overflow on first /play.
func assistantSelf() (*telegram.UserObj, error) {
	if cachedAssistant != nil {
		return cachedAssistant, nil
	}
	if Assistant == nil {
		return nil, fmt.Errorf("assistant not ready")
	}
	me, err := Assistant.GetMe()
	if err != nil {
		return nil, err
	}
	if me == nil {
		return nil, fmt.Errorf("assistant GetMe returned nil")
	}
	cachedAssistant = me
	return cachedAssistant, nil
}

// holdMedia streams 30 s of silence to keep the call slot open while the real
// track downloads. Was 600 s before — that spawned a long-lived ffmpeg process
// that pushed the container past its memory limit and triggered the OOM killer.
func holdMedia() ntgcalls.MediaDescription {
	return ntgcalls.MediaDescription{Microphone: &ntgcalls.AudioDescription{
		MediaSource:  ntgcalls.MediaSourceShell,
		SampleRate:   48000,
		ChannelCount: 2,
		Input:        "ffmpeg -nostdin -hide_banner -re -threads 1 -f lavfi -i anullsrc=r=48000:cl=stereo -t 30 -f s16le -ac 2 -ar 48000 -v quiet pipe:1",
	}}
}
