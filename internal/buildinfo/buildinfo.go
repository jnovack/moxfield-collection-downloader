package buildinfo

import "runtime/debug"

const (
	// DefaultVersion is used when no build-time version is injected.
	DefaultVersion = "dev"
	// DefaultBuildRFC3339 is used when no build-time timestamp is injected.
	DefaultBuildRFC3339 = "1970-01-01T00:00:00Z"
	// DefaultRevision is used when no build-time revision is injected.
	DefaultRevision = "local"
)

// Populate fills unset build metadata from Go build info when available.
func Populate(version, buildRFC3339, revision string) (string, string, string) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return version, buildRFC3339, revision
	}

	if version == DefaultVersion && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if revision == DefaultRevision && setting.Value != "" {
				revision = setting.Value
			}
		case "vcs.time":
			if buildRFC3339 == DefaultBuildRFC3339 && setting.Value != "" {
				buildRFC3339 = setting.Value
			}
		}
	}

	return version, buildRFC3339, revision
}
