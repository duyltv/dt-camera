// Package version exposes the application's build identity.
//
// The four fields below are set at link time:
//
//	go build -ldflags "\
//	  -X github.com/dt-camera/backend/internal/version.AppVersion=v1.2.3 \
//	  -X github.com/dt-camera/backend/internal/version.GitCommit=$(git rev-parse --short HEAD) \
//	  -X github.com/dt-camera/backend/internal/version.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
//
// When built without ldflags the variables fall back to "dev", the empty
// string, and "unknown" respectively. Tests and `go run` always see those
// fallbacks.
package version

// Variables overridden via -ldflags at link time.
var (
	AppVersion = "dev"
	GitCommit  = ""
	BuildTime  = "unknown"
)

// Info is the immutable snapshot returned by /api/version.
type Info struct {
	AppVersion string `json:"app_version"`
	GitCommit  string `json:"git_commit"`
	BuildTime  string `json:"build_time"`
}

// Snapshot returns the current build identity as a value (not a pointer) so
// callers can log/embed it without worrying about mutation.
func Snapshot() Info {
	return Info{
		AppVersion: AppVersion,
		GitCommit:  GitCommit,
		BuildTime:  BuildTime,
	}
}