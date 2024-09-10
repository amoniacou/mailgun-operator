package versions

const (
	// Version is the version of the operator
	Version = "1.0.0"
)

// BuildInfo is a struct containing all the info about the build
type BuildInfo struct {
	Version, Commit, Date string
}

var (
	// buildVersion injected during the build
	buildVersion = "1.0.0"

	// buildCommit injected during the build
	buildCommit = "none"

	// buildDate injected during the build
	buildDate = "unknown"

	// Info contains the build info
	Info = BuildInfo{
		Version: buildVersion,
		Commit:  buildCommit,
		Date:    buildDate,
	}
)
