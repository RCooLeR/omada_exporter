package cmd

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

const maxSecondFlagValue int64 = (1<<63 - 1) / int64(time.Second)

func validateControllerURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("host must be an absolute http or https URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("host URL must not contain a query or fragment")
	}
	if parsed.User != nil {
		return fmt.Errorf("host URL must not contain user credentials")
	}
	if port := parsed.Port(); port != "" {
		if err := validatePort(port); err != nil {
			return fmt.Errorf("invalid host URL port: %w", err)
		}
	}
	return nil
}

func validatePort(value string) error {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("port must be an integer between 1 and 65535")
	}
	return nil
}

func validateLogLevel(value string) error {
	if _, err := zerolog.ParseLevel(value); err != nil {
		return fmt.Errorf("invalid log-level %q: %w", value, err)
	}
	return nil
}

func oneOf(name string, allowed ...string) func(string) error {
	return func(value string) error {
		for _, candidate := range allowed {
			if strings.EqualFold(value, candidate) {
				return nil
			}
		}
		return fmt.Errorf("%s must be one of %s", name, strings.Join(allowed, ", "))
	}
}

func nonNegativeInt(name string) func(int) error {
	return func(value int) error {
		if value < 0 {
			return fmt.Errorf("%s must not be negative", name)
		}
		return nil
	}
}

func positiveSeconds(name string) func(int) error {
	return boundedSeconds(name, false)
}

func nonNegativeSeconds(name string) func(int) error {
	return boundedSeconds(name, true)
}

func boundedSeconds(name string, allowZero bool) func(int) error {
	return func(value int) error {
		if value < 0 || (!allowZero && value == 0) {
			if allowZero {
				return fmt.Errorf("%s must not be negative", name)
			}
			return fmt.Errorf("%s must be greater than zero", name)
		}
		if int64(value) > maxSecondFlagValue {
			return fmt.Errorf("%s must not exceed %d seconds", name, maxSecondFlagValue)
		}
		return nil
	}
}
