package appconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cast"

	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
)

// DefaultNodeHome is the default home directory for the layer node.
var DefaultNodeHome string

// CLI flag names.
const (
	GrpcAddress    = "grpc.address"
	GrpcEnable     = "grpc.enable"
	KeyringBackend = "keyring-backend"
)

// Flags contains the values of all flags.
type Flags struct {
	GrpcAddress    string
	GrpcEnable     bool
	KeyringBackend string
}

func init() {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	DefaultNodeHome = filepath.Join(userHomeDir, ".layer")
}

// Validate checks that the flags are valid.
func (f *Flags) Validate() error {
	if !f.GrpcEnable {
		return fmt.Errorf("grpc.enable must be set to true - validating requires gRPC server")
	}
	return nil
}

// GetFlagValuesFromOptions gets values from the AppOptions struct which contains values
// from the command-line flags.
func GetFlagValuesFromOptions(appOpts servertypes.AppOptions) Flags {
	result := Flags{
		GrpcAddress:    config.DefaultGRPCAddress,
		GrpcEnable:     true,
		KeyringBackend: "test",
	}

	if option := appOpts.Get(GrpcAddress); option != nil {
		if v, err := cast.ToStringE(option); err == nil {
			result.GrpcAddress = v
		}
	}

	if option := appOpts.Get(GrpcEnable); option != nil {
		if v, err := cast.ToBoolE(option); err == nil {
			result.GrpcEnable = v
		}
	}

	if option := appOpts.Get(KeyringBackend); option != nil {
		if v, err := cast.ToStringE(option); err == nil {
			result.KeyringBackend = v
		}
	}

	return result
}
