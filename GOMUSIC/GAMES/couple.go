package games

import (
	"html"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
)

func init() {
	_ = gif.Decode
	_ = png.Decode
}

type Pair struct {
	Caption string
	Image   string
	ErrText string
}

var (
	coolMu sync.Mutex
	cool   = map[int64]time.Time{}
)

func Pick(bot *telegram.Client, chatID int64) Pair {
	if bot == nil || chatID == 0 {
		return Pair{ErrText: "bot is not ready"}
	}
	if wait := cooldownLeft(chatID); wait > 0 {
		return Pair{ErrText: "wait " + itoa(int(wait.Seconds())+1) + "s before the next pair"}
	}
	members, _, err := bot.GetChatMembers(chatID, &telegram.ParticipantOptions{Limit: 200})
	if err != nil {
		return Pair{ErrText: "bot needs admin rights to read members"}
	}
	var users []*telegram.UserObj
	seen := map[int64]bool{}
	for _, p := range members {
		if p == nil || p.User == nil || p.User.Bot || p.User.Deleted || p.User.ID == 0 || seen[p.User.ID] {
			continue
		}
		st := strings.ToLower(p.Status)
		if strings.Contains(st, "left") || strings.Contains(st, "ban") {
			continue
		}
		seen[p.User.ID] = true
		users = append(users, p.User)
	}
	if len(users) < 2 {
		return Pair{ErrText: "need at least 2 members for /couple"}
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(users), func(i, j int) { users[i], users[j] = users[j], users[i] })
	a, b := users[0], users[1]
	score := 60 + rng.Intn(41)
	img, err := buildCard(bot, a, b, score)
	if err != nil {
		return Pair{ErrText: "could not build the pair card"}
	}
	markCooldown(chatID)
	return Pair{
		Image:   img,
		Caption: caption(a, b, score),
	}
}

func cooldownLeft(chatID int64) time.Duration {
	coolMu.Lock()
	defer coolMu.Unlock()
	until, ok := cool[chatID]
	if !ok {
		return 0
	}
	return time.Until(until)
}

func markCooldown(chatID int64) {
	coolMu.Lock()
	cool[chatID] = time.Now().Add(20 * time.Second)
	coolMu.Unlock()
}

func caption(a, b *telegram.UserObj, score int) string {
	return "<b>" + sc("today's pair") + "</b>\n\n" +
		mention(a) + "\n+\n" + mention(b) + "\n\n" +
		sc("match") + " : <b>" + itoa(score) + "%</b>"
}

func mention(u *telegram.UserObj) string {
	name := "User"
	if u != nil && strings.TrimSpace(u.FirstName) != "" {
		name = strings.TrimSpace(u.FirstName)
	}
	if u == nil {
		return htmlEsc(name)
	}
	return `<a href="tg://user?id=` + itoa64(u.ID) + `">` + htmlEsc(name) + `</a>`
}

func buildCard(bot *telegram.Client, a, b *telegram.UserObj, score int) (string, error) {
	const W, H = 1080, 720
	canvas := image.NewRGBA(image.Rect(0, 0, W, H))
	paintBG(canvas)
	left := loadAvatar(bot, a.ID, 280)
	right := loadAvatar(bot, b.ID, 280)
	drawRing(canvas, 300, 340, 156, color.RGBA{255, 186, 214, 255})
	drawRing(canvas, 780, 340, 156, color.RGBA{186, 214, 255, 255})
	pasteCircle(canvas, left, 160, 200)
	pasteCircle(canvas, right, 640, 200)
	drawPlus(canvas, 540, 340, color.RGBA{255, 230, 240, 255})
	drawText(canvas, (W-textWidth("PAIR OF THE DAY", 3))/2, 48, "PAIR OF THE DAY", 3, color.RGBA{255, 236, 244, 255})
	drawText(canvas, (W-textWidth(itoa(score)+"%", 4))/2, 620, itoa(score)+"%", 4, color.RGBA{255, 220, 230, 255})
	_ = os.MkdirAll("downloads", 0o755)
	out := filepath.Join("downloads", "couple_"+itoa64(time.Now().UnixNano())+".jpg")
	f, err := os.Create(out)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := jpeg.Encode(f, canvas, &jpeg.Options{Quality: 90}); err != nil {
		return "", err
	}
	return out, nil
}

func paintBG(dst *image.RGBA) {
	b := dst.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			t := y * 255 / b.Dy()
			dst.SetRGBA(x, y, color.RGBA{
				R: uint8(36 + t/5),
				G: uint8(16 + t/10),
				B: uint8(48 + t/6),
				A: 255,
			})
		}
	}
}

func loadAvatar(bot *telegram.Client, uid int64, size int) image.Image {
	fallback := defaultAvatar(size)
	if bot == nil || uid == 0 {
		return fallback
	}
	photos, err := bot.GetProfilePhotos(uid, &telegram.PhotosOptions{Limit: 1})
	if err != nil || len(photos) == 0 {
		return fallback
	}
	_ = os.MkdirAll("downloads", 0o755)
	dest := filepath.Join("downloads", "dp_"+itoa64(uid)+".jpg")
	path, err := bot.DownloadMedia(photos[0].Photo, &telegram.DownloadOptions{FileName: dest})
	if err != nil || path == "" {
		path = dest
	}
	f, err := os.Open(path)
	if err != nil {
		return fallback
	}
	img, _, decErr := image.Decode(f)
	f.Close()
	_ = os.Remove(path)
	if decErr != nil {
		return fallback
	}
	return cropSquare(img, size)
}

