package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	binance "github.com/adshao/go-binance/v2"

	"cosmossdk.io/log"
)

// trader wraps the Binance client and converts TRB held on the Binance account
// into BTC: sell TRB->USDC (market), then buy BTC with the USDC (market).
type trader struct {
	client *binance.Client
	logger log.Logger
}

func newTrader(ctx context.Context, cfg *Config, logger log.Logger) (*trader, error) {
	c := binance.NewClient(cfg.BinanceAPIKey, cfg.BinanceSecretKey)
	if cfg.BinanceAPIURL != "" {
		c.BaseURL = cfg.BinanceAPIURL
	}
	// Validate credentials early.
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := c.NewGetAccountService().Do(checkCtx); err != nil {
		return nil, fmt.Errorf("binance auth: %w", err)
	}
	return &trader{client: c, logger: logger}, nil
}

// freeBalance returns the free (available) balance for an asset on Binance.
func (t *trader) freeBalance(ctx context.Context, asset string) (float64, error) {
	acct, err := t.client.NewGetAccountService().Do(ctx)
	if err != nil {
		return 0, fmt.Errorf("get account: %w", err)
	}
	for _, b := range acct.Balances {
		if b.Asset == asset {
			return strconv.ParseFloat(b.Free, 64)
		}
	}
	return 0, nil
}

// convertTRBtoBTC sells all free TRB on Binance for USDC, then buys BTC with the
// resulting USDC. Returns the BTC bought. A no-op (0, nil) when there is no TRB.
func (t *trader) convertTRBtoBTC(ctx context.Context) (float64, error) {
	trb, err := t.freeBalance(ctx, "TRB")
	if err != nil {
		return 0, err
	}
	if trb < 0.5 { // Binance min order size guard
		t.logger.Info("no TRB to convert on Binance", "trb", trb)
		return 0, nil
	}

	// Sell TRB -> USDC at market.
	qty := strconv.FormatFloat(trb, 'f', 2, 64)
	if _, err := t.client.NewCreateOrderService().
		Symbol("TRBUSDC").Side(binance.SideTypeSell).
		Type(binance.OrderTypeMarket).Quantity(qty).Do(ctx); err != nil {
		return 0, fmt.Errorf("sell TRBUSDC: %w", err)
	}
	time.Sleep(time.Second) // let Binance settle the fill

	usdc, err := t.freeBalance(ctx, "USDC")
	if err != nil {
		return 0, err
	}
	if usdc <= 0 {
		return 0, fmt.Errorf("no USDC after selling TRB")
	}

	// Buy BTC with the USDC at market (quoteOrderQty spends a USDC amount).
	spend := strconv.FormatFloat(usdc*0.999, 'f', 2, 64) // small headroom for fees/precision
	btcBefore, _ := t.freeBalance(ctx, "BTC")
	if _, err := t.client.NewCreateOrderService().
		Symbol("BTCUSDC").Side(binance.SideTypeBuy).
		Type(binance.OrderTypeMarket).QuoteOrderQty(spend).Do(ctx); err != nil {
		return 0, fmt.Errorf("buy BTCUSDC: %w", err)
	}
	time.Sleep(time.Second)
	btcAfter, _ := t.freeBalance(ctx, "BTC")

	bought := btcAfter - btcBefore
	t.logger.Info("converted TRB to BTC on Binance", "trb_sold", trb, "btc_bought", bought)
	return bought, nil
}
