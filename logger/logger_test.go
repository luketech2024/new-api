package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogWriterRotatesByDate(t *testing.T) {
	tempDir := t.TempDir()
	useTestLogDirectory(t, tempDir)

	firstDay := time.Date(2026, time.August, 18, 23, 59, 59, 0, time.Local)
	_, err := writeLogAt(firstDay, []byte("first\n"))
	require.NoError(t, err)
	firstPath := filepath.Join(tempDir, "20260818", "oneapi.log")
	require.Equal(t, firstPath, GetCurrentLogPath())

	_, err = writeLogAt(firstDay.AddDate(0, 0, 1), []byte("second\n"))
	require.NoError(t, err)
	secondPath := filepath.Join(tempDir, "20260819", "oneapi.log")
	require.Equal(t, secondPath, GetCurrentLogPath())

	_, err = os.Stat(firstPath)
	require.NoError(t, err)
	_, err = os.Stat(secondPath)
	require.NoError(t, err)
	assert.NotEqual(t, firstPath, secondPath)

	data, err := os.ReadFile(secondPath)
	require.NoError(t, err)
	assert.Equal(t, "second\n", string(data))
	firstData, err := os.ReadFile(firstPath)
	require.NoError(t, err)
	assert.Equal(t, "first\n", string(firstData))
}

func TestLogWriterAddsSequenceAfterLogCountLimit(t *testing.T) {
	tempDir := t.TempDir()
	useTestLogDirectory(t, tempDir)

	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.Local)
	_, err := writeLogAt(now, []byte("first\n"))
	require.NoError(t, err)
	setupLogLock.Lock()
	currentLogCount = maxLogCount
	setupLogLock.Unlock()
	_, err = writeLogAt(now, []byte("next\n"))
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(tempDir, "20260818", "oneapi-1.log"), GetCurrentLogPath())
}

func TestLogWriterRotatesExistingFileAtLineLimitAfterRestart(t *testing.T) {
	tempDir := t.TempDir()
	useTestLogDirectory(t, tempDir)

	now := time.Date(2026, time.August, 18, 12, 0, 0, 0, time.Local)
	logDir := filepath.Join(tempDir, "20260818")
	require.NoError(t, os.MkdirAll(logDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(logDir, "oneapi.log"),
		[]byte(strings.Repeat("existing\n", maxLogCount)),
		0644,
	))

	_, err := writeLogAt(now, []byte("next\n"))
	require.NoError(t, err)

	rotatedPath := filepath.Join(logDir, "oneapi-1.log")
	assert.Equal(t, rotatedPath, GetCurrentLogPath())
	data, err := os.ReadFile(rotatedPath)
	require.NoError(t, err)
	assert.Equal(t, "next\n", string(data))
}

func useTestLogDirectory(t *testing.T, dir string) {
	t.Helper()
	setupLogLock.Lock()
	originalLogDir := *common.LogDir
	originalPath := currentLogPath
	originalFile := currentLogFile
	originalDate := currentLogDate
	originalSequence := currentLogSequence
	originalCount := currentLogCount
	currentLogPath = ""
	currentLogFile = nil
	currentLogDate = ""
	currentLogSequence = 0
	currentLogCount = 0
	*common.LogDir = dir
	setupLogLock.Unlock()

	common.LogWriterMu.Lock()
	originalWriter := gin.DefaultWriter
	originalErrorWriter := gin.DefaultErrorWriter
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = originalWriter
		gin.DefaultErrorWriter = originalErrorWriter
		common.LogWriterMu.Unlock()

		setupLogLock.Lock()
		testFile := currentLogFile
		currentLogPath = originalPath
		currentLogFile = originalFile
		currentLogDate = originalDate
		currentLogSequence = originalSequence
		currentLogCount = originalCount
		*common.LogDir = originalLogDir
		setupLogLock.Unlock()
		if testFile != nil {
			require.NoError(t, testFile.Close())
		}
	})
}
