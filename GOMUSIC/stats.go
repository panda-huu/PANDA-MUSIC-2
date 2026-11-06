package main

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"

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

func statsGib(n uint64) string {
	return fmt.Sprintf("%.2f", float64(n)/(1024*1024*1024))
}

func statsModuleCount() int {
	entries, err := os.ReadDir("GOMUSIC")
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			n++
		}
	}
	return n
}

func statsAssistantCount() int {
	if Assistant != nil {
		return 1
	}
	return 0
}

func statsSudoCount() int {
	if OwnerID > 0 {
		return 1
	}
	return 0
}

func statsBlockedCount() int {
	return len(getBlockedGroups()) + len(getBlockedUsers())
}

func statsMemory() (used, total uint64) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	var totalKB, availKB uint64
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			totalKB = v
		case "MemAvailable":
			availKB = v
		}
	}
	if totalKB == 0 {
		return 0, 0
	}
	return (totalKB - availKB) * 1024, totalKB * 1024
}

func statsCPUInfo() (physical int, frequency string) {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return 0, "N/A"
	}
	cores := make(map[string]struct{})
	frequency = "N/A"
	processor := 0
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "processor":
			processor++
		case "core id":
			cores[value] = struct{}{}
		case "cpu MHz":
			if frequency == "N/A" {
				if f, err := strconv.ParseFloat(value, 64); err == nil {
					frequency = fmt.Sprintf("%d MHz", int(f))
				}
			}
		}
	}
	if len(cores) > 0 {
		physical = len(cores)
	} else {
		physical = processor
	}
	return physical, frequency
}

func statsDisk() (total, used, free uint64) {
	var s syscall.Statfs_t
	if err := syscall.Statfs("/", &s); err != nil {
		return 0, 0, 0
	}
	total = s.Blocks * uint64(s.Bsize)
	free = s.Bavail * uint64(s.Bsize)
	used = total - s.Bfree*uint64(s.Bsize)
	return
}

func statsOverallText(username string) string {
	assistants := statsAssistantCount()
	blocked := statsBlockedCount()
	chats := getServedChatsCount()
	users := getServedUsersCount()
	modules := statsModuleCount()
	sudos := statsSudoCount()
	duration := MaxDurationSeconds / 60

	return fmt.Sprintf(
		"❖ <b>@%s</b> sᴛᴀᴛs ᴀɴᴅ ɪɴғᴏʀᴍᴀᴛɪᴏɴ :\n\n"+
			"ᴀssɪsᴛᴀɴᴛs : <code>%d</code>\n"+
			"ʙʟᴏᴄᴋᴇᴅ : <code>%d</code>\n"+
			"ᴄʜᴀᴛs: <code>%d</code>\n"+
			"ᴜsᴇʀs : <code>%d</code>\n"+
			"ᴍᴏᴅᴜʟᴇs : <code>%d</code>\n"+
			"sᴜᴅᴏᴇʀs : <code>%d</code>\n\n"+
			"ᴀᴜᴛᴏ ʟᴇᴀᴠɪɴɢ ᴀssɪsᴛᴀɴᴛ : <code>False</code>\n"+
			"ᴘʟᴀʏ ᴅᴜʀᴀᴛɪᴏɴ ʟɪᴍɪᴛ : <code>%d</code> ᴍɪɴᴜᴛᴇs",
		richEsc(username), assistants, blocked, chats, users, modules, sudos, duration,
	)
}

