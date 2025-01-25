package version

import "fmt"

const (
	Version   = "0.1.0"
	GitCommit = "dev" // This would be set during build
)

func GetVersion() string {
	return fmt.Sprintf("v%s (%s)", Version, GitCommit)
}