func defaultAvatar(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{90, 70, 120, 255}}, image.Point{}, draw.Src)
	cx, cy := size/2, size/2
	fillCircle(img, cx, cy-size/8, size/7, color.RGBA{210, 190, 220, 255})
	fillCircle(img, cx, cy+size/5, size/4, color.RGBA{210, 190, 220, 255})
	return img
}

func cropSquare(src image.Image, size int) image.Image {
	b := src.Bounds()
	side := b.Dx()
	if b.Dy() < side {
		side = b.Dy()
	}
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		sy := y0 + y*side/size
		for x := 0; x < size; x++ {
			sx := x0 + x*side/size
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}

func pasteCircle(dst *image.RGBA, src image.Image, x, y int) {
	b := src.Bounds()
	r := b.Dx() / 2
	rr := r * r
	for yy := 0; yy < b.Dy(); yy++ {
		for xx := 0; xx < b.Dx(); xx++ {
			dx, dy := xx-r, yy-r
			if dx*dx+dy*dy > rr {
				continue
			}
			dst.Set(x+xx, y+yy, src.At(b.Min.X+xx, b.Min.Y+yy))
		}
	}
}

func drawRing(dst *image.RGBA, cx, cy, r int, c color.RGBA) {
	outer := r * r
	inner := (r - 8) * (r - 8)
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			d := x*x + y*y
			if d <= outer && d >= inner {
				dst.SetRGBA(cx+x, cy+y, c)
			}
		}
	}
}

func fillCircle(dst *image.RGBA, cx, cy, r int, c color.RGBA) {
	rr := r * r
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if x*x+y*y <= rr {
				dst.SetRGBA(cx+x, cy+y, c)
			}
		}
	}
}

func drawPlus(dst *image.RGBA, cx, cy int, c color.RGBA) {
	for i := -28; i <= 28; i++ {
		for t := -6; t <= 6; t++ {
			dst.SetRGBA(cx+i, cy+t, c)
			dst.SetRGBA(cx+t, cy+i, c)
		}
	}
}

func htmlEsc(s string) string {
	return html.EscapeString(s)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [24]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

var scMap = map[rune]rune{
	'a': 'ᴀ', 'b': 'ʙ', 'c': 'ᴄ', 'd': 'ᴅ', 'e': 'ᴇ',
	'f': 'ꜰ', 'g': 'ɢ', 'h': 'ʜ', 'i': 'ɪ', 'j': 'ᴊ',
	'k': 'ᴋ', 'l': 'ʟ', 'm': 'ᴍ', 'n': 'ɴ', 'o': 'ᴏ',
	'p': 'ᴘ', 'q': 'ǫ', 'r': 'ʀ', 's': 's', 't': 'ᴛ',
	'u': 'ᴜ', 'v': 'ᴠ', 'w': 'ᴡ', 'x': 'x', 'y': 'ʏ', 'z': 'z',
}

func sc(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r = r - 'A' + 'a'
		}
		if m, ok := scMap[r]; ok {
			b.WriteRune(m)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// 5x7 glyphs for the card title and score.
var glyph = map[byte]string{
	'A': "01110,10001,10001,11111,10001,10001,10001",
	'D': "11110,10001,10001,10001,10001,10001,11110",
	'E': "11111,10000,10000,11110,10000,10000,11111",
	'F': "11111,10000,10000,11110,10000,10000,10000",
	'H': "10001,10001,10001,11111,10001,10001,10001",
	'I': "11111,00100,00100,00100,00100,00100,11111",
	'O': "01110,10001,10001,10001,10001,10001,01110",
	'P': "11110,10001,10001,11110,10000,10000,10000",
	'R': "11110,10001,10001,11110,10100,10010,10001",
	'T': "11111,00100,00100,00100,00100,00100,00100",
	'Y': "10001,10001,01010,00100,00100,00100,00100",
	' ': "00000,00000,00000,00000,00000,00000,00000",
	'%': "11001,11010,00100,01000,10110,10011,00000",
	'0': "01110,10001,10011,10101,11001,10001,01110",
	'1': "00100,01100,00100,00100,00100,00100,01110",
	'2': "01110,10001,00001,00010,00100,01000,11111",
	'3': "11110,00001,00001,01110,00001,00001,11110",
	'4': "00010,00110,01010,10010,11111,00010,00010",
	'5': "11111,10000,10000,11110,00001,00001,11110",
	'6': "01110,10000,10000,11110,10001,10001,01110",
	'7': "11111,00001,00010,00100,01000,01000,01000",
	'8': "01110,10001,10001,01110,10001,10001,01110",
	'9': "01110,10001,10001,01111,00001,00001,01110",
}

func textWidth(s string, scale int) int {
	return len(s)*(5*scale+scale) - scale
}

func drawText(dst *image.RGBA, x, y int, s string, scale int, c color.RGBA) {
	for i := 0; i < len(s); i++ {
		rows := strings.Split(glyph[s[i]], ",")
		if len(rows) != 7 {
			rows = strings.Split(glyph[' '], ",")
		}
		for ry, row := range rows {
			for rx := 0; rx < len(row) && rx < 5; rx++ {
				if row[rx] != '1' {
					continue
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						dst.SetRGBA(x+rx*scale+dx, y+ry*scale+dy, c)
					}
				}
			}
		}
		x += 6 * scale
	}
}