func statsGeneralText(username string) string {
	users := getServedUsersCount()
	chats := getServedChatsCount()
	sudos := statsSudoCount()
	modules := statsModuleCount()
	physical, cpuMHz := statsCPUInfo()

	usedRAM, totalRAM := statsMemory()
	diskTotal, diskUsed, diskFree := statsDisk()

	return fmt.Sprintf(
		"❖ <b>@%s</b> sᴛᴀᴛs ᴀɴᴅ ɪɴғᴏʀᴍᴀᴛɪᴏɴ :\n\n"+
			"❖ ᴍᴏᴅᴜʟᴇs : <code>%d</code>\n"+
			"ᴘʟᴀᴛғᴏʀᴍ : <code>%s</code>\n"+
			"ʀᴀᴍ : <code>%s / %s GiB</code>\n"+
			"ᴘʜʏsɪᴄᴀʟ ᴄᴏʀᴇs : <code>%d</code>\n"+
			"ᴛᴏᴛᴀʟ ᴄᴏʀᴇs : <code>%d</code>\n"+
			"ᴄᴘᴜ ғʀᴇǫᴜᴇɴᴄʏ : <code>%s</code>\n\n"+
			"ɢᴏ : <code>%s</code>\n"+
			"ɢᴏɢʀᴀᴍ : <code>v1.8.0-beta.3</code>\n"+
			"ɴᴛɢᴄᴀʟʟs : <code>bundled</code>\n\n"+
			"sᴛᴏʀᴀɢᴇ ᴀᴠᴀɪʟᴀʙʟᴇ : <code>%s</code> ɢɪʙ\n"+
			"sᴛᴏʀᴀɢᴇ ᴜsᴇᴅ : <code>%s</code> ɢɪʙ\n"+
			"sᴛᴏʀᴀɢᴇ ʟᴇғᴛ : <code>%s</code> ɢɪʙ\n\n"+
			"sᴇʀᴠᴇᴅ ᴄʜᴀᴛs : <code>%d</code>\n"+
			"sᴇʀᴠᴇᴅ ᴜsᴇʀs : <code>%d</code>\n"+
			"ʙʟᴏᴄᴋᴇᴅ ᴜsᴇʀs : <code>%d</code>\n"+
			"sᴜᴅᴏ ᴜsᴇʀs : <code>%d</code>\n\n"+
			"ᴛᴏᴛᴀʟ ᴅʙ sɪᴢᴇ : <code>N/A</code> ᴍʙ\n"+
			"ᴛᴏᴛᴀʟ ᴅʙ sᴛᴏʀᴀɢᴇ : <code>N/A</code> ᴍʙ\n"+
			"ᴛᴏᴛᴀʟ ᴅʙ ᴄᴏʟʟᴇᴄᴛɪᴏɴs : <code>N/A</code>\n"+
			"ᴛᴏᴛᴀʟ ᴅʙ ᴋᴇʏs : <code>N/A</code>",
		richEsc(username), modules, runtime.GOOS,
		statsGib(usedRAM), statsGib(totalRAM), physical, runtime.NumCPU(), cpuMHz,
		runtime.Version(),
		statsGib(diskTotal), statsGib(diskUsed), statsGib(diskFree),
		chats, users, len(getBlockedUsers()), sudos,
	)
}

func handleStats(m *telegram.NewMessage) error {
	if blocked(m) {
		return nil
	}

	uid := userIDOf(m)
	_ = addServedUser(uid)
	if !m.IsPrivate() {
		addServedChat(canonChatID(m.ChatID()))
	}

	// PANDA-MUSICV4 deletes the /stats command before showing the menu.
	_, _ = m.Delete()

	username := BotName
	if me, err := Bot.GetMe(); err == nil && me != nil && me.Username != "" {
		username = me.Username
	}
	if username == "" {
		username = "GOMUSICV2"
	}
	caption := "❖ <b>CLICK ON THE BUTTONS</b>\n" +
		"<b>BELOW TO CHECK THE STATS OF</b>\n" +
		"<b>@" + richEsc(username) + "</b>"

	if _, err := sendHTML(Bot, m.ChatID(), caption, statsKeyboard()); err != nil {
		return fmt.Errorf("send stats menu: %w", err)
	}
	return nil
}

func handleStatsCallback(cb *telegram.CallbackQuery, data string) bool {
	if data != "stats_general" && data != "stats_overall" {
		return false
	}

	username := BotName
	if me, err := Bot.GetMe(); err == nil && me != nil && me.Username != "" {
		username = me.Username
	}
	if username == "" {
		username = "GOMUSICV2"
	}

	var content string
	if data == "stats_overall" {
		content = statsOverallText(username)
	} else {
		content = statsGeneralText(username)
	}
	if err := editMenu(cb, content, statsKeyboard()); err != nil {
		// If the menu was sent as a normal text message, editMenu still uses
		// Telegram's EditMessage API and preserves the inline keyboard.
		if !isNotModified(err) {
			_, _ = cb.Answer("Unable to update stats")
			return true
		}
	}
	_, _ = cb.Answer("")
	return true
}
