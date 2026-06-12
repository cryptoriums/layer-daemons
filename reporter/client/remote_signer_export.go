package client

import (
	"context"

	"google.golang.org/grpc"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// NewRemoteSignerKeyring is an exported wrapper around newKeyringFromRemoteSigner.
// It lets standalone tools (e.g. cmd/unjail, cmd/create-validator) sign cosmos
// txs through the remote signer using the blind SignRaw path, without a local
// key. These one-shot operator commands sign high-stakes messages that must NOT
// be on the reporter's SignTx allowlist, so they use SignRaw (with the node
// cert, which is authorized for SignRaw) rather than the scope-checked SignTx
// the reporter daemon uses. The chain ID returned by the signer is discarded
// here; tools that need it should call newKeyringFromRemoteSigner directly.
func NewRemoteSignerKeyring(ctx context.Context, keyName, addr, caCert, clientCert, clientKey string) (keyring.Keyring, sdk.AccAddress, *grpc.ClientConn, error) {
	kr, accAddr, _, conn, err := newKeyringFromRemoteSigner(ctx, keyName, addr, caCert, clientCert, clientKey, false)
	return kr, accAddr, conn, err
}

// NewRemoteSignerKeyringTx is like NewRemoteSignerKeyring but signs through the
// scope-checked SignTx RPC (the blind SignRaw is disabled on the hardened
// signer). Use it for operator commands whose message type is on the signer's
// allowlist — e.g. cmd/unjail (MsgUnjail). Commands whose message type is NOT on
// the allowlist (create-validator, create-reporter) cannot be served by the
// hardened signer and are setup-only.
func NewRemoteSignerKeyringTx(ctx context.Context, keyName, addr, caCert, clientCert, clientKey string) (keyring.Keyring, sdk.AccAddress, *grpc.ClientConn, error) {
	kr, accAddr, _, conn, err := newKeyringFromRemoteSigner(ctx, keyName, addr, caCert, clientCert, clientKey, true)
	return kr, accAddr, conn, err
}
