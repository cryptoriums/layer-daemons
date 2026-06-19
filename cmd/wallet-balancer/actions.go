package main

import (
	"context"
	"fmt"

	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	bridgetypes "github.com/tellor-io/layer/x/bridge/types"
	reportertypes "github.com/tellor-io/layer/x/reporter/types"
)

// claimTips withdraws the reporter's accumulated tips (MsgWithdrawTip).
func (s *signer) claimTips(ctx context.Context) error {
	msg := &reportertypes.MsgWithdrawTip{
		SelectorAddress:  s.fromAddr.String(),
		ValidatorAddress: s.validatorOperator(),
	}
	res, err := s.signAndBroadcast(ctx, msg)
	if err != nil {
		return err
	}
	_ = res
	return nil
}

// autoUnbond undelegates unbondAmountLoya from the validator (MsgUndelegate).
func (s *signer) autoUnbond(ctx context.Context, amountLoya uint64) error {
	msg := &stakingtypes.MsgUndelegate{
		DelegatorAddress: s.fromAddr.String(),
		ValidatorAddress: s.validatorOperator(),
		Amount:           sdk.NewCoin("loya", math.NewIntFromUint64(amountLoya)),
	}
	_, err := s.signAndBroadcast(ctx, msg)
	return err
}

// walletLoya queries the signing account's loya balance via the RPC node's ABCI.
func (s *signer) walletLoya(ctx context.Context) (math.Int, error) {
	bankQ := banktypes.NewQueryClient(s.clientCtx)
	resp, err := bankQ.Balance(ctx, &banktypes.QueryBalanceRequest{
		Address: s.fromAddr.String(),
		Denom:   "loya",
	})
	if err != nil {
		return math.ZeroInt(), fmt.Errorf("query balance: %w", err)
	}
	if resp.Balance == nil {
		return math.ZeroInt(), nil
	}
	return resp.Balance.Amount, nil
}

// bridgeExcess bridges (wallet - keep - gasReserve) loya to the ETH address when
// the wallet holds at least bridge_threshold_trb. Returns the bridged amount (0
// if below threshold / nothing to bridge).
func (s *signer) bridgeExcess(ctx context.Context, ethAddr string) (math.Int, error) {
	wallet, err := s.walletLoya(ctx)
	if err != nil {
		return math.ZeroInt(), err
	}

	threshold := math.NewIntFromUint64(s.cfg.BridgeThresholdTRB).MulRaw(loyaPerTRB)
	if wallet.LT(threshold) {
		return math.ZeroInt(), nil
	}

	keep := math.NewIntFromUint64(s.cfg.BalanceToKeepLoya)
	amount := wallet.Sub(keep).Sub(math.NewInt(gasReserveLoya))
	if !amount.IsPositive() {
		return math.ZeroInt(), nil
	}

	msg := &bridgetypes.MsgWithdrawTokens{
		Creator:   s.fromAddr.String(),
		Recipient: ethAddr, // already normalized (no 0x)
		Amount:    sdk.NewCoin("loya", amount),
	}
	if _, err := s.signAndBroadcast(ctx, msg); err != nil {
		return math.ZeroInt(), err
	}
	return amount, nil
}
