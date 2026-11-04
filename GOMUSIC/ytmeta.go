package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func probeDuration(path string) float64 {
	if strings.TrimSpace(path) == "" {
		return 0
	}
	try := [][]string{
		{"-show_entries", "format=duration"},
		{"-select_streams", "a:0", "-show_entries", "stream=duration"},
		{"-select_streams", "v:0", "-show_entries", "stream=duration"},
	}
	for _, extra := range try {
		args := append([]string{"-v", "error"}, extra...)
		args = append(args, "-of", "default=noprint_wrappers=1:nokey=1", path)
		out, err := exec.Command("ffprobe", args...).Output()
		if err != nil {
			continue
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		if err == nil && n > 1 {
			return n
		}
	}
	return 0
}

func formatClock(secs float64) string {
	if secs < 0 {
		secs = 0
	}
	t := int(secs + 0.5)
	h := t / 3600
	m := (t % 3600) / 60
	s := t % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

func oembedTitle(id string) string {
	if len(id) != 11 {
		return ""
	}
	u := "https://www.youtube.com/oembed?format=json&url=https://www.youtube.com/watch?v=" + id
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var data struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return ""
	}
	return strings.TrimSpace(data.Title)
}

func searchSmart(query string) (string, string, string, string, []ytItem, error) {
	return searchYT(query)
}
