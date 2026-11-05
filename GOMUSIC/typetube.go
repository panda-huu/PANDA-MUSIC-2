package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/playeon/typetube-go"
)

var (
	typeTubeOnce sync.Once
	typeTube     *typetube.Client
)

func typeTubeClient() *typetube.Client {
	typeTubeOnce.Do(func() {
		// TypeTube public Singapore cluster; public access does not need an API key.
		typeTube = typetube.NewClient(
			typetube.WithHost(typetube.DefaultHost),
			typetube.WithTimeout(20*time.Second),
		)
	})
	return typeTube
}

func searchYT(query string) (urlStr, title, duration, thumbnail string, playlist []ytItem, err error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return "", "", "", "", nil, fmt.Errorf("empty search query")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := typeTubeClient()

	if strings.Contains(q, "youtube.com/") || strings.Contains(q, "youtu.be/") || len(q) == 11 {
		track, resolveErr := client.Resolve(ctx, q)
		if resolveErr != nil {
			return "", "", "", "", nil, fmt.Errorf("TypeTube resolve: %w", resolveErr)
		}
		if track == nil || strings.TrimSpace(track.ID) == "" {
			return "", "", "", "", nil, fmt.Errorf("TypeTube returned empty track metadata")
		}
		link := "https://www.youtube.com/watch?v=" + track.ID
		return link, track.Title, formatClock(float64(track.DurationSeconds)), thumbFor(link, track.Thumbnail), nil, nil
	}

	results, searchErr := client.Search(ctx, q, 5)
	if searchErr != nil {
		return "", "", "", "", nil, fmt.Errorf("TypeTube search: %w", searchErr)
	}
	if len(results) == 0 {
		return "", "", "", "", nil, fmt.Errorf("TypeTube: no search results")
	}

	// Keep all TypeTube search results. Returning only results[0] made
	// autoplay repeatedly hit the same top result for title-based queries.
	playlist = make([]ytItem, 0, len(results))
	for _, item := range results {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = extractVideoID(item.URL)
		}
		if len(id) != 11 {
			continue
		}
		link := "https://www.youtube.com/watch?v=" + id
		dur := "0:00"
		if item.Duration > 0 {
			dur = formatClock(float64(item.Duration))
		}
		playlist = append(playlist, ytItem{
			Link: link, Title: item.Title, Duration: dur,
			Thumbnail: thumbFor(link, ""),
		})
	}
	if len(playlist) == 0 {
		return "", "", "", "", nil, fmt.Errorf("TypeTube returned no valid video IDs")
	}
	first := playlist[0]
	return first.Link, first.Title, first.Duration, first.Thumbnail, playlist, nil
}

// streamHeight reads the real pixel height from TypeTube's quality/resolution
// fields. The resolution field is preferred because quality labels vary.
func streamHeight(resolution, quality string) int {
	for _, value := range []string{resolution, quality} {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if x := strings.LastIndex(value, "x"); x >= 0 && x+1 < len(value) {
			if h, err := strconv.Atoi(strings.TrimLeft(value[x+1:], " p")); err == nil && h > 0 {
				return h
			}
		}
		digits := ""
		for _, r := range value {
			if r >= '0' && r <= '9' {
				digits += string(r)
			}
		}
		if h, err := strconv.Atoi(digits); err == nil && h > 0 {
			return h
		}
	}
	return 0
}

