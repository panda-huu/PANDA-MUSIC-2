package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

func songFromReply(m *telegram.NewMessage, video bool) (*Song, string) {
	if m == nil {
		return nil, ""
	}
	target := m
	replied := m.IsReply()
	if replied {
		reply, err := m.GetReplyMessage()
		if err != nil || reply == nil {
			log.Println("reply media fetch fail", err)
			return nil, "could not read the replied message"
		}
		target = reply
	} else if !m.IsMedia() {
		return nil, ""
	}
	kind, title := replyMediaKind(target)
	log.Println("reply media kind", kind, "title", title, "type", target.MediaType())
	if kind == "" {
		if video {
			return nil, "reply to a video for /vplay"
		}
		return nil, "reply to an audio file for /play"
	}
	if video && kind != "video" {
		return nil, "reply to a video for /vplay"
	}
	if !video && kind != "audio" {
		return nil, "reply to an audio file for /play"
	}
	_ = os.MkdirAll(downloadDir, 0o755)
	name := "media"
	extn := ".mp3"
	if video {
		extn = ".mp4"
	}
	if f := target.File(); f != nil && strings.TrimSpace(f.Name) != "" {
		name = safeMediaName(f.Name)
		if title == "" || title == "audio" || title == "video" || title == "voice" {
			title = strings.TrimSuffix(name, filepath.Ext(name))
		}
		if filepath.Ext(name) == "" {
			name += extn
		}
	} else if video {
		name = "video.mp4"
	} else {
		name = "audio.mp3"
	}
	dest := filepath.Join(downloadDir, fmt.Sprintf("tg_%d_%s", time.Now().UnixNano(), name))
	path, err := target.Download(&telegram.DownloadOptions{FileName: dest})
	if err != nil || path == "" {
		if err == nil {
			err = fmt.Errorf("empty download")
		}
		return nil, "download failed: " + err.Error()
	}
	if st, e := os.Stat(path); e != nil || st.Size() < 1024 {
		if st2, e2 := os.Stat(dest); e2 == nil && st2.Size() >= 1024 {
			path = dest
		} else {
			return nil, "downloaded file is empty"
		}
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	dur := "0:00"
	secs := 0
	if n := probeDuration(path); n > 1 {
		secs = int(n)
		dur = formatClock(n)
	}
	return &Song{
		URL:             path,
		FilePath:        path,
		Title:           title,
		Duration:        dur,
		DurationSeconds: secs,
		Requester:       userNameOf(m),
		RequesterID:     userIDOf(m),
		Video:           video,
	}, ""
}

func replyMediaKind(reply *telegram.NewMessage) (kind, title string) {
	if reply == nil {
		return "", ""
	}
	dump := strings.ToLower(reply.MediaType() + " " + fmt.Sprintf("%v", reply.Media()))
	if f := reply.File(); f != nil {
		dump += " " + strings.ToLower(f.Name)
	}
	if media, ok := reply.Media().(*telegram.MessageMediaDocument); ok && media != nil {
		doc := asDocument(media.Document)
		switch {
		case media.Voice:
			return "audio", mediaTitle(reply, doc, "voice")
		case media.Round, media.Video:
			return "video", mediaTitle(reply, doc, "video")
		}
		if doc != nil {
			if k := kindFromMime(doc.MimeType); k != "" {
				return k, mediaTitle(reply, doc, k)
			}
		}
	}
	if doc := reply.Audio(); doc != nil {
		return "audio", mediaTitle(reply, doc, "audio")
	}
	if doc := reply.Video(); doc != nil {
		return "video", mediaTitle(reply, doc, "video")
	}
	if doc := reply.Document(); doc != nil {
		if k := kindFromMime(doc.MimeType); k != "" {
			return k, mediaTitle(reply, doc, k)
		}
		return kindFromName(mediaTitle(reply, doc, ""), doc)
	}
	if k := kindFromName(dump, nil); k != "" {
		return k, mediaTitle(reply, nil, k)
	}
	switch {
	case strings.Contains(dump, "voice"), strings.Contains(dump, "audio"):
		return "audio", mediaTitle(reply, nil, "audio")
	case strings.Contains(dump, "video"), strings.Contains(dump, "round"):
		return "video", mediaTitle(reply, nil, "video")
	}
	return "", ""
}

func kindFromMime(mime string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	switch {
	case strings.HasPrefix(mime, "audio/"), mime == "application/ogg", mime == "application/x-ogg":
		return "audio"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	}
	return ""
}

func kindFromName(name string, doc *telegram.DocumentObj) (string, string) {
	low := strings.ToLower(name)
	audioExt := []string{".mp3", ".m4a", ".aac", ".ogg", ".opus", ".wav", ".flac", ".oga", ".wma"}
	videoExt := []string{".mp4", ".mkv", ".webm", ".mov", ".avi", ".m4v", ".3gp"}
	for _, ext := range audioExt {
		if strings.Contains(low, ext) {
			return "audio", mediaTitle(nil, doc, "audio")
		}
	}
	for _, ext := range videoExt {
		if strings.Contains(low, ext) {
			return "video", mediaTitle(nil, doc, "video")
		}
	}
	return "", ""
}

func asDocument(doc telegram.Document) *telegram.DocumentObj {
	if doc == nil {
		return nil
	}
	d, _ := doc.(*telegram.DocumentObj)
	return d
}

func mediaTitle(reply *telegram.NewMessage, doc *telegram.DocumentObj, fallback string) string {
	if reply != nil {
		if f := reply.File(); f != nil {
			name := strings.TrimSpace(f.Name)
			if name != "" {
				return strings.TrimSuffix(name, filepath.Ext(name))
			}
		}
	}
	if doc != nil {
		for _, attr := range doc.Attributes {
			switch a := attr.(type) {
			case *telegram.DocumentAttributeFilename:
				if a != nil && a.FileName != "" {
					return strings.TrimSuffix(a.FileName, filepath.Ext(a.FileName))
				}
			case *telegram.DocumentAttributeAudio:
				if a != nil && a.Title != "" {
					if a.Performer != "" {
						return a.Performer + " - " + a.Title
					}
					return a.Title
				}
			}
		}
	}
	return fallback
}

func safeMediaName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "media"
	}
	return b.String()
}

func playPrepared(m *telegram.NewMessage, song Song) error {
	return playPreparedMsg(m, song, nil)
}

func playPreparedMsg(m *telegram.NewMessage, song Song, pm *telegram.NewMessage) error {
	chatID := m.ChatID()
	already := isBusy(chatID)
	if pm == nil {
		if already {
			pm, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("adding to queue...")), nil)
		} else {
			pm, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("processing...")), nil)
		}
	} else if already {
		_ = editHTML(pm, wrapBQ(smallcaps("adding to queue...")), nil)
	}
	if !already {
		ok, banned := assistantIn(chatID)
		if banned {
			_ = editHTML(pm, wrapBQ(smallcaps("assistant banned")+"\n"+smallcaps("unban")+" @"+assistantUsername), nil)
			return nil
		}
		if !ok {
			_ = editHTML(pm, wrapBQ(smallcaps("assistant is joining...")), nil)
			if !tryJoinAssistant(chatID, pm) {
				return nil
			}
		}
		_ = promoteAssistant(chatID)
	}
	return RoomPlay(chatID, song, false, pm)
}
