package logger_test

import (
	"testing"

	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

func TestSetup(t *testing.T) {
	t.Run("json format with info level", func(_ *testing.T) {
		logger.Setup("info", "json")
	})

	t.Run("text format with debug level", func(_ *testing.T) {
		logger.Setup("debug", "text")
	})

	t.Run("warn level", func(_ *testing.T) {
		logger.Setup("warn", "json")
	})

	t.Run("warning level alias", func(_ *testing.T) {
		logger.Setup("warning", "json")
	})

	t.Run("error level", func(_ *testing.T) {
		logger.Setup("error", "json")
	})

	t.Run("unknown level defaults to info", func(_ *testing.T) {
		logger.Setup("unknown", "json")
	})

	t.Run("unknown format defaults to json", func(_ *testing.T) {
		logger.Setup("info", "unknown")
	})
}
