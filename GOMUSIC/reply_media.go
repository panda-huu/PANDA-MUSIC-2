package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

var playOnce sync.Map

func claimPlay(m *telegram.NewMessage) bool {
	if m == nil || m.Message == nil {
		return true
	}
	key := fmt.Sprintf("%d:%d", m.ChatID(), m.Message.ID)
	if _, loaded := playOnce.LoadOrStore(key, time.Now()); loaded {
		return false
	}
	return true
}

func replyMsgID(m *telegram.NewMessage) int32 {
	if m == nil {
		return 0
	}
	if id := m.ReplyToMsgID(); id != 0 {
		return id
	}
	if m.Message == nil || m.Message.ReplyTo == nil {
		return 0
	}
	switch hdr := m.Message.ReplyTo.(type) {
	case *telegram.MessageReplyHeaderObj:
		if hdr != nil && hdr.ReplyToMsgID != 0 {
			return hdr.ReplyToMsgID
		}
	}
	s := fmt.Sprintf("%v", m.Message.ReplyTo)
	for _, key := range []string{"ReplyToMsgID:", "reply_to_msg_id:"} {
		if i := strings.Index(s, key); i >= 0 {
			var id int32
			fmt.Sscanf(s[i+len(key):], "%d", &id)
			if id != 0 {
				return id
			}
		}
	}
	return 0
}

func playableMessage(m *telegram.NewMessage) bool {
	if m == nil {
		return false
	}
	if m.Audio() != nil || m.Video() != nil || m.Voice() != nil || m.Document() != nil {
		return true
	}
	if m.File != nil && strings.TrimSpace(m.File.Name) != "" {
		return true
	}
	mt := strings.ToLower(m.MediaType())
	return strings.Contains(mt, "audio") || strings.Contains(mt, "video") || strings.Contains(mt, "voice") || strings.Contains(mt, "document")
}

func fetchByID(chatID int64, id int32) *telegram.NewMessage {
	if id == 0 || chatID == 0 {
		return nil
	}
	for _, c := range []*telegram.Client{Bot, Assistant} {
		if c == nil {
			continue
		}
		msgs, err := c.GetMessages(chatID, &telegram.SearchOption{IDs: []int32{id}})
		if err == nil && len(msgs) > 0 {
			return &msgs[0]
		}
	}
	return nil
}

func replyTarget(m *telegram.NewMessage) (*telegram.NewMessage, bool) {
	if m == nil {
		return nil, false
	}
	if playableMessage(m) {
		return m, true
	}
	replyID := replyMsgID(m)
	if replyID == 0 {
		return nil, false
	}
	chatID := m.ChannelID()
	if chatID == 0 {
		chatID = m.ChatID()
	}
	if got := fetchByID(chatID, replyID); got != nil {
		return got, true
	}
	if m.ChatID() != chatID {
		if got := fetchByID(m.ChatID(), replyID); got != nil {
			return got, true
		}
	}
	if reply, err := m.GetReplyMessage(); err == nil && reply != nil {
		return reply, true
	}
	log.Println("reply fetch fail", replyID, "chat", chatID)
	return nil, true
}

func songFromReply(m *telegram.NewMessage, video bool) (*Song, string) {
	if m == nil {
		return nil, ""
	}
	target, found := replyTarget(m)
	if !found {
		return nil, ""
	}
	if target == nil {
		return nil, "could not read the replied message"
	}
	kind, title := replyMediaKind(target)
	log.Println("reply media kind", kind, "title", title, "type", target.MediaType())
	if kind == "" {
		if video {
			return nil, "reply to a video or audio file for /vplay"
		}
		return nil, "reply to an audio file for /play"
	}
	wantVideo := video && kind == "video"
	_ = os.MkdirAll(downloadDir, 0o755)
	name := "audio.mp3"
	if wantVideo {
		name = "video.mp4"
	}
	if target.File != nil && strings.TrimSpace(target.File.Name) != "" {
		name = safeMediaName(target.File.Name)
		if title == "" || title == "audio" || title == "video" || title == "voice" {
			title = strings.TrimSuffix(name, filepath.Ext(name))
		}
		if filepath.Ext(name) == "" {
			if wantVideo {
				name += ".mp4"
			} else {
				name += ".mp3"
			}
		}
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
		URL: path, FilePath: path, Title: title, Duration: dur, DurationSeconds: secs,
		Requester: userNameOf(m), RequesterID: userIDOf(m), Video: wantVideo,
	}, ""
}

func replyMediaKind(reply *telegram.NewMessage) (kind, title string) {
	if reply == nil {
		return "", ""
	}
	dump := strings.ToLower(reply.MediaType() + " " + fmt.Sprintf("%v", reply.Media()))
	if reply.File != nil {
		dump += " " + strings.ToLower(reply.File.Name)
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
		if k, t := kindFromName(mediaTitle(reply, doc, ""), doc); k != "" {
			return k, t
		}
	}
	if k, t := kindFromName(dump, nil); k != "" {
		return k, t
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
	for _, ext := range []string{".mp3", ".m4a", ".aac", ".ogg", ".opus", ".wav", ".flac", ".oga", ".wma"} {
		if strings.Contains(low, ext) {
			return "audio", mediaTitle(nil, doc, "audio")
		}
	}
	for _, ext := range []string{".mp4", ".mkv", ".webm", ".mov", ".avi", ".m4v", ".3gp"} {
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
	if reply != nil && reply.File != nil && strings.TrimSpace(reply.File.Name) != "" {
		return strings.TrimSuffix(strings.TrimSpace(reply.File.Name), filepath.Ext(reply.File.Name))
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
