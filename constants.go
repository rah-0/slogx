package slogx

const (
	defaultTimeLayout = "2006-01-02 15:04:05.000000"
	ansiReset         = "\x1b[0m"
	ansiGray          = "\x1b[90m"
	ansiGreen         = "\x1b[32m"
	ansiYellow        = "\x1b[33m"
	ansiRed           = "\x1b[31m"
	systemdDebug      = "<7>"
	systemdInfo       = "<6>"
	systemdWarning    = "<4>"
	systemdError      = "<3>"
)

// Format controls the output format used by Slogx constructors.
type Format uint8

const (
	// Text selects slog.TextHandler.
	Text Format = iota
	// JSON selects slog.JSONHandler.
	JSON
	// TextColored selects slog.TextHandler with colored standard level names.
	TextColored
	// Systemd selects slog.TextHandler with systemd journal priority prefixes.
	Systemd
)
