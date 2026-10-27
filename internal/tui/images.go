package tui

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	// Decoders for the formats Slack thumbnails come in.
	_ "image/gif"
	_ "image/jpeg"

	_ "golang.org/x/image/webp"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/image/draw"
	"golang.org/x/sys/unix"

	"github.com/magnusdalkvist/slack-tui/internal/backend"
	"github.com/magnusdalkvist/slack-tui/internal/cache"
	"github.com/magnusdalkvist/slack-tui/internal/kitty"
)

const (
	maxThumbBytes = 25 << 20
	maxOpenBytes  = 500 << 20
	maxPixels     = 50_000_000 // refuse decompression bombs before decoding
)

type imgStatus int

const (
	imgLoading imgStatus = iota
	imgReady
	imgFailed
)

type imgState struct {
	id         uint32
	cols, rows int
	status     imgStatus
	err        string
	png        []byte // kept to resend after the pane was hidden

	// An animated GIF: every frame is in the terminal under its own id
	// (ids[0] is id), and the view prints the one showing.
	anim      *animFrames
	ids       []uint32
	animReady bool // all frames uploaded
	frame     int
	next      time.Time // when the next frame is due
	seen      int       // m.viewSeq of the last view that drew it
}

// shownID is the image id the view prints: the current frame's, for an
// animation.
func (st *imgState) shownID() uint32 {
	if st.animReady && st.frame < len(st.ids) {
		return st.ids[st.frame]
	}
	return st.id
}

type imageMsg struct {
	fileID     string
	cols, rows int
	png        []byte
	anim       *animFrames
	err        error
}

type openedMsg struct {
	name string
	err  error
}

// newImageID hands out image ids. Each run starts at a random point in
// the 24-bit id space: the terminal can still hold images from an earlier
// run (or from other programs in other tmux panes), and reusing their ids
// would draw the wrong picture in the wrong box.
func (m *Model) newImageID() uint32 {
	if m.imgIDBase == 0 {
		m.imgIDBase = 0x100000 + rand.Uint32N(0xE00000)
	}
	m.nextImgID++
	return (m.imgIDBase+m.nextImgID-1)%0xFFFFFF + 1
}

// SetGraphics turns inline images on; w is nil when the terminal can't draw them.
func (m *Model) SetGraphics(w *kitty.Writer, reason string) {
	m.gfx, m.gfxReason = w, reason
}

// cellSize is the terminal's cell in pixels, falling back to the
// RobotoMono 10 cell measured on this machine.
func cellSize() (int, int) {
	f, err := os.Open("/dev/tty")
	if err == nil {
		defer f.Close()
		if ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ); err == nil && ws.Col > 0 && ws.Row > 0 && ws.Xpixel > 0 && ws.Ypixel > 0 {
			return int(ws.Xpixel) / int(ws.Col), int(ws.Ypixel) / int(ws.Row)
		}
	}
	return 8, 18
}

// scanImages starts downloads for images in the lists on screen.
func (m *Model) scanImages() tea.Cmd {
	if m.gfx == nil {
		return nil
	}
	var cmds []tea.Cmd
	scan := func(l *msgList) {
		if l == nil {
			return
		}
		for _, msg := range l.msgs {
			for _, f := range msg.Files {
				if !f.IsImage() || m.images[f.ID] != nil {
					continue
				}
				m.images[f.ID] = &imgState{id: m.newImageID()}
				cmds = append(cmds, m.fetchImage(f, m.images[f.ID].id))
			}
		}
	}
	if m.current != unreadsID {
		scan(m.lists[m.current])
	}
	if m.thread != nil {
		scan(&m.thread.list)
	}
	return tea.Batch(cmds...)
}

// fetchImage loads an inline image into exactly the box imageBox kept
// for it, so the layout doesn't move when it arrives.
func (m *Model) fetchImage(f backend.File, id uint32) tea.Cmd {
	if f.W <= 0 || f.H <= 0 {
		return m.fetchImageSized(f.ID, f, id, m.cfg.ImageMaxCols, m.cfg.ImageMaxRows, false)
	}
	cols, rows := m.imageBox(f)
	return m.fetchImageSized(f.ID, f, id, cols, rows, true)
}

