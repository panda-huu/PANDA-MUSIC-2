package main

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

var (
	APIID              int
	APIHash            string
	BotToken           string
	StringSession      string
	OwnerID            int64
	BotName            string
	BotLink            string
	UpdatesChannel     string
	SupportGroup       string
	LoggerID           int64
	PingImgURL         string
	SessionName        string
	Port               int
	StartPhotos        []string
	MaxDurationSeconds int
	QueueLimit         int
	Cooldown           int
	AruYTAPI           string
	AruAPIKey          string
)

func loadLoggerID() int64 {
	for _, k := range []string{"LOGGER_ID", "LOG_GROUP_ID"} {
		v := strings.TrimSpace(os.Getenv(k))
		v = strings.Trim(v, "\"'")
		if v == "" {
			continue
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err == nil && n != 0 {
			return n
		}
	}
	return -1003861170542
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func loadAruAPI() {
	url := firstEnv("ARU_YT_API", "ARU_YT_URL", "SHRUTI_API_URL")
	key := firstEnv("ARU_API_KEY", "ARU_YT_KEY", "SHRUTI_API_KEY")

	// nano/phone: both vars stuck on one line
	// ARU_YT_API=https://x ARU_API_KEY=PANDA-KEY
	if i := strings.Index(url, " ARU_API_KEY="); i >= 0 {
		if key == "" {
			key = strings.TrimSpace(url[i+len(" ARU_API_KEY="):])
		}
		url = strings.TrimSpace(url[:i])
	}
	if i := strings.Index(url, " ARU_YT_KEY="); i >= 0 {
		if key == "" {
			key = strings.TrimSpace(url[i+len(" ARU_YT_KEY="):])
		}
		url = strings.TrimSpace(url[:i])
	}
	url = strings.Trim(url, "\"'")
	key = strings.Trim(key, "\"'")
	if url == "" {
		url = "http://127.0.0.1:8080"
	}
	if key == "" {
		key = "PANDA-KEY"
	}
	AruYTAPI = strings.TrimRight(url, "/")
	AruAPIKey = key
	log.Printf("download API %s key=%d chars", AruYTAPI, len(AruAPIKey))
}

func loadConfig() error {
	_ = godotenv.Load()
	_ = godotenv.Load(".env")
	_ = godotenv.Load("/root/GOMUSICV2/.env")
	var err error
	APIID, err = strconv.Atoi(mustEnv("API_ID"))
	if err != nil {
		return err
	}
	APIHash = mustEnv("API_HASH")
	BotToken = mustEnv("BOT_TOKEN")
	StringSession = mustEnv("STRING_SESSION")
	OwnerID, err = strconv.ParseInt(mustEnv("OWNER_ID"), 10, 64)
	if err != nil {
		return err
	}
	BotName = envOr("BOT_NAME", "ѕρσтιfу")
	BotLink = envOr("BOT_LINK", "https://t.me/sptfyX_bot")
	UpdatesChannel = envOr("UPDATES_CHANNEL", "https://t.me/sxypndu")
	SupportGroup = envOr("SUPPORT_GROUP", "https://t.me/crzy_soul")
	LoggerID = loadLoggerID()
	PingImgURL = envOr("PING_IMG_URL", "https://files.catbox.moe/ddzvc0.jpg")
	SessionName = envOr("SESSION_NAME", "ShizuMusic")
	Port, _ = strconv.Atoi(envOr("PORT", "10000"))
	StartPhotos = splitCSV(envOr("START_PHOTOS", "https://files.catbox.moe/hzaxeq.jpg"))
	MaxDurationSeconds, _ = strconv.Atoi(envOr("MAX_DURATION_SECONDS", "1800"))
	QueueLimit, _ = strconv.Atoi(envOr("QUEUE_LIMIT", "20"))
	Cooldown, _ = strconv.Atoi(envOr("COOLDOWN", "10"))
	loadAruAPI()
	return nil
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		panic("missing required env: " + k)
	}
	return v
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{"https://files.catbox.moe/jgt2vm.png"}
	}
	return out
}
