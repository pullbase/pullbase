package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionString(t *testing.T) {
	t.Run("returns correct format with default values", func(t *testing.T) {
		result := VersionString()

		assert.Contains(t, result, "pullbase-agent version")
		assert.Contains(t, result, Version)
		assert.Contains(t, result, GitCommit)
		assert.Contains(t, result, BuildTime)
	})

	t.Run("includes all version components", func(t *testing.T) {
		originalVersion := Version
		originalCommit := GitCommit
		originalBuildTime := BuildTime
		defer func() {
			Version = originalVersion
			GitCommit = originalCommit
			BuildTime = originalBuildTime
		}()

		Version = "v1.2.3"
		GitCommit = "abc123"
		BuildTime = "2025-01-01T00:00:00Z"

		result := VersionString()

		assert.Equal(t, "pullbase-agent version v1.2.3 (commit: abc123, built: 2025-01-01T00:00:00Z)", result)
	})

	t.Run("format matches expected pattern", func(t *testing.T) {
		result := VersionString()

		assert.True(t, strings.HasPrefix(result, "pullbase-agent version "))
		assert.Contains(t, result, "(commit: ")
		assert.Contains(t, result, ", built: ")
		assert.True(t, strings.HasSuffix(result, ")"))
	})
}
