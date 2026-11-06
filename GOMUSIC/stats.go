package main

import (
	"fmt"
	"os"
	"log"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

func statsKeyboard() telegram.ReplyMarkup {
	kb := telegram.NewKeyboard()
	kb.AddRow(
		telegram.Button.Data("❖ GENERAL ❖", "stats_general"),
		telegram.Button.Data("❖ OVERALL ❖", "stats_overall"),
	)
	kb.AddRow(telegram.Button.Data("CLOSE", "close"))
	return kb.Build()
}

func statsGib(n uint64) string { return fmt.Sprintf("%.2f", float64(n)/(1024*1024*1024)) }

func statsModuleCount() int {
	entries, err := os.ReadDir("GOMUSIC")
	if err != nil { return 0 }
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") { n++ }
	}
	return n
}

func statsMemory() (used, total uint64) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil { return 0, 0 }
	var totalKB, availKB uint64
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 { continue }
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal": totalKB = v
		case "MemAvailable": availKB = v
		}
	}
	if totalKB == 0 { return 0, 0 }
	return (totalKB-availKB)*1024, totalKB*1024
}

func statsDisk() (total, used, free uint64) {
	var s syscall.Statfs_t
	if err := syscall.Statfs("/", &s); err != nil { return 0, 0, 0 }
	total = s.Blocks * uint64(s.Bsize)
	free = s.Bavail * uint64(s.Bsize)
	used = total - s.Bfree*uint64(s.Bsize)
	return
}

func statsOverallText() string {
	assistant := "offline"
	if Assistant != nil { assistant = "online" }
	duration := MaxDurationSeconds / 60
	return richHeading("❖ "+richEsc(BotName)+" sᴛᴀᴛs ᴀɴᴅ ɪɴғᴏʀᴍᴀᴛɪᴏɴ :", 3) + "\n" +
		richKVTable([][2]string{
			{"ᴀssɪsᴛᴀɴᴛ", "<code>"+assistant+"</code>"},
			{"ʙʟᴏᴄᴋᴇᴅ ɢʀᴏᴜᴘs", fmt.Sprintf("<code>%d</code>", len(getBlockedGroups()))},
			{"ʙʟᴏᴄᴋᴇᴅ ᴜsᴇʀs", fmt.Sprintf("<code>%d</code>", len(getBlockedUsers()))},
			{"ᴄʜᴀᴛs", fmt.Sprintf("<code>%d</code>", getServedChatsCount())},
			{"ᴜsᴇʀs", fmt.Sprintf("<code>%d</code>", getServedUsersCount())},
			{"ᴘʟᴀʏs", fmt.Sprintf("<code>%d</code>", getTotalPlays())},
			{"ᴍᴏᴅᴜʟᴇs", fmt.Sprintf("<code>%d</code>", statsModuleCount())},
			{"ᴜᴘᴛɪᴍᴇ", "<code>"+time.Since(botStartTime).Truncate(time.Second).String()+"</code>"},
			{"ᴘʟᴀʏ ᴅᴜʀᴀᴛɪᴏɴ ʟɪᴍɪᴛ", fmt.Sprintf("<code>%d minutes</code>", duration)},
		})
}

func statsGeneralText() string {
	usedRAM, totalRAM := statsMemory()
	diskTotal, diskUsed, diskFree := statsDisk()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	assistant := "offline"
	if Assistant != nil { assistant = "online" }
	return richHeading("❖ "+richEsc(BotName)+" sᴛᴀᴛs ᴀɴᴅ ɪɴғᴏʀᴍᴀᴛɪᴏɴ :", 3) + "\n" +
		richKVTable([][2]string{
			{"ᴍᴏᴅᴜʟᴇs", fmt.Sprintf("<code>%d Go files</code>", statsModuleCount())},
			{"ᴘʟᴀᴛғᴏʀᴍ", "<code>"+runtime.GOOS+"/"+runtime.GOARCH+"</code>"},
			{"ʀᴀᴍ", fmt.Sprintf("<code>%s / %s GiB</code>", statsGib(usedRAM), statsGib(totalRAM))},
			{"ᴄᴘᴜ ᴄᴏʀᴇs", fmt.Sprintf("<code>%d</code>", runtime.NumCPU())},
			{"ɢᴏ", "<code>"+runtime.Version()+"</code>"},
			{"ᴀssɪsᴛᴀɴᴛ", "<code>"+assistant+"</code>"},
			{"ʙᴏᴛ ᴘʀᴏᴄᴇss ʀᴀᴍ", fmt.Sprintf("<code>%.2f MiB</code>", float64(ms.Alloc)/1024/1024)},
			{"sᴛᴏʀᴀɢᴇ ᴛᴏᴛᴀʟ", "<code>"+statsGib(diskTotal)+" GiB</code>"},
			{"sᴛᴏʀᴀɢᴇ ᴜsᴇᴅ", "<code>"+statsGib(diskUsed)+" GiB</code>"},
			{"sᴛᴏʀᴀɢᴇ ʟᴇғᴛ", "<code>"+statsGib(diskFree)+" GiB</code>"},
			{"sᴇʀᴠᴇᴅ ᴄʜᴀᴛs", fmt.Sprintf("<code>%d</code>", getServedChatsCount())},
			{"sᴇʀᴠᴇᴅ ᴜsᴇʀs", fmt.Sprintf("<code>%d</code>", getServedUsersCount())},
			{"ᴘʟᴀʏs", fmt.Sprintf("<code>%d</code>", getTotalPlays())},
			{"ᴜᴘᴛɪᴍᴇ", "<code>"+time.Since(botStartTime).Truncate(time.Second).String()+"</code>"},
		})
}

func handleStats(m *telegram.NewMessage) error {
	if blocked(m) { return nil }
	_ = addServedUser(userIDOf(m))
	if !m.IsPrivate() { addServedChat(canonChatID(m.ChatID())) }
	_, _ = sendHTML(Bot, m.ChatID(),
		"❖ <b>CLICK ON THE BUTTONS BELOW TO CHECK THE STATS OF</b>\n<b>@"+richEsc(BotName)+"</b>",
		statsKeyboard())
	return nil
}

func handleStatsCallback(cb *telegram.CallbackQuery, data string) bool {
	if data != "stats_general" && data != "stats_overall" { return false }
	text := statsGeneralText()
	if data == "stats_overall" { text = statsOverallText() }
	if err := editMenu(cb, text, statsKeyboard()); err != nil {
		// A text message may not have a caption; editMenu handles both through
		// the existing message edit helper and the fallback below.
		log.Printf("[stats] edit failed: %v", err)
	}
	_, _ = cb.Answer("")
	return true
}

var _ = filepath.Separator