// fetchImageSized fetches an image into m.images[key], fitted to at most
// maxCols×maxRows cells; exact fills that box whatever the thumbnail's
// rounding, because the layout already holds it.
func (m *Model) fetchImageSized(key string, f backend.File, id uint32, maxCols, maxRows int, exact bool) tea.Cmd {
	cw, ch := m.cellW, m.cellH
	return func() tea.Msg {
		m.imgSem <- struct{}{} // a few downloads at a time
		defer func() { <-m.imgSem }()

		if f.Anim != "" {
			if data, err := m.client.Download(m.ctx, f.Anim, maxThumbBytes); err == nil {
				if a, cols, rows, err := scaleAnim(data, cw, ch, maxCols, maxRows); err == nil {
					if exact {
						cols, rows = maxCols, maxRows
					}
					if err := m.gfx.Show(id, a.frames[0], cols, rows); err != nil {
						return imageMsg{fileID: key, err: err}
					}
					return imageMsg{fileID: key, cols: cols, rows: rows, png: a.frames[0], anim: a}
				}
			}
			// Not animated after all, or too big: the still thumbnail.
		}
		src := f.Thumb
		if src == "" {
			src = f.URL
		}
		data, err := m.client.Download(m.ctx, src, maxThumbBytes)
		if err != nil {
			return imageMsg{fileID: key, err: err}
		}
		pngData, cols, rows, err := scaleImage(data, cw, ch, maxCols, maxRows, exact)
		if err != nil {
			return imageMsg{fileID: key, err: err}
		}
		if exact {
			cols, rows = maxCols, maxRows
		}
		if err := m.gfx.Show(id, pngData, cols, rows); err != nil {
			return imageMsg{fileID: key, err: err}
		}
		return imageMsg{fileID: key, cols: cols, rows: rows, png: pngData}
	}
}

// fitImage decodes an image, sizes it to at most maxCols×maxRows cells
// keeping its shape, scales it to exactly that many pixels and returns PNG.
func fitImage(data []byte, cw, ch, maxCols, maxRows int) ([]byte, int, int, error) {
	return scaleImage(data, cw, ch, maxCols, maxRows, false)
}

// scaleImage is fitImage, with grow to also enlarge small images to fill
// the box.
func scaleImage(data []byte, cw, ch, maxCols, maxRows int, grow bool) ([]byte, int, int, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, fmt.Errorf("not an image I can read: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixels {
		return nil, 0, 0, fmt.Errorf("image is %dx%d, too large", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, err
	}
	w, h := cfg.Width, cfg.Height
	if grow {
		// Pretend it is at least as big as the box; fitCells then fills it.
		f := max(float64(maxCols*cw)/float64(w), float64(maxRows*ch)/float64(h), 1)
		w, h = int(float64(w)*f), int(float64(h)*f)
	}
	cols, rows := fitCells(w, h, cw, ch, maxCols, maxRows)
	// Scale into the box, keeping the aspect ratio.
	bw, bh := cols*cw, rows*ch
	w, h = bw, bw*cfg.Height/cfg.Width
	if h > bh {
		w, h = bh*cfg.Width/cfg.Height, bh
	}
	dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, dst); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), cols, rows, nil
}

// fitCells picks the box in cells: no wider than the image itself, within
// the limits, and the right shape.
func fitCells(w, h, cw, ch, maxCols, maxRows int) (int, int) {
	cols := min(maxCols, max(1, (w+cw-1)/cw))
	rows := max(1, (cols*cw*h/w+ch/2)/ch)
	if rows > maxRows {
		rows = maxRows
		cols = max(1, min(cols, (rows*ch*w/h+cw/2)/cw))
	}
	return cols, rows
}

// resendImages sends ready images again. tmux drops passthrough for a
// hidden pane, so an image that arrived while we were hidden never reached
// the terminal; this runs when the pane gets focus.
func (m *Model) resendImages() tea.Cmd {
	if m.gfx == nil {
		return nil
	}
	type job struct {
		id         uint32
		png        []byte
		cols, rows int
	}
	var jobs []job
	for _, set := range []map[string]*imgState{m.images, m.emojiImgs} {
		for _, st := range set {
			if st.status != imgReady {
				continue
			}
			if st.animReady {
				for i, id := range st.ids {
					jobs = append(jobs, job{id, st.anim.frames[i], st.cols, st.rows})
				}
				continue
			}
			jobs = append(jobs, job{st.id, st.png, st.cols, st.rows})
		}
	}
	if len(jobs) == 0 {
		return nil
	}
	return func() tea.Msg {
		for _, j := range jobs {
			m.gfx.Show(j.id, j.png, j.cols, j.rows)
		}
		return nil
	}
}

