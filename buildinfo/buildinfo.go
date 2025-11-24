package buildinfo

var (
	// GitCommit stores the full commit hash.
	GitCommit string = "dev"

	// GitBranch stores the current branch name.
	GitBranch string = "local"

	// GitTag stores the nearest tag (e.g., v1.0.0).
	GitTag string = "untagged"

	// BuildDate stores the build timestamp.
	BuildDate string = "N/A"
)
