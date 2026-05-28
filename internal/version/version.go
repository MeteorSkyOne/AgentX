package version

import "strings"

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func Short() string {
	v := strings.TrimSpace(Version)
	if v == "" {
		return "dev"
	}
	return v
}

func Info() string {
	return Short() + " (commit " + valueOrUnknown(Commit) + ", built " + valueOrUnknown(Date) + ")"
}

func IsDev() bool {
	v := Short()
	return v == "dev" || strings.Contains(v, "-dev") || strings.Contains(v, "dirty")
}

func valueOrUnknown(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}