// imageLines renders an attachment (DESIGN.md §10): its file line, and
// for an image the picture in a frame below it, accent while the message
// is selected. While it loads, a dashed frame holds its final size, so
// nothing moves when it arrives; a failure is one red line.
func (m *Model) imageLines(f backend.File, width int, sel bool) []string {
	st := m.images[f.ID]
	if m.gfx == nil || !f.IsImage() || st == nil {
		return wrap(fileLine(f, f.Size), width)
	}
	name := fileLabel(f)
	switch st.status {
	case imgLoading:
		cols, rows := m.imageBox(f)
		head := sty.Faint.Render(ico.Image) + " " + sty.Normal.Render(name) + "  " + sty.Faint.Render("loading…")
		return append([]string{ansi.Truncate(head, width, "…")}, dashedBox(cols, rows)...)
	case imgFailed:
		line := sty.Err.Render(ico.Image) + " " + sty.Normal.Render(name) + "  " + sty.Err.Render("couldn't load · "+st.err)
		return []string{ansi.Truncate(line, width, "…")}
	}
	st.seen = m.viewSeq
	head := fileLine(f, f.Size)
	if st.anim != nil {
		state := "❚❚"
		if st.animReady {
			state = "▶ playing"
		}
		head += sty.Faint.Render(" · " + state)
	}
	border := sty.Rule
	if sel {
		border = sty.Bar
	}
	lines := []string{ansi.Truncate(head, width, "…"), border.Render("╭" + strings.Repeat("─", st.cols) + "╮")}
	side := border.Render("│")
	for _, row := range kitty.Placeholder(st.shownID(), st.cols, st.rows) {
		lines = append(lines, side+row+side)
	}
	return append(lines, border.Render("╰"+strings.Repeat("─", st.cols)+"╯"))
}

// focusedMsg is the selected message of the list that has focus.
func (m *Model) focusedMsg() (backend.Message, bool) {
	l := m.lists[m.current]
	switch {
	case m.focus == fThread && m.thread != nil:
		l = &m.thread.list
	case m.focus != fMessages:
		return backend.Message{}, false
	}
	if l == nil || len(l.msgs) == 0 {
		return backend.Message{}, false
	}
	sel := l.sel
	if sel < 0 || sel >= len(l.msgs) {
		sel = len(l.msgs) - 1
	}
	return l.msgs[sel], true
}

// failedImages are the selected message's images that couldn't load.
func (m *Model) failedImages() []string {
	msg, ok := m.focusedMsg()
	if !ok {
		return nil
	}
	var ids []string
	for _, f := range msg.Files {
		if st := m.images[f.ID]; st != nil && st.status == imgFailed {
			ids = append(ids, f.ID)
		}
	}
	return ids
}

// retryImages is r on a message whose images failed: load them again.
func (m *Model) retryImages(ids []string) tea.Cmd {
	for _, id := range ids {
		delete(m.images, id) // scanImages fetches them anew
	}
	return m.setStatus("loading the image again…", false)
}

// imageBox is the cells an inline image takes, from the file's own size,
// so its place is kept before it loads. Without a size it guesses 4:3.
func (m *Model) imageBox(f backend.File) (int, int) {
	w, h := f.W, f.H
	if w <= 0 || h <= 0 {
		w, h = 4*m.cellW*m.cfg.ImageMaxCols, 3*m.cellW*m.cfg.ImageMaxCols
	}
	return fitCells(w, h, max(m.cellW, 1), max(m.cellH, 1), m.cfg.ImageMaxCols, m.cfg.ImageMaxRows)
}

// dashedBox is the frame an image loads into.
func dashedBox(cols, rows int) []string {
	out := []string{sty.Faint.Render("╭" + strings.Repeat("┄", cols) + "╮")}
	for range rows {
		out = append(out, sty.Faint.Render("┆"+strings.Repeat(" ", cols)+"┆"))
	}
	return append(out, sty.Faint.Render("╰"+strings.Repeat("┄", cols)+"╯"))
}

// --- opening files --------------------------------------------------------

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// openFile downloads a file to the cache and opens it in a viewer.
func (m *Model) openFile(f backend.File) tea.Cmd {
	viewer := m.cfg.ImageViewer
	if !f.IsImage() || viewer == "" {
		viewer = "xdg-open"
	}
	name := f.Name
	if name == "" {
		name = f.ID
	}
	return func() tea.Msg {
		path, err := m.cachedFile(f)
		if err != nil {
			return openedMsg{name, err}
		}
		argv := strings.Fields(viewer)
		cmd := exec.Command(argv[0], append(argv[1:], path)...)
		// Its own session and no terminal, so it can't draw over the TUI
		// and outlives it.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			return openedMsg{name, fmt.Errorf("%s: %w", argv[0], err)}
		}
		go cmd.Wait()
		return openedMsg{name: name}
	}
}

// cachedFile downloads a file's full original into the cache once and
// returns where it is.
func (m *Model) cachedFile(f backend.File) (string, error) {
	name := f.Name
	if name == "" {
		name = f.ID
	}
	dir := filepath.Join(cacheDir(), "files")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, f.ID+"-"+strings.Trim(unsafeName.ReplaceAllString(name, "_"), "._"))
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	data, err := m.client.Download(m.ctx, f.URL, maxOpenBytes)
	if err != nil {
		return "", err
	}
	// Write then rename, so a half-written file is never taken as cached.
	tmp := path + ".part"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}

// openURL hands a web link to the desktop's browser.
func (m *Model) openURL(url string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("xdg-open", url)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			return openedMsg{url, fmt.Errorf("xdg-open: %w", err)}
		}
		go cmd.Wait()
		return openedMsg{name: url}
	}
}

func cacheDir() string { return cache.Dir() }
