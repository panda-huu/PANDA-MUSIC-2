package main

import (
	"strings"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

func nowPlayingCaption(song Song) string {
	return streamNowPlayingHTML(song)
}

func nowPlayingKB(chatID int64, elapsed, total float64) telegram.ReplyMarkup {
	return gogramMarkup(GetNowPlayingMarkup(progressBar(elapsed, total)))
}

func sendNowPlaying(chatID int64, song Song) *telegram.NewMessage {
	caption := nowPlayingCaption(song)
	kb := nowPlayingKB(chatID, 0, float64(parseDur(song.Duration)))
	if thumbsOff(chatID) {
		msg, _ := Bot.SendMessage(chatID, caption, &telegram.SendOptions{ParseMode: "HTML", ReplyMarkup: kb})
		return msg
	}
	vid := extractVideoID(song.URL)
	cover := cacheThumb(thumbFor(song.URL, song.Thumbnail))
	thumb := makeEditedThumb(cover, song.Title, song.Duration, song.Requester, vid)
	if thumb != "" {
		if abs, err := filepath.Abs(thumb); err == nil {
			thumb = abs
		}
		msg, err := Bot.SendMedia(chatID, thumb, &telegram.MediaOptions{
			Caption:     caption,
			ParseMode:   "HTML",
			ReplyMarkup: kb,
		})
		if err == nil {
			return msg
		}
		log.Println("panel upload failed:", err)
	}
	msg, _ := Bot.SendMessage(chatID, caption, &telegram.SendOptions{ParseMode: "HTML", ReplyMarkup: kb})
	return msg
}

func shouldStayInCall(chatID int64) bool {
	return hasLocalCall(chatID) && isLiveSession(chatID)
}

func playSong(chatID int64, message *telegram.NewMessage, song Song) error {
	return playSongOpt(chatID, message, song, shouldStayInCall(chatID))
}

func playSongOpt(chatID int64, message *telegram.NewMessage, song Song, stayInCall bool) error {
	holdSwitch(chatID)
	defer releaseSwitch(chatID)

	loading := wrapBQ("<b>" + smallcaps("loading") + "...</b>\n" + richEsc(shortTitle(song.Title, 40)))
	if message != nil {
		_ = editHTML(message, loading, nil)
	} else {
		message, _ = sendHTML(Bot, chatID, loading, nil)
	}

	if t := oembedTitle(extractVideoID(song.URL)); t != "" && (song.Title == "" || song.Title == "YouTube Video") {
		song.Title = t
	}
	setSeekState(chatID, 0)

	src, err := takePrefetch(song.URL, song.Video)
	if err != nil {
		src, err = downloadWithRetry(song.URL, song.Video)
	}
	if err != nil {
		removeFromQueue(chatID, 0)
		_, _ = sendHTML(Bot, chatID, userFail("download failed", err), nil)
		return err
	}
	if src.File != "" && !isHTTP(src.File) && song.Video && !fileHasVideo(src.File) {
		removeFromQueue(chatID, 0)
		_, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("vplay failed")+"\n"+smallcaps("api gave audio file, not video stream.")), nil)
		return fmt.Errorf("no video track in %s", src.File)
	}
	if src.File != "" && !isHTTP(src.File) && !song.Video {
		if p, e := maybeApplyEffects(chatID, src.File); e == nil {
			src.File, src.Audio = p, p
		}
	}
	if src.File != "" && !isHTTP(src.File) {
		// Use the downloaded media's real duration. Search metadata can be
		// rounded or stale; using it caused the progress timer to end playback
		// early and disconnect the assistant while audio was still playing.
		if secs := probeDuration(src.File); secs > 1 {
			song.DurationSeconds = int(secs + 0.5)
			song.Duration = formatClock(secs)
		} else if song.DurationSeconds > 1 && (song.Duration == "" || song.Duration == "0:00") {
			song.Duration = formatClock(float64(song.DurationSeconds))
		}
		setCurrentDuration(chatID, song)
	}
	playPath := src.File
	if playPath == "" {
		playPath = src.Audio
	}
	setCurrentPath(chatID, playPath)

	wasVideo := callIsVideo[chatID]
	media := buildMediaAV(src.Audio, src.Video, song.Video, 0)
	if !song.Video && (wasVideo || hasLocalCall(chatID)) {
		cover := cacheThumb(thumbFor(song.URL, song.Thumbnail))
		if cover == "" {
			cover = playPath
		}
		media = withStillCover(media, cover)
	}
	startErr := ntgPlay(chatID, media, song.Video)
	if startErr != nil {
		_, _ = sendHTML(Bot, chatID, userFail("playback failed", startErr), nil)
		return startErr
	}

	markLiveSession(chatID)
	callIsVideo[chatID] = song.Video
	addServedChat(chatID)
	incrementPlayCount(chatID)
	go logPlayAction(chatID, song, false)
	total := float64(song.DurationSeconds)
	if total <= 0 {
		total = float64(parseDur(song.Duration))
	}
	if message != nil {
		_, _ = message.Delete()
	}
	msg := sendNowPlaying(chatID, song)
	go updateProgress(chatID, msg, time.Now(), total, song)
	return nil
}

func samePlaying(cur *Song, song Song) bool {
	if cur == nil {
		return false
	}
	if cur.URL != "" && cur.URL == song.URL {
		return true
	}
	if cur.FilePath != "" && (cur.FilePath == song.FilePath || cur.FilePath == song.URL || cur.URL == song.FilePath) {
		return true
	}
	return cur.Title == song.Title && cur.RequesterID == song.RequesterID && song.RequesterID != 0
}

func updateProgress(chatID int64, msg *telegram.NewMessage, start time.Time, total float64, song Song) {
	for {
		time.Sleep(8 * time.Second)
		if isSwitching(chatID) {
			continue
		}
		cur := peekCurrent(chatID)
		if !samePlaying(cur, song) {
			return
		}
		if cur.DurationSeconds > 1 && total <= 1 {
			total = float64(cur.DurationSeconds)
			song.Duration = cur.Duration
			song.DurationSeconds = cur.DurationSeconds
		}
		elapsed := time.Since(start).Seconds() + float64(getSeekState(chatID))
		if elapsed < 0 {
			elapsed = 0
		}
		if total > 0 && elapsed > total {
			elapsed = total
		}
		if msg != nil {
			if err := editHTML(msg, nowPlayingCaption(song), nowPlayingKB(chatID, elapsed, total)); err != nil && !isNotModified(err) {
				log.Println("progress edit failed", chatID, err)
			}
		}
		if total > 0 && elapsed >= total {
			go handleStreamEnd(chatID)
			return
		}
	}
}

func userFail(title string, err error) string {
	reason := "try again in a moment"
	msg := ""
	if err != nil {
		msg = strings.ToLower(err.Error())
	}
	switch {
	case strings.Contains(msg, "page needs to be reloaded"), strings.Contains(msg, "sign in"), strings.Contains(msg, "bot"):
		reason = "youtube blocked this download"
	case strings.Contains(msg, "empty api file"):
		reason = "download came back empty"
	case strings.Contains(msg, "audio file, not video"):
		reason = "video file was not ready"
	case strings.Contains(msg, "timeout"):
		reason = "download timed out"
	}
	return wrapBQ(smallcaps(title) + "\n" + smallcaps(reason))
}
