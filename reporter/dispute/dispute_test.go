package dispute

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"cosmossdk.io/log"
)

var queryInterval = 50 * time.Millisecond

func testLogger() log.Logger {
	return log.NewLogger(os.Stderr, log.LevelOption(zerolog.DebugLevel), log.ColorOption(false))
}

// mockOpenDisputesResponse returns the open-disputes JSON. IDs are encoded as strings to
// match cosmos proto-JSON for uint64.
func mockOpenDisputesResponse(ids []uint64) []byte {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = strconv.FormatUint(id, 10)
	}
	resp := map[string]any{"openDisputes": map[string]any{"ids": strs}}
	b, _ := json.Marshal(resp)
	return b
}

func serveDisputes(ids []uint64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mockOpenDisputesResponse(ids))
	}))
}

// runExpectPanic runs the monitor and returns the recovered panic value (or nil).
func runExpectPanic(t *testing.T, cfg Config) (panicked bool, msg string) {
	t.Helper()
	cfg.Enabled = true
	m := New(testLogger(), cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	panicCh := make(chan any, 1)
	doneCh := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				panicCh <- r
			}
			close(doneCh)
		}()
		m.Run(ctx)
	}()

	select {
	case p := <-panicCh:
		s, _ := p.(string)
		return true, s
	case <-doneCh:
		return false, ""
	case <-time.After(10 * queryInterval):
		return false, "timeout"
	}
}

func TestPanicsOnOpenDisputes_MultiServer(t *testing.T) {
	open := serveDisputes([]uint64{1, 2, 3})
	defer open.Close()
	none := serveDisputes([]uint64{})
	defer none.Close()

	// One server open, one none → must panic.
	panicked, msg := runExpectPanic(t, Config{LayerAPIURLs: []string{open.URL, none.URL}, CheckInterval: queryInterval})
	if !panicked || !strings.Contains(msg, ReasonOpenDisputes) {
		t.Fatalf("expected panic with %q, got panicked=%v msg=%q", ReasonOpenDisputes, panicked, msg)
	}

	// Both servers open → must panic.
	open2 := serveDisputes([]uint64{4, 5})
	defer open2.Close()
	panicked, msg = runExpectPanic(t, Config{LayerAPIURLs: []string{open.URL, open2.URL}, CheckInterval: queryInterval})
	if !panicked || !strings.Contains(msg, "OPEN DISPUTES DETECTED") {
		t.Fatalf("expected panic (both open), got panicked=%v msg=%q", panicked, msg)
	}
}

func TestDoesNotPanicOnErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if panicked, msg := runExpectPanic(t, Config{LayerAPIURLs: []string{srv.URL}, CheckInterval: queryInterval}); panicked {
		t.Fatalf("monitor panicked on API errors (should not): %q", msg)
	}
}

func TestDoesNotPanicWhenNoOpenDisputes(t *testing.T) {
	srv := serveDisputes([]uint64{})
	defer srv.Close()
	if panicked, msg := runExpectPanic(t, Config{LayerAPIURLs: []string{srv.URL}, CheckInterval: queryInterval}); panicked {
		t.Fatalf("monitor panicked with no open disputes (should not): %q", msg)
	}
}

func TestDoesNotPanicWhenDisputeIsIgnored(t *testing.T) {
	srv := serveDisputes([]uint64{42})
	defer srv.Close()
	if panicked, msg := runExpectPanic(t, Config{
		LayerAPIURLs:   []string{srv.URL},
		IgnoreDisputes: []uint64{42},
		CheckInterval:  queryInterval,
	}); panicked {
		t.Fatalf("monitor panicked on an ignored dispute (should not): %q", msg)
	}
}

func TestDisabledMonitorDoesNothing(t *testing.T) {
	srv := serveDisputes([]uint64{7})
	defer srv.Close()
	m := New(testLogger(), Config{Enabled: false, LayerAPIURLs: []string{srv.URL}, CheckInterval: queryInterval})
	// CheckBeforeStart and Run must be no-ops when disabled (no panic).
	m.CheckBeforeStart(context.Background())
}

func TestIsIgnored(t *testing.T) {
	if !isIgnored([]uint64{1, 2, 3}, 2) {
		t.Fatal("2 should be ignored")
	}
	if isIgnored([]uint64{1, 2, 3}, 9) {
		t.Fatal("9 should not be ignored")
	}
}

func TestParseDisputeID(t *testing.T) {
	id, err := ParseDisputeID("123")
	if err != nil || id != 123 {
		t.Fatalf("ParseDisputeID(123) = %d, %v", id, err)
	}
	if _, err := ParseDisputeID("nope"); err == nil {
		t.Fatal("expected error for non-numeric id")
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("DISPUTE_MONITOR_ENABLED", "true")
	t.Setenv("API_URLS", "http://a:1317, http://b:1317")
	t.Setenv("DISPUTE_IGNORE_IDS", "5, 9")
	t.Setenv("DISPUTE_CHECK_INTERVAL", "3s")
	cfg := LoadConfigFromEnv([]string{"tcp://rpc:26657"})
	if !cfg.Enabled {
		t.Fatal("expected enabled")
	}
	if len(cfg.LayerAPIURLs) != 2 || cfg.LayerAPIURLs[1] != "http://b:1317" {
		t.Fatalf("api urls: %v", cfg.LayerAPIURLs)
	}
	if len(cfg.IgnoreDisputes) != 2 || cfg.IgnoreDisputes[0] != 5 {
		t.Fatalf("ignore: %v", cfg.IgnoreDisputes)
	}
	if cfg.CheckInterval != 3*time.Second {
		t.Fatalf("interval: %v", cfg.CheckInterval)
	}
	if len(cfg.RPCEndpoints) != 1 {
		t.Fatalf("rpc: %v", cfg.RPCEndpoints)
	}
}
