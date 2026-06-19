package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the on-disk configuration for wallet_balancer. The Ethereum address
// and Binance credentials are supplied here (per Krasi); signing uses a local
// file key, so there is no remote-signer section.
type Config struct {
	// Chain connectivity.
	Node      string `yaml:"node"`       // CometBFT RPC, e.g. tcp://127.0.0.1:26657
	ChainID   string `yaml:"chain_id"`   // e.g. tellor-1
	GasPrices string `yaml:"gas_prices"` // e.g. 0.000025loya
	Gas       uint64 `yaml:"gas"`        // gas limit per tx

	// Local-key signing (no remote signer).
	KeyringBackend  string `yaml:"keyring_backend"`   // file | os | test
	KeyringDir      string `yaml:"keyring_dir"`       // home dir holding the keyring
	KeyName         string `yaml:"key_name"`          // key to sign with (the reporter/operator key)
	KeyPasswordFile string `yaml:"key_password_file"` // optional: passphrase file for the "file" backend

	// Optional explicit validator operator address (tellorvaloper...). If empty it
	// is derived from the signing account address.
	ValidatorAddress string `yaml:"validator_address"`

	// Scheduling.
	Interval time.Duration `yaml:"interval"` // run cycle every interval (default 24h)

	// Unbonding.
	UnbondAmountLoya uint64 `yaml:"unbond_amount_loya"` // amount to undelegate each cycle (0 = skip)

	// Bridge.
	BalanceToKeepLoya  uint64 `yaml:"balance_to_keep_loya"` // keep this much on the layer wallet
	BridgeEthAddress   string `yaml:"bridge_eth_address"`   // destination ETH address (funds Binance)
	BridgeThresholdTRB uint64 `yaml:"bridge_threshold_trb"` // bridge only when wallet TRB >= this

	// Binance trading.
	BinanceAPIKey    string `yaml:"binance_api_key"`
	BinanceSecretKey string `yaml:"binance_secret_key"`
	BinanceAPIURL    string `yaml:"binance_api_url"` // optional override (testing)

	// Observability.
	MetricsAddr string `yaml:"metrics_addr"` // Prometheus listen addr, e.g. :9095
}

const (
	loyaPerTRB     = 1_000_000 // 1 TRB = 1e6 loya
	gasReserveLoya = 1_000_000 // keep 1 TRB for gas, matches reporter AutoBridge
)

// LoadConfig reads and validates the YAML config at path.
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Node == "" {
		c.Node = "tcp://127.0.0.1:26657"
	}
	if c.GasPrices == "" {
		c.GasPrices = "0.000025loya"
	}
	if c.Gas == 0 {
		c.Gas = 400000
	}
	if c.KeyringBackend == "" {
		c.KeyringBackend = "file"
	}
	if c.KeyName == "" {
		c.KeyName = "reporter"
	}
	if c.Interval == 0 {
		c.Interval = 24 * time.Hour
	}
	if c.MetricsAddr == "" {
		c.MetricsAddr = ":9095"
	}
}

func (c *Config) validate() error {
	if c.ChainID == "" {
		return fmt.Errorf("chain_id is required")
	}
	if c.KeyringDir == "" {
		return fmt.Errorf("keyring_dir is required")
	}
	if c.BridgeEthAddress == "" {
		return fmt.Errorf("bridge_eth_address is required")
	}
	if _, err := normalizeEthAddr(c.BridgeEthAddress); err != nil {
		return fmt.Errorf("bridge_eth_address: %w", err)
	}
	if c.BinanceAPIKey == "" || c.BinanceSecretKey == "" {
		return fmt.Errorf("binance_api_key and binance_secret_key are required")
	}
	return nil
}

// normalizeEthAddr validates an Ethereum address and returns it without the 0x
// prefix (the form MsgWithdrawTokens.Recipient expects).
func normalizeEthAddr(addr string) (string, error) {
	a := strings.TrimSpace(addr)
	a = strings.TrimPrefix(a, "0x")
	a = strings.TrimPrefix(a, "0X")
	if len(a) != 40 {
		return "", fmt.Errorf("expected 40 hex chars (got %d)", len(a))
	}
	for _, r := range a {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
		if !isHex {
			return "", fmt.Errorf("not a hex address: %q", addr)
		}
	}
	return a, nil
}
