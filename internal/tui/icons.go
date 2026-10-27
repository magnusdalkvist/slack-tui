package tui

import (
	"path"
	"strings"
)

// Icons are the glyphs views draw (DESIGN.md §3): Nerd Font v3 by default,
// plain Unicode with `[ui] icons = "unicode"`. Views ask for ico.Channel,
// never a codepoint. Each is one cell; icon slots add a space to make two.
type Icons struct {
	Channel, Private         string
	UserActive, UserAway     string
	Group, Unreads           string
	SecChannels, SecDMs      string
	SecExternal              string
	Folded, Unfolded         string
	Thread, Pinned, Bookmark string
	Scheduled, Bot, Search   string
	DND, Offline, Link       string
	Image, PDF, Doc, Sheet   string
	Slides, Archive, Code    string
	Audio, Video, File       string
	Unicode                  bool // the fallback set: file kinds become tags
}

var nerdIcons = Icons{
	// The Octicons hashtag (thinner than Font Awesome's, which fills its
	// cell and runs into the name) and presence circles for people.
	Channel: "\uf4df", Private: "",
	UserActive: "\U000f0765", UserAway: "\U000f0766", // nf-md-circle, nf-md-circle_outline
	Group: "\U000f0695", Unreads: "", // nf-md-circle_multiple_outline: a group, not a person
	// nf-oct-hash, nf-cod-comment_discussion
	SecChannels: "\uf4df", SecDMs: "\ueac7", SecExternal: "",
	Folded: "", Unfolded: "",
	Thread: "", Pinned: "", Bookmark: "",
	Scheduled: "", Bot: "\U000f06a9", Search: "",
	DND: "", Offline: "\U000f05aa", Link: "",
	Image: "", PDF: "", Doc: "", Sheet: "",
	Slides: "", Archive: "", Code: "",
	Audio: "", Video: "", File: "",
}

var unicodeIcons = Icons{
	Channel: "#", Private: "◇",
	UserActive: "●", UserAway: "○",
	Group: "◎", Unreads: "≡",
	Folded: "▸", Unfolded: "▾",
	Thread: "↳", Pinned: "⚑", Bookmark: "⚑",
	Scheduled: "◷", DND: "☾", Offline: "✕", Link: "↗",
	Image: "▣", PDF: "▣", Doc: "▣", Sheet: "▣",
	Slides: "▣", Archive: "▣", Code: "▣",
	Audio: "▣", Video: "▣", File: "▣",
	Unicode: true,
}

// ico is the active icon set; useIcons replaces it.
var ico = nerdIcons

// useIcons picks the set by the config's `[ui] icons`: "unicode" or the
// Nerd Font default.
func useIcons(name string) {
	if strings.EqualFold(name, "unicode") {
		ico = unicodeIcons
	} else {
		ico = nerdIcons
	}
}

// slot is an icon in its 2-cell slot; an icon the set leaves out is blank.
func slot(icon string) string {
	if icon == "" {
		return "  "
	}
	return icon + " "
}

// fileIcon is the glyph for a file's type, from its MIME type and name.
func fileIcon(mimetype, name string) string {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	switch {
	case strings.HasPrefix(mimetype, "image/"):
		return ico.Image
	case mimetype == "application/pdf" || ext == "pdf":
		return ico.PDF
	case strings.HasPrefix(mimetype, "audio/"):
		return ico.Audio
	case strings.HasPrefix(mimetype, "video/"):
		return ico.Video
	}
	switch ext {
	case "doc", "docx", "odt", "rtf", "txt", "md":
		return ico.Doc
	case "xls", "xlsx", "ods", "csv", "tsv":
		return ico.Sheet
	case "ppt", "pptx", "odp", "key":
		return ico.Slides
	case "zip", "tar", "gz", "tgz", "bz2", "xz", "7z", "rar":
		return ico.Archive
	case "go", "js", "ts", "py", "rb", "rs", "c", "h", "java", "json", "yaml", "yml", "toml", "sh", "html", "css", "sql":
		return ico.Code
	}
	return ico.File
}
