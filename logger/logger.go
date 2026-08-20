package logger

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

const (
	loggerINFO  = "INFO"
	loggerWarn  = "WARN"
	loggerError = "ERR"
	loggerDebug = "DEBUG"
	maxLogCount = 1000000
)

var setupLogLock sync.Mutex
var currentLogPath string
var currentLogPathMu sync.RWMutex
var currentLogFile *os.File
var currentLogDate string
var currentLogSequence int
var currentLogCount int
var logFileWriter = &rotatingLogWriter{}

type rotatingLogWriter struct{}

func (*rotatingLogWriter) Write(data []byte) (int, error) {
	return writeLogAt(time.Now(), data)
}

func writeLogAt(now time.Time, data []byte) (int, error) {
	setupLogLock.Lock()
	defer setupLogLock.Unlock()
	if currentLogCount >= maxLogCount {
		if err := setupLoggerAtLocked(now, true); err != nil {
			return 0, err
		}
	} else if err := setupLoggerAtLocked(now, false); err != nil {
		return 0, err
	}
	if currentLogFile == nil {
		return len(data), nil
	}
	n, err := currentLogFile.Write(data)
	if err == nil {
		currentLogCount += strings.Count(string(data), "\n")
	}
	return n, err
}

func GetCurrentLogPath() string {
	currentLogPathMu.RLock()
	defer currentLogPathMu.RUnlock()
	return currentLogPath
}

func SetupLogger() {
	setupLoggerAt(time.Now())
	common.LogWriterMu.Lock()
	gin.DefaultWriter = io.MultiWriter(os.Stdout, logFileWriter)
	gin.DefaultErrorWriter = io.MultiWriter(os.Stderr, logFileWriter)
	common.LogWriterMu.Unlock()
}

func setupLoggerAt(now time.Time) {
	setupLogLock.Lock()
	defer setupLogLock.Unlock()
	if err := setupLoggerAtLocked(now, false); err != nil {
		log.Printf("failed to open log file: %v", err)
	}
}

func setupLoggerAtLocked(now time.Time, forceNextFile bool) error {
	if *common.LogDir == "" {
		return nil
	}
	date := now.Format("20060102")
	if currentLogFile != nil && currentLogDate == date && !forceNextFile {
		return nil
	}
	logDir := filepath.Join(*common.LogDir, date)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}
	if currentLogDate != date {
		currentLogSequence = latestLogSequence(logDir)
		currentLogCount = 0
	} else if forceNextFile {
		currentLogSequence++
		currentLogCount = 0
	}

	logPath := filepath.Join(logDir, logFileName(currentLogSequence))
	for {
		count, err := countLogLines(logPath)
		if err != nil {
			return err
		}
		if count < maxLogCount {
			currentLogCount = count
			break
		}
		currentLogSequence++
		logPath = filepath.Join(logDir, logFileName(currentLogSequence))
	}
	fd, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	currentLogPathMu.Lock()
	oldFile := currentLogFile
	currentLogPath = logPath
	currentLogFile = fd
	currentLogDate = date
	currentLogPathMu.Unlock()

	if oldFile != nil {
		_ = oldFile.Close()
	}
	return nil
}

func logFileName(sequence int) string {
	if sequence == 0 {
		return "oneapi.log"
	}
	return fmt.Sprintf("oneapi-%d.log", sequence)
}

func latestLogSequence(logDir string) int {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return 0
	}

	latest := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "oneapi.log" {
			continue
		}
		if !strings.HasPrefix(name, "oneapi-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		sequence, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "oneapi-"), ".log"))
		if err == nil && sequence > latest {
			latest = sequence
		}
	}
	return latest
}

func countLogLines(path string) (int, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := 0
	for scanner.Scan() {
		lines++
	}
	return lines, scanner.Err()
}

func LogInfo(ctx context.Context, msg string) {
	logHelper(ctx, loggerINFO, msg)
}

func LogWarn(ctx context.Context, msg string) {
	logHelper(ctx, loggerWarn, msg)
}

func LogError(ctx context.Context, msg string) {
	logHelper(ctx, loggerError, msg)
}

func LogDebug(ctx context.Context, msg string, args ...any) {
	if common.DebugEnabled {
		if len(args) > 0 {
			msg = fmt.Sprintf(msg, args...)
		}
		logHelper(ctx, loggerDebug, msg)
	}
}

func logHelper(ctx context.Context, level string, msg string) {
	var id any = "SYSTEM"
	if ctx != nil {
		if requestID := ctx.Value(common.RequestIdKey); requestID != nil {
			id = requestID
		}
	}
	now := time.Now()
	common.LogWriterMu.RLock()
	writer := gin.DefaultErrorWriter
	if level == loggerINFO {
		writer = gin.DefaultWriter
	}
	_, _ = fmt.Fprintf(writer, "[%s] %v | %s | %s \n", level, now.Format("2006/01/02 - 15:04:05"), id, msg)
	common.LogWriterMu.RUnlock()
}

func LogQuota(quota int) string {
	// 新逻辑：根据额度展示类型输出
	q := float64(quota)
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		usd := q / common.QuotaPerUnit
		cny := usd * operation_setting.USDExchangeRate
		return fmt.Sprintf("¥%.6f 额度", cny)
	case operation_setting.QuotaDisplayTypeCustom:
		usd := q / common.QuotaPerUnit
		rate := operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
		symbol := operation_setting.GetGeneralSetting().CustomCurrencySymbol
		if symbol == "" {
			symbol = "¤"
		}
		if rate <= 0 {
			rate = 1
		}
		v := usd * rate
		return fmt.Sprintf("%s%.6f 额度", symbol, v)
	case operation_setting.QuotaDisplayTypeTokens:
		return fmt.Sprintf("%d 点额度", quota)
	default: // USD
		return fmt.Sprintf("＄%.6f 额度", q/common.QuotaPerUnit)
	}
}

func FormatQuota(quota int) string {
	q := float64(quota)
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		usd := q / common.QuotaPerUnit
		cny := usd * operation_setting.USDExchangeRate
		return fmt.Sprintf("¥%.6f", cny)
	case operation_setting.QuotaDisplayTypeCustom:
		usd := q / common.QuotaPerUnit
		rate := operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
		symbol := operation_setting.GetGeneralSetting().CustomCurrencySymbol
		if symbol == "" {
			symbol = "¤"
		}
		if rate <= 0 {
			rate = 1
		}
		v := usd * rate
		return fmt.Sprintf("%s%.6f", symbol, v)
	case operation_setting.QuotaDisplayTypeTokens:
		return fmt.Sprintf("%d", quota)
	default:
		return fmt.Sprintf("＄%.6f", q/common.QuotaPerUnit)
	}
}

// LogJson 仅供测试使用 only for test
func LogJson(ctx context.Context, msg string, obj any) {
	if !common.DebugEnabled {
		return
	}
	jsonStr, err := common.Marshal(obj)
	if err != nil {
		LogError(ctx, fmt.Sprintf("json marshal failed: %s", err.Error()))
		return
	}
	LogDebug(ctx, "%s | %s", msg, jsonStr)
}
