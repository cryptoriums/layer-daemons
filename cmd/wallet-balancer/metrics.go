package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"cosmossdk.io/log"
)

// metrics holds the Prometheus instruments wallet_balancer exposes. Krasi scrapes
// these instead of the (now-disabled) reporter auto-balancing metrics.
type metrics struct {
	lastRun      prometheus.Gauge
	cycleErrors  *prometheus.CounterVec
	tipsClaimed  prometheus.Counter
	unbonded     prometheus.Counter
	bridged      prometheus.Counter
	btcConverted prometheus.Counter
}

func newMetrics(reg prometheus.Registerer) *metrics {
	ns := "wallet_balancer"
	return &metrics{
		lastRun: promauto.With(reg).NewGauge(prometheus.GaugeOpts{
			Namespace: ns, Name: "last_run_timestamp_seconds",
			Help: "Unix time of the last completed balancing cycle.",
		}),
		cycleErrors: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Namespace: ns, Name: "errors_total",
			Help: "Errors per action.",
		}, []string{"action"}),
		tipsClaimed: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: ns, Name: "tips_claimed_total",
			Help: "Number of successful claim-tips txs.",
		}),
		unbonded: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: ns, Name: "unbonded_loya_total",
			Help: "Total loya submitted for undelegation.",
		}),
		bridged: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: ns, Name: "bridged_loya_total",
			Help: "Total loya bridged to the Ethereum address.",
		}),
		btcConverted: promauto.With(reg).NewCounter(prometheus.CounterOpts{
			Namespace: ns, Name: "btc_converted_total",
			Help: "Total BTC bought from converted TRB rewards.",
		}),
	}
}

// serveMetrics starts the Prometheus HTTP endpoint in a goroutine.
func serveMetrics(addr string, reg *prometheus.Registry, logger log.Logger) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		logger.Info("serving metrics", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server stopped", "error", err)
		}
	}()
}
