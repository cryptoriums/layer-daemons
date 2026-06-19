// Command wallet-balancer runs, once per interval (default 24h):
//  1. claim all reporter tips
//  2. auto-unbond a configured amount of stake
//  3. convert TRB held on Binance into BTC
//  4. when the layer wallet holds >= bridge_threshold_trb, bridge the excess to a
//     configured Ethereum address (which funds the Binance trading account)
//
// It signs with a LOCAL file key (not the remote signer / signer container) and is
// meant to run on the signer server. The Ethereum address and Binance credentials
// come from the YAML config. Steps are independent: a failure in one is logged,
// counted, and the cycle continues. Prometheus metrics are exposed for scraping.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"cosmossdk.io/log"

	// sets the tellor bech32 address prefix via init()
	_ "github.com/tellor-io/layer/app/config"
)

func main() {
	configPath := flag.String("config", "wallet-balancer.yaml", "path to YAML config")
	once := flag.Bool("once", false, "run a single cycle and exit (no scheduler)")
	flag.Parse()

	logger := log.NewLogger(os.Stdout).With("module", "wallet-balancer")

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}
	ethAddr, err := normalizeEthAddr(cfg.BridgeEthAddress)
	if err != nil {
		logger.Error("bridge address", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	reg := prometheus.NewRegistry()
	m := newMetrics(reg)
	serveMetrics(cfg.MetricsAddr, reg, logger)

	sgnr, err := newSigner(cfg)
	if err != nil {
		logger.Error("init signer", "error", err)
		os.Exit(1)
	}
	logger.Info("wallet-balancer ready",
		"account", sgnr.fromAddr.String(),
		"validator", sgnr.validatorOperator(),
		"interval", cfg.Interval.String(),
	)

	trd, err := newTrader(ctx, cfg, logger)
	if err != nil {
		logger.Error("init binance trader", "error", err)
		os.Exit(1)
	}

	run := func() {
		runCycle(ctx, cfg, ethAddr, sgnr, trd, m, logger)
	}

	// First cycle immediately, then on the interval.
	run()
	if *once {
		return
	}
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down")
			return
		case <-ticker.C:
			run()
		}
	}
}

// runCycle executes the four steps in order. Each step is independent: errors are
// logged and counted, and the cycle proceeds to the next step.
func runCycle(ctx context.Context, cfg *Config, ethAddr string, s *signer, t *trader, m *metrics, logger log.Logger) {
	logger.Info("starting balancing cycle")

	// 1. Claim tips.
	if err := s.claimTips(ctx); err != nil {
		logger.Error("claim tips failed", "error", err)
		m.cycleErrors.WithLabelValues("claim_tips").Inc()
	} else {
		logger.Info("claimed tips")
		m.tipsClaimed.Inc()
	}

	// 2. Auto-unbond.
	if cfg.UnbondAmountLoya > 0 {
		if err := s.autoUnbond(ctx, cfg.UnbondAmountLoya); err != nil {
			logger.Error("auto-unbond failed", "error", err)
			m.cycleErrors.WithLabelValues("unbond").Inc()
		} else {
			logger.Info("unbonded stake", "loya", cfg.UnbondAmountLoya)
			m.unbonded.Add(float64(cfg.UnbondAmountLoya))
		}
	}

	// 3. Convert TRB -> BTC on Binance.
	if btc, err := t.convertTRBtoBTC(ctx); err != nil {
		logger.Error("binance conversion failed", "error", err)
		m.cycleErrors.WithLabelValues("trade").Inc()
	} else if btc > 0 {
		m.btcConverted.Add(btc)
	}

	// 4. Bridge excess TRB to the ETH address (funds Binance).
	if bridged, err := s.bridgeExcess(ctx, ethAddr); err != nil {
		logger.Error("bridge failed", "error", err)
		m.cycleErrors.WithLabelValues("bridge").Inc()
	} else if bridged.IsPositive() {
		logger.Info("bridged excess to eth", "loya", bridged.String(), "eth", "0x"+ethAddr)
		m.bridged.Add(float64(bridged.Int64()))
	} else {
		logger.Info("wallet below bridge threshold, nothing bridged")
	}

	m.lastRun.SetToCurrentTime()
	logger.Info("balancing cycle complete")
}
