package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/playeon/typetube-go"
)

var (
	typeTubeOnce sync.Once
	typeTube      *typetube.Client
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

	item := results[0]
	id := strings.TrimSpace(item.ID)
	if id == "" {
		id = extractVideoID(item.URL)
	}
	if len(id) != 11 {
		return "", "", "", "", nil, fmt.Errorf("TypeTube returned invalid video ID")
	}
	link := "https://www.youtube.com/watch?v=" + id
	dur := "0:00"
	if item.Duration > 0 {
		dur = formatClock(float64(item.Duration))
	}
	return link, item.Title, dur, thumbFor(link, ""), nil, nil
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
	if video {
		ext = ".video.mp4"
	}
	dest := filepath.Join(downloadDir, track.ID+ext)
	if st, statErr := os.Stat(dest); statErr == nil && st.Size() > 1024 {
		if !video || fileHasVideo(dest) {
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
	if video && !fileHasVideo(result.FilePath) {
		_ = os.Remove(result.FilePath)
		return "", fmt.Errorf("TypeTube result has no video stream")
	}
	log.Printf("[TypeTube] saved %s title=%q bytes=%d video=%t", result.FilePath, track.Title, st.Size(), video)
	return result.FilePath, nil
}
