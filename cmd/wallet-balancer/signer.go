package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	rsclient "github.com/tellor-io/layer-daemons/reporter/client"

	cosmosclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

// signer holds the cosmos client context and local keyring used to sign and
// broadcast txs with a local file key (no remote signer, no signer container).
type signer struct {
	cfg       *Config
	ec        rsclient.EncodingConfig
	clientCtx cosmosclient.Context
	kr        keyring.Keyring
	fromAddr  sdk.AccAddress
}

// newSigner builds the encoding config, opens the local keyring, and assembles a
// cosmos client context wired to the RPC node.
func newSigner(cfg *Config) (*signer, error) {
	ec := rsclient.CreateEncodingConfig()

	input, err := keyringInput(cfg)
	if err != nil {
		return nil, err
	}
	kr, err := keyring.New("", cfg.KeyringBackend, cfg.KeyringDir, input, ec.Codec)
	if err != nil {
		return nil, fmt.Errorf("open keyring (backend=%s dir=%s): %w", cfg.KeyringBackend, cfg.KeyringDir, err)
	}
	rec, err := kr.Key(cfg.KeyName)
	if err != nil {
		return nil, fmt.Errorf("key %q not found in keyring: %w", cfg.KeyName, err)
	}
	fromAddr, err := rec.GetAddress()
	if err != nil {
		return nil, fmt.Errorf("get address for key %q: %w", cfg.KeyName, err)
	}

	rpcClient, err := rpchttp.New(cfg.Node, "/websocket")
	if err != nil {
		return nil, fmt.Errorf("create rpc client: %w", err)
	}

	clientCtx := cosmosclient.Context{}.
		WithCodec(ec.Codec).
		WithInterfaceRegistry(ec.InterfaceRegistry).
		WithTxConfig(ec.TxConfig).
		WithChainID(cfg.ChainID).
		WithKeyring(kr).
		WithFromName(cfg.KeyName).
		WithFrom(cfg.KeyName).
		WithFromAddress(fromAddr).
		WithClient(rpcClient).
		WithBroadcastMode("sync").
		WithAccountRetriever(authtypes.AccountRetriever{}).
		WithSkipConfirmation(true)

	return &signer{cfg: cfg, ec: ec, clientCtx: clientCtx, kr: kr, fromAddr: fromAddr}, nil
}

// keyringInput returns the passphrase reader for the keyring. For the "file"
// backend it feeds the passphrase from key_password_file (entered twice, the way
// the cosmos file backend prompts); otherwise stdin.
func keyringInput(cfg *Config) (io.Reader, error) {
	if cfg.KeyringBackend == keyring.BackendFile && cfg.KeyPasswordFile != "" {
		b, err := os.ReadFile(cfg.KeyPasswordFile)
		if err != nil {
			return nil, fmt.Errorf("read key_password_file: %w", err)
		}
		pw := strings.TrimRight(string(b), "\r\n")
		return strings.NewReader(pw + "\n" + pw + "\n"), nil
	}
	return os.Stdin, nil
}

// validatorOperator returns the configured validator operator address, or derives
// it from the signing account (same key bytes, tellorvaloper prefix).
func (s *signer) validatorOperator() string {
	if s.cfg.ValidatorAddress != "" {
		return s.cfg.ValidatorAddress
	}
	return sdk.ValAddress(s.fromAddr).String()
}

// signAndBroadcast signs msg with the local key and broadcasts it as an unordered
// tx (matching the reporter's ADR-070 flow), then returns the broadcast result.
func (s *signer) signAndBroadcast(ctx context.Context, msg sdk.Msg) (*sdk.TxResponse, error) {
	txf := tx.Factory{}.
		WithChainID(s.cfg.ChainID).
		WithKeybase(s.kr).
		WithTxConfig(s.ec.TxConfig).
		WithAccountRetriever(s.clientCtx.AccountRetriever).
		WithGas(s.cfg.Gas).
		WithGasPrices(s.cfg.GasPrices).
		WithSignMode(signing.SignMode_SIGN_MODE_DIRECT).
		WithSequence(0).
		WithUnordered(true).
		WithTimeoutTimestamp(time.Now().Add(60 * time.Second))

	txf, err := txf.Prepare(s.clientCtx)
	if err != nil {
		return nil, fmt.Errorf("prepare tx factory: %w", err)
	}
	txb, err := txf.BuildUnsignedTx(msg)
	if err != nil {
		return nil, fmt.Errorf("build unsigned tx: %w", err)
	}
	if err := tx.Sign(ctx, txf, s.cfg.KeyName, txb, true); err != nil {
		return nil, fmt.Errorf("sign tx: %w", err)
	}
	txBytes, err := s.ec.TxConfig.TxEncoder()(txb.GetTx())
	if err != nil {
		return nil, fmt.Errorf("encode tx: %w", err)
	}
	res, err := s.clientCtx.BroadcastTx(txBytes)
	if err != nil {
		return nil, fmt.Errorf("broadcast tx: %w", err)
	}
	if res.Code != 0 {
		return res, fmt.Errorf("tx failed: code=%d log=%s", res.Code, res.RawLog)
	}
	return res, nil
}
