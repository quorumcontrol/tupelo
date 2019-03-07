package resources

import (
	"strings"

	"github.com/gobuffalo/packr/v2"
)

var version string

// Version gets the current version.
func Version() (string, error) {
	if version != "" {
		return version, nil
	}

	box := packr.New("Resources", "./templates")
	version, err := box.FindString("version.txt")
	if err == nil {
		version = strings.TrimSpace(version)
	}
	return version, err
}
