package config

import (
	"fmt"
	"regexp"
)

var bridgeIDPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

// ValidateBridgeID accepts an immutable deployment namespace or the empty
// legacy namespace. Never normalize it: that could alias two installations.
func ValidateBridgeID(value string) error {
	if value == "" || bridgeIDPattern.MatchString(value) {
		return nil
	}
	return fmt.Errorf("bridge-id must be empty (legacy) or 1-64 lowercase letters/digits/hyphens, starting and ending with a letter or digit")
}
