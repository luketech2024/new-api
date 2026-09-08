package observability

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
)

type Metrics struct {
	store    *store.Store
	mu       sync.Mutex
	requests map[requestMetricKey]requestMetric
	channel  map[channelMetricKey]uint64
}

type channelMetricKey struct {
	Channel string
	Event   string
}

type requestMetricKey struct {
	Route  string
	Method string
	Status int
}

type requestMetric struct {
	Count      uint64
	DurationNS uint64
}

type Logger struct {
	logger *slog.Logger
}

const maxLogCount = 1000000

type dailyLogWriter struct {
	dir      string
	now      func() time.Time
	mu       sync.Mutex
	date     string
	sequence int
	count    int
	file     *os.File
}

func NewMetrics(database *store.Store) *Metrics {
	return &Metrics{store: database, requests: make(map[requestMetricKey]requestMetric), channel: make(map[channelMetricKey]uint64)}
}

func NewLogger(level, logDir string) (*Logger, error) {
	parsed := new(slog.LevelVar)
	if err := parsed.UnmarshalText([]byte(level)); err != nil {
		parsed.Set(slog.LevelInfo)
	}
	writer := &dailyLogWriter{dir: logDir, now: time.Now}
	writer.mu.Lock()
	err := writer.rotateLocked(false)
	writer.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("initialize adapter log file: %w", err)
	}
	return &Logger{logger: slog.New(slog.NewJSONHandler(io.MultiWriter(os.Stdout, writer), &slog.HandlerOptions{Level: parsed}))}, nil
}

func (w *dailyLogWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.rotateLocked(w.count >= maxLogCount); err != nil {
		return 0, err
	}
	n, err := w.file.Write(data)
	if err == nil {
		w.count++
	}
	return n, err
}

func (w *dailyLogWriter) rotateLocked(forceNextFile bool) error {
	date := w.now().Format("20060102")
	if w.file != nil && w.date == date && !forceNextFile {
		return nil
	}
	if w.date != date {
		w.sequence = 0
		w.count = 0
	} else if forceNextFile {
		w.sequence++
		w.count = 0
	}
	logDir := filepath.Join(w.dir, date)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	name := "wechat-epay.log"
	if w.sequence > 0 {
		name = fmt.Sprintf("wechat-epay-%d.log", w.sequence)
	}
	file, err := os.OpenFile(filepath.Join(logDir, name), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	previous := w.file
	w.file = file
	w.date = date
	if previous != nil {
		return previous.Close()
	}
	return nil
}

func (m *Metrics) ObserveChannel(channel, event string) {
	if channel != "wxpay" && channel != "alipay" {
		channel = "unknown"
	}
	switch event {
	case "created", "paid", "notify_fail", "review", "backlog", "verify_fail":
	default:
		event = "other"
	}
	m.mu.Lock()
	m.channel[channelMetricKey{Channel: channel, Event: event}]++
	m.mu.Unlock()
}

func (m *Metrics) ObserveRequest(route, method string, status int, duration time.Duration) {
	if route == "" {
		route = "unmatched"
	}
	key := requestMetricKey{Route: route, Method: method, Status: status}
	m.mu.Lock()
	metric := m.requests[key]
	metric.Count++
	metric.DurationNS += uint64(duration)
	m.requests[key] = metric
	m.mu.Unlock()
}

func (l *Logger) LogRequest(requestID, method, route string, status int, duration time.Duration, rejectReason string) {
	attributes := []any{
		slog.String("request_id", requestID),
		slog.String("method", method),
		slog.String("route", route),
		slog.Int("status", status),
		slog.Int64("duration_ms", duration.Milliseconds()),
	}
	if rejectReason == "" {
		l.logger.Info("http request completed", attributes...)
		return
	}
	l.logger.Warn("http request refused", append(attributes, slog.String("reject_reason", rejectReason))...)
}

func (m *Metrics) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	orderCounts, taskCounts, err := m.store.StateCounts()
	if err != nil {
		http.Error(writer, "metrics unavailable", http.StatusServiceUnavailable)
		return
	}
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	var output strings.Builder
	output.WriteString("# HELP payment_order_state Number of payment orders by state.\n# TYPE payment_order_state gauge\n")
	for _, count := range orderCounts {
		fmt.Fprintf(&output, "payment_order_state{state=%q} %d\n", count.State, count.Count)
	}
	output.WriteString("# HELP notification_tasks_pending Number of notification tasks by state.\n# TYPE notification_tasks_pending gauge\n")
	for _, count := range taskCounts {
		fmt.Fprintf(&output, "notification_tasks_pending{state=%q} %d\n", count.State, count.Count)
	}
	output.WriteString("# HELP payment_channel_events Payment adapter events by channel.\n# TYPE payment_channel_events counter\n")
	m.mu.Lock()
	channelKeys := make([]channelMetricKey, 0, len(m.channel))
	for key := range m.channel {
		channelKeys = append(channelKeys, key)
	}
	sort.Slice(channelKeys, func(i, j int) bool {
		return channelKeys[i].Channel+channelKeys[i].Event < channelKeys[j].Channel+channelKeys[j].Event
	})
	for _, key := range channelKeys {
		fmt.Fprintf(&output, "payment_channel_events{channel=%q,event=%q} %d\n", key.Channel, key.Event, m.channel[key])
	}
	output.WriteString("# HELP http_request_duration_seconds HTTP request duration.\n# TYPE http_request_duration_seconds summary\n")
	keys := make([]requestMetricKey, 0, len(m.requests))
	for key := range m.requests {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Route+keys[i].Method+strconv.Itoa(keys[i].Status) < keys[j].Route+keys[j].Method+strconv.Itoa(keys[j].Status)
	})
	for _, key := range keys {
		metric := m.requests[key]
		labels := fmt.Sprintf("route=%q,method=%q,status=%q", key.Route, key.Method, strconv.Itoa(key.Status))
		fmt.Fprintf(&output, "http_request_duration_seconds_sum{%s} %.9f\n", labels, float64(metric.DurationNS)/float64(time.Second))
		fmt.Fprintf(&output, "http_request_duration_seconds_count{%s} %d\n", labels, metric.Count)
	}
	m.mu.Unlock()
	_, _ = writer.Write([]byte(output.String()))
}
