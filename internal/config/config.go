// Package config loads ~/.config/slack-tui/config.toml.
package config

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Section is a custom sidebar section. Slack's own sections are not in the
// public API, so they are defined here instead.
type Section struct {
	Name string `toml:"name"`
	// Items are "#channel" for channels and "@name" for DMs, where name is a
	// username, display name or real name.
	Items []string `toml:"items"`
	// Icon is an emoji shortcode shown before the name, custom ones too
	// (":partyparrot:").
	Icon string `toml:"icon"`
}

// StatusTemplate is a ready-made status for `:status <name>`.
type StatusTemplate struct {
	Name  string `toml:"name"`
	Emoji string `toml:"emoji"`
	Text  string `toml:"text"`
	// Expires is "30m", "2h", "1d", "until 17:00" or "until tomorrow";
	// empty keeps the status until changed.
	Expires string `toml:"expires"`
}

// Theme is the [theme] table: a palette by name and single tokens changed.
type Theme struct {
	// Name is auto (by the terminal background), dark, light or ansi (the
	// terminal's own palette).
	Name string `toml:"name"`
	// Overrides sets tokens (accent, mention, …) to "#rrggbb" or a 256-colour index.
	Overrides map[string]string `toml:"overrides"`
}

// UI is the [ui] table.
type UI struct {
	// Icons is "nerd" (Nerd Font glyphs, the default) or "unicode" for
	// terminals without a Nerd Font.
	Icons string `toml:"icons"`
}

// Layout is the [layout] table: the window widths the layout changes at.
type Layout struct {
	// NarrowCols: below it, one frame fills the window at a time (stack
	// mode: conversations, then the chat, then the thread).
	NarrowCols int `toml:"narrow_cols"`
	// ThreadSplitCols: from it on, an open thread (or a pane's preview)
	// sits beside the chat; below it, the thread takes the chat's place.
	ThreadSplitCols int `toml:"thread_split_cols"`
}

type Config struct {
	Theme    Theme            `toml:"theme"`
	UI       UI               `toml:"ui"`
	Layout   Layout           `toml:"layout"`
	Sections []Section        `toml:"section"`
	Statuses []StatusTemplate `toml:"status"`
	// TimeFormat is a Go time layout for message timestamps.
	TimeFormat string `toml:"time_format"`
	// SidebarWidth is the width of the left column in cells.
	SidebarWidth int `toml:"sidebar_width"`
	// RefreshMinutes is how often unread markers are re-checked against
	// Slack, to pick up messages read on another device.
	RefreshMinutes int `toml:"refresh_minutes"`
	// PresenceMinutes is how often the presence of the people in the
	// sidebar's DMs is checked; negative turns it off.
	PresenceMinutes int `toml:"presence_minutes"`
	// Images is "auto" (inline images when the terminal can draw them) or "off".
	Images       string `toml:"images"`
	ImageMaxCols int    `toml:"image_max_cols"`
	ImageMaxRows int    `toml:"image_max_rows"`
	// ImageViewer opens images with `o`; other files use xdg-open.
	ImageViewer string `toml:"image_viewer"`
	// DownloadDir is where `s` saves files; empty means the desktop's
	// Downloads folder. A leading ~ is the home directory.
	DownloadDir string `toml:"download_dir"`
}

func Default() Config {
	return Config{Layout: Layout{NarrowCols: 100, ThreadSplitCols: 140}, TimeFormat: "15:04", SidebarWidth: 30, RefreshMinutes: 5, PresenceMinutes: 3, Images: "auto", ImageMaxCols: 40, ImageMaxRows: 12}
}

// Dir is the config directory.
func Dir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "slack-tui")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "slack-tui")
}

// Path is the config file location.
func Path() string { return filepath.Join(Dir(), "config.toml") }

// Load reads the config file. A missing file gives the defaults.
func Load() (Config, error) {
	c := Default()
	// loupe (GNOME's viewer) when installed: xdg-open sends images to the
	// browser on this machine.
	if _, err := exec.LookPath("loupe"); err == nil {
		c.ImageViewer = "loupe"
	}
	_, err := toml.DecodeFile(Path(), &c)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	d := Default()
	if c.TimeFormat == "" {
		c.TimeFormat = d.TimeFormat
	}
	if c.SidebarWidth < 16 {
		c.SidebarWidth = d.SidebarWidth
	}
	if c.RefreshMinutes <= 0 {
		c.RefreshMinutes = d.RefreshMinutes
	}
	if c.PresenceMinutes == 0 {
		c.PresenceMinutes = d.PresenceMinutes
	}
	if c.Images == "" {
		c.Images = d.Images
	}
	if c.ImageMaxCols <= 0 {
		c.ImageMaxCols = d.ImageMaxCols
	}
	if c.ImageMaxRows <= 0 {
		c.ImageMaxRows = d.ImageMaxRows
	}
	if c.Layout.NarrowCols <= 0 {
		c.Layout.NarrowCols = d.Layout.NarrowCols
	}
	if c.Layout.ThreadSplitCols <= 0 {
		c.Layout.ThreadSplitCols = d.Layout.ThreadSplitCols
	}
	return c, nil
}
