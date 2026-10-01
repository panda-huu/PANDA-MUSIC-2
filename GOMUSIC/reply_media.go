package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

func songFromReply(m *telegram.NewMessage, video bool) (*Song, string) {
	if m == nil || !m.IsReply() {
		return nil, ""
	}
	reply, err := m.GetReplyMessage()
	if err != nil || reply == nil {
		return nil, ""
	}
	kind, title := replyMediaKind(reply)
	if kind == "" {
		return nil, ""
	}
	if video && kind != "video" {
		return nil, "reply to a video for /vplay"
	}
	if !video && kind != "audio" {
		return nil, "reply to an audio file for /play"
	}
	_ = os.MkdirAll(downloadDir, 0o755)
	name := "media"
	if f := reply.File(); f != nil && strings.TrimSpace(f.Name) != "" {
		name = safeMediaName(f.Name)
		if title == "" || title == "audio" || title == "video" {
			title = strings.TrimSuffix(name, filepath.Ext(name))
		}
	} else if video {
		name = "video.mp4"
	} else {
		name = "audio.mp3"
	}
	dest := filepath.Join(downloadDir, fmt.Sprintf("tg_%d_%s", time.Now().UnixNano(), name))
	path, err := reply.Download(&telegram.DownloadOptions{FileName: dest})
	if err != nil || path == "" {
		if err == nil {
			err = fmt.Errorf("empty download")
		}
		return nil, "download failed: " + err.Error()
	}
	if st, e := os.Stat(path); e != nil || st.Size() < 64 {
		return nil, "downloaded file is empty"
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
	if doc := reply.Audio(); doc != nil {
		return "audio", mediaTitle(reply, doc, "audio")
	}
	if doc := reply.Video(); doc != nil {
		return "video", mediaTitle(reply, doc, "video")
	}
	if doc := reply.Document(); doc != nil {
		mime := strings.ToLower(doc.MimeType)
		switch {
		case strings.HasPrefix(mime, "audio/"), mime == "application/ogg":
			return "audio", mediaTitle(reply, doc, "audio")
		case strings.HasPrefix(mime, "video/"):
			return "video", mediaTitle(reply, doc, "video")
		}
	}
	return "", ""
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
	chatID := m.ChatID()
	already := isBusy(chatID)
	var pm *telegram.NewMessage
	if already {
		pm, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("adding to queue...")), nil)
	} else {
		pm, _ = sendHTML(Bot, chatID, wrapBQ(smallcaps("processing...")), nil)
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
