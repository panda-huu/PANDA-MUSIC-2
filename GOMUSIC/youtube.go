package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const downloadDir = "downloads"

var (
	ytIDRe    = regexp.MustCompile(`(?:v=|youtu\.be/)([A-Za-z0-9_-]{11})`)
	htmlIDRe  = regexp.MustCompile(`"videoId":"([A-Za-z0-9_-]{11})"`)
	htmlTitle = regexp.MustCompile(`"title":\{"runs":\[\{"text":"([^"]+)"`)
)

type ytItem struct {
	Link, Title, Duration, Thumbnail string
}

type streamSrc struct {
	Audio string
	Video string
	File  string
}

func extractVideoID(raw string) string {
	raw = strings.TrimSpace(raw)
	if m := ytIDRe.FindStringSubmatch(raw); len(m) > 1 {
		return m[1]
	}
	if len(raw) == 11 && !strings.Contains(raw, " ") {
		return raw
	}
	return raw
}

func thumbFor(raw, existing string) string {
	existing = strings.TrimSpace(existing)
	if existing != "" && existing != "NA" && strings.HasPrefix(existing, "http") {
		return existing
	}
	id := extractVideoID(raw)
	if len(id) == 11 {
		return "https://i.ytimg.com/vi/" + id + "/hqdefault.jpg"
	}
	return ""
}

func cacheThumb(src string) string {
	if src == "" {
		return ""
	}
	if st, err := os.Stat(src); err == nil && !st.IsDir() && st.Size() > 0 {
		return src
	}
	_ = os.MkdirAll(downloadDir, 0o755)
	id := extractVideoID(src)
	name := "thumb.jpg"
	if len(id) == 11 {
		name = "t_" + id + ".jpg"
	}
	dest := filepath.Join(downloadDir, name)
	if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
		return dest
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(src)
	if err != nil {
		return src
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return src
	}
	f, err := os.Create(dest)
	if err != nil {
		return src
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = os.Remove(dest)
		return src
	}
	return dest
}

func fileHasVideo(path string) bool {
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_type", "-of", "csv=p=0", path).Output()
	return err == nil && strings.Contains(strings.ToLower(string(out)), "video")
}

func searchYT(query string) (urlStr, title, durISO, thumb string, playlist []ytItem, err error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "", "", "", "", nil, fmt.Errorf("empty query")
	}
	if strings.Contains(q, "youtube.com") || strings.Contains(q, "youtu.be") {
		id := extractVideoID(q)
		link := "https://www.youtube.com/watch?v=" + id
		return link, "YouTube Video", "0:00", thumbFor(link, ""), nil, nil
	}
	if item, e := searchInnertube(q); e == nil && item.Link != "" {
		return item.Link, item.Title, item.Duration, item.Thumbnail, nil, nil
	}
	if item, e := searchYouTubeHTML(q); e == nil && item.Link != "" {
		return item.Link, item.Title, item.Duration, item.Thumbnail, nil, nil
	}
	return "", "", "", "", nil, fmt.Errorf("search failed")
}

func searchInnertube(query string) (ytItem, error) {
	body := map[string]any{
		"context": map[string]any{"client": map[string]any{"clientName": "WEB", "clientVersion": "2.20240815.00.00", "hl": "en"}},
		"query":   query,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, "https://www.youtube.com/youtubei/v1/search?prettyPrint=false", bytes.NewReader(raw))
	if err != nil {
		return ytItem{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ytItem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return ytItem{}, fmt.Errorf("innertube %d", resp.StatusCode)
	}
	var data any
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ytItem{}, err
	}
	id, title, dur := walkYTSearch(data)
	if id == "" {
		return ytItem{}, fmt.Errorf("no video in innertube")
	}
	link := "https://www.youtube.com/watch?v=" + id
	return ytItem{Link: link, Title: title, Duration: dur, Thumbnail: thumbFor(link, "")}, nil
}

