package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const downloadDir = "downloads"

var ytIDRe = regexp.MustCompile(`(?:v=|youtu\.be/|/vi/)([A-Za-z0-9_-]{11})`)

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

var thumbClient = &http.Client{Timeout: 20 * time.Second}

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
	resp, err := thumbClient.Get(src)
	if err != nil || resp == nil {
		return src
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
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

func resolveDirectURL(raw string, video bool) (streamSrc, error) {
	if st, err := os.Stat(raw); err == nil && !st.IsDir() && st.Size() > 1024 {
		if video && !fileHasVideo(raw) {
			return streamSrc{}, fmt.Errorf("local file has no video stream")
		}
		return streamSrc{File: raw, Audio: raw, Video: raw}, nil
	}
	path, err := resolveStream(raw, video)
	if err != nil {
		return streamSrc{}, err
	}
	return streamSrc{File: path, Audio: path, Video: path}, nil
}

func resolveStream(raw string, video bool) (string, error) {
	if st, err := os.Stat(raw); err == nil && !st.IsDir() && st.Size() > 1024 {
		if video && !fileHasVideo(raw) {
			return "", fmt.Errorf("local file has no video stream")
		}
		return raw, nil
	}
	return downloadTypeTube(raw, video)
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
