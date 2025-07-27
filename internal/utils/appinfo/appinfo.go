// Package appinfo provides application information utilities
package appinfo

import (
	"os"
	"runtime/debug"
	"strings"
)

// GetEnvironment returns the current environment
// It checks for the following in order:
// 1. ENVIRONMENT environment variable
// 2. GO_ENV environment variable
// 3. NODE_ENV environment variable (common in some setups)
// 4. Defaults to "development"
func GetEnvironment() string {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = os.Getenv("GO_ENV")
	}
	if env == "" {
		env = os.Getenv("NODE_ENV") // For compatibility with some deployment setups
	}
	if env == "" {
		env = "development"
	}
	// Normalize common environment names
	switch strings.ToLower(env) {
	case "prod", "production":
		return "production"
	case "stage", "staging":
		return "staging"
	case "test", "testing":
		return "test"
	case "dev", "development":
		return "development"
	default:
		return env
	}
}

// GetVersion returns the application version
// It checks for the following in order:
// 1. VERSION environment variable
// 2. APP_VERSION environment variable
// 3. Build info from debug.BuildInfo (if built with module support)
// 4. Defaults to "0.0.0-unknown"
func GetVersion() string {
	// Check environment variables first
	if version := os.Getenv("VERSION"); version != "" {
		return version
	}
	if version := os.Getenv("APP_VERSION"); version != "" {
		return version
	}

	// Try to get version from build info
	if info, ok := debug.ReadBuildInfo(); ok {
		// Main module version
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}

		// Check for version in build settings (set via -ldflags)
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" || setting.Key == "vcs.version" {
				if setting.Value != "" {
					return setting.Value
				}
			}
		}
	}

	// Fallback to unknown version
	return "0.0.0-unknown"
}