func walkYTSearch(v any) (id, title, dur string) {
	switch t := v.(type) {
	case map[string]any:
		if _, isVR := t["videoRenderer"]; isVR {
			vr := t["videoRenderer"].(map[string]any)
			if vid, ok := vr["videoId"].(string); ok && len(vid) == 11 {
				return vid, titleFromMap(vr), durationFromMap(vr)
			}
		}
		for _, c := range t {
			if id, title, dur = walkYTSearch(c); id != "" {
				return
			}
		}
	case []any:
		for _, c := range t {
			if id, title, dur = walkYTSearch(c); id != "" {
				return
			}
		}
	}
	return "", "", ""
}

func titleFromMap(m map[string]any) string {
	if t, ok := m["title"].(map[string]any); ok {
		if s, ok := t["simpleText"].(string); ok && s != "" {
			return s
		}
		if runs, ok := t["runs"].([]any); ok && len(runs) > 0 {
			if r, ok := runs[0].(map[string]any); ok {
				if s, ok := r["text"].(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return "YouTube Video"
}

func durationFromMap(m map[string]any) string {
	if t, ok := m["lengthText"].(map[string]any); ok {
		if s, ok := t["simpleText"].(string); ok && s != "" {
			return s
		}
	}
	return "0:00"
}

func searchYouTubeHTML(query string) (ytItem, error) {
	u := "https://www.youtube.com/results?search_query=" + url.QueryEscape(query) + "&hl=en"
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ytItem{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	idm := htmlIDRe.FindSubmatch(b)
	if len(idm) < 2 {
		return ytItem{}, fmt.Errorf("no videoId in html")
	}
	id := string(idm[1])
	title := "YouTube Video"
	if tm := htmlTitle.FindSubmatch(b); len(tm) > 1 {
		title = string(tm[1])
	}
	link := "https://www.youtube.com/watch?v=" + id
	return ytItem{Link: link, Title: title, Duration: "0:00", Thumbnail: thumbFor(link, "")}, nil
}

func resolveDirectURL(raw string, video bool) (streamSrc, error) {
	if st, err := os.Stat(raw); err == nil && !st.IsDir() && st.Size() > 1024 {
		return streamSrc{File: raw, Audio: raw, Video: raw}, nil
	}
	path, err := resolveStream(raw, video)
	if err != nil {
		return streamSrc{}, err
	}
	return streamSrc{File: path, Audio: path, Video: path}, nil
}

func resolveStream(raw string, video bool) (string, error) {
	if st, err := os.Stat(raw); err == nil && !st.IsDir() {
		return raw, nil
	}
	vid := extractVideoID(raw)
	_ = os.MkdirAll(downloadDir, 0o755)
	typ, dest := "audio", filepath.Join(downloadDir, vid+".audio.m4a")
	if video {
		typ, dest = "video", filepath.Join(downloadDir, vid+".video.mp4")
	}
	if st, err := os.Stat(dest); err == nil && st.Size() > 1024 {
		if !video || fileHasVideo(dest) {
			return dest, nil
		}
		_ = os.Remove(dest)
	}
	if video {
		for _, p := range []string{
			filepath.Join(downloadDir, vid+".video.mp4"),
			filepath.Join(downloadDir, vid+".mp4"),
		} {
			if st, err := os.Stat(p); err == nil && st.Size() > 1024 && fileHasVideo(p) {
				return p, nil
			}
			if _, err := os.Stat(p); err == nil && !fileHasVideo(p) {
				_ = os.Remove(p)
			}
		}
	} else {
		for _, ext := range []string{".audio.m4a", ".audio.mp3", ".m4a", ".mp3"} {
			p := filepath.Join(downloadDir, vid+ext)
			if st, err := os.Stat(p); err == nil && st.Size() > 1024 {
				return p, nil
			}
		}
	}
	path, err := downloadViaAPI(vid, typ, dest)
	if err != nil {
		return "", fmt.Errorf("download API: %v", err)
	}
	if isHTTP(path) {
		return path, nil
	}
	if video && !fileHasVideo(path) {
		_ = os.Remove(path)
		return "", fmt.Errorf("API gave audio file, not video stream")
	}
	return path, nil
}

// videoQualityFallbacks tried in order for video downloads.
// "worst" removed — the API rejects it with 400 (quality must be between 144 and 1080).
var videoQualityFallbacks = []string{"360", "240", "144"}

// isFormatUnavailable reports whether the API error means the video has no
// available format at all — so retrying other qualities is pointless.
func isFormatUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "requested format is not available") ||
		strings.Contains(msg, "format is not available")
}

func downloadViaAPI(videoID, typ, dest string) (string, error) {
	api := strings.TrimRight(AruYTAPI, "/")
	key := strings.TrimSpace(AruAPIKey)
	if api == "" {
		api = "https://aru.up.railway.app"
	}
	if key == "" {
		key = "PANDA"
	}
	full := "https://www.youtube.com/watch?v=" + videoID

	if typ == "video" {
		var lastErr error
		for _, q := range videoQualityFallbacks {
			u, _ := url.Parse(api + "/download")
			qv := u.Query()
			qv.Set("url", full)
			qv.Set("type", "video")
			qv.Set("quality", q)
			qv.Set("api_key", key)
			u.RawQuery = qv.Encode()
			log.Printf("download API request %s type=video quality=%s", api+"/download", q)
			path, err := doAPIDownload(u.String(), dest)
			if err == nil {
				return path, nil
			}
			log.Printf("download API quality=%s failed: %v", q, err)
			lastErr = err
			_ = os.Remove(dest)
			// If the video has no available formats at all, bail out immediately.
			// Trying lower qualities won't help — yt-dlp can't download what YouTube
			// doesn't provide, and we'd just spam the API for nothing.
			if isFormatUnavailable(err) {
				return "", fmt.Errorf("video unavailable: no downloadable format found")
			}
		}
		return "", lastErr
	}

	// Audio path — unchanged.
	u, _ := url.Parse(api + "/download")
	qv := u.Query()
	qv.Set("url", full)
	qv.Set("type", "audio")
	qv.Set("format", "m4a")
	qv.Set("api_key", key)
	u.RawQuery = qv.Encode()
	log.Printf("download API request %s type=audio", api+"/download")
	return doAPIDownload(u.String(), dest)
}

// doAPIDownload performs one GET to the download API and saves the body to dest.
func doAPIDownload(apiURL, dest string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Minute}
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		return "", fmt.Errorf("API HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(apiURL, "type=video") && strings.HasPrefix(ct, "audio/") {
		io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("API gave audio file, not video stream")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		_ = os.Remove(dest)
		return "", err
	}
	if n < 1024 {
		_ = os.Remove(dest)
		return "", fmt.Errorf("empty API file")
	}
	log.Printf("download API saved %s bytes=%d", dest, n)
	return dest, nil
}

func deleteFile(path string) {
	if path != "" && !isHTTP(path) {
		_ = os.Remove(path)
	}
}

type prefetchResult struct {
	src streamSrc
	err error
}

var prefetchMu sync.Mutex
var prefetching = map[string]chan prefetchResult{}

func prefetchKey(raw string, video bool) string {
	if video {
		return raw + "|v"
	}
	return raw + "|a"
}

func startPrefetch(raw string, video bool) {
	key := prefetchKey(raw, video)
	prefetchMu.Lock()
	if _, ok := prefetching[key]; ok {
		prefetchMu.Unlock()
		return
	}
	ch := make(chan prefetchResult, 1)
	prefetching[key] = ch
	prefetchMu.Unlock()
	go func() {
		src, err := downloadWithRetry(raw, video)
		ch <- prefetchResult{src, err}
	}()
}

func takePrefetch(raw string, video bool) (streamSrc, error) {
	key := prefetchKey(raw, video)
	prefetchMu.Lock()
	ch := prefetching[key]
	if ch != nil {
		delete(prefetching, key)
	}
	prefetchMu.Unlock()
	if ch == nil {
		return streamSrc{}, fmt.Errorf("no prefetch")
	}
	res := <-ch
	return res.src, res.err
}