// TypeTube's selector returns the first stream <= target, not necessarily the
// highest one. Sort descending and remove streams above 1080p before calling it.
func prepare1080pTrack(track *typetube.TrackResult) (int, error) {
	if track == nil {
		return 0, fmt.Errorf("TypeTube returned empty track metadata")
	}
	streams := make([]typetube.VideoStream, 0, len(track.VideoStreams))
	for _, stream := range track.VideoStreams {
		height := streamHeight(stream.Resolution, stream.Quality)
		if height > 0 && height <= 1080 && strings.TrimSpace(stream.URL) != "" {
			streams = append(streams, stream)
		}
	}
	sort.SliceStable(streams, func(i, j int) bool {
		return streamHeight(streams[i].Resolution, streams[i].Quality) >
			streamHeight(streams[j].Resolution, streams[j].Quality)
	})
	if len(streams) > 0 {
		track.VideoStreams = streams
		track.BestVideo = &track.VideoStreams[0]
		return streamHeight(track.BestVideo.Resolution, track.BestVideo.Quality), nil
	}

	if track.BestVideo != nil && strings.TrimSpace(track.BestVideo.URL) != "" {
		height := streamHeight(track.BestVideo.Resolution, track.BestVideo.Quality)
		if height > 0 && height <= 1080 {
			return height, nil
		}
	}
	return 0, fmt.Errorf("TypeTube has no video stream at or below 1080p for this video")
}

func probeVideoHeight(path string) int {
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=height", "-of", "default=noprint_wrappers=1:nokey=1", path).Output()
	if err != nil {
		return 0
	}
	height, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return height
}

func downloadTypeTube(raw string, video bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	client := typeTubeClient()
	track, err := client.Resolve(ctx, raw)
	if err != nil {
		return "", fmt.Errorf("TypeTube resolve failed: %w", err)
	}
	if track == nil || strings.TrimSpace(track.ID) == "" {
		return "", fmt.Errorf("TypeTube returned empty track metadata")
	}

	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return "", err
	}

	ext := ".audio.m4a"
	targetHeight := 0
	if video {
		ext = ".video.mp4"
		targetHeight, err = prepare1080pTrack(track)
		if err != nil {
			return "", err
		}
		log.Printf("[TypeTube] selected video height=%dp title=%q", targetHeight, track.Title)
	}
	dest := filepath.Join(downloadDir, track.ID+ext)

	if st, statErr := os.Stat(dest); statErr == nil && st.Size() > 1024 {
		if !video {
			return dest, nil
		}
		// Reuse only a cache matching the currently selected source resolution.
		if fileHasVideo(dest) && probeVideoHeight(dest) == targetHeight {
			return dest, nil
		}
		_ = os.Remove(dest)
	}

	cfg := typetube.DownloadConfig{
		Workers:        6,
		ChunkSizeBytes: 10 * 1024 * 1024,
	}
	var result *typetube.DownloadResult
	if video {
		cfg.Quality = "1080p"
		result, err = client.DownloadMedia(ctx, track, dest, cfg)
	} else {
		cfg.Quality = "highest"
		result, err = client.DownloadAudio(ctx, track, dest, cfg)
	}
	if err != nil {
		_ = os.Remove(dest)
		return "", fmt.Errorf("TypeTube download failed: %w", err)
	}
	if result == nil || result.FilePath == "" {
		_ = os.Remove(dest)
		return "", fmt.Errorf("TypeTube returned an empty download result")
	}

	st, err := os.Stat(result.FilePath)
	if err != nil || st.Size() < 1024 {
		_ = os.Remove(result.FilePath)
		return "", fmt.Errorf("TypeTube downloaded an empty or invalid file")
	}
	if video {
		if !fileHasVideo(result.FilePath) {
			_ = os.Remove(result.FilePath)
			return "", fmt.Errorf("TypeTube result has no video stream")
		}
		actualHeight := probeVideoHeight(result.FilePath)
		if actualHeight != targetHeight {
			_ = os.Remove(result.FilePath)
			return "", fmt.Errorf("video resolution mismatch: requested best available %dp, downloaded %dp", targetHeight, actualHeight)
		}
		log.Printf("[TypeTube] verified video resolution=%dp", actualHeight)
	}
	log.Printf("[TypeTube] saved %s title=%q bytes=%d video=%t", result.FilePath, track.Title, st.Size(), video)
	return result.FilePath, nil
}
