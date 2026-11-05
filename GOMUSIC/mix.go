package main

import (
	"fmt"
	"os"
)

func fetchYouTubeMix(videoID string, limit int) []ytItem {
	videoID = extractVideoID(videoID)
	if len(videoID) != 11 {
		return nil
	}
	if limit <= 0 {
		limit = 15
	}
	body := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    "WEB",
				"clientVersion": "2.20240815.00.00",
				"hl":            "en",
			},
		},
		"playlistId": "RD" + videoID,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, "https://www.youtube.com/youtubei/v1/next?prettyPrint=false", bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	var data any
	if err := json.Unmarshal(b, &data); err != nil {
		return nil
	}
	var out []ytItem
	walkMix(data, &out, limit)
	return out
}

func walkMix(v any, out *[]ytItem, limit int) {
	if len(*out) >= limit {
		return
	}
	switch t := v.(type) {
	case map[string]any:
		if vr, ok := t["playlistPanelVideoRenderer"].(map[string]any); ok {
			id, _ := vr["videoId"].(string)
			if len(id) == 11 {
				title := titleFromMap(vr)
				dur := durationFromMap(vr)
				link := "https://www.youtube.com/watch?v=" + id
				*out = append(*out, ytItem{Link: link, Title: title, Duration: dur, Thumbnail: thumbFor(link, "")})
			}
			return
		}
		for _, c := range t {
			walkMix(c, out, limit)
			if len(*out) >= limit {
				return
			}
		}
	case []any:
		for _, c := range t {
			walkMix(c, out, limit)
			if len(*out) >= limit {
				return
			}
		}
	}
}

func downloadWithRetry(raw string, video bool) (streamSrc, error) {
	var last error
	for i := 0; i < 3; i++ {
		src, err := resolveDirectURL(raw, video)
		if err == nil {
			if src.File != "" && !isHTTP(src.File) {
				st, e := os.Stat(src.File)
				if e != nil || st.Size() < 1024 {
					last = fmt.Errorf("invalid or empty file")
					continue
				}
			}
			return src, nil
		}
		last = err
	}
	if last == nil {
		last = fmt.Errorf("download failed")
	}
	return streamSrc{}, last
}
