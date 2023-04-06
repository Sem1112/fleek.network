package keeper

import (
	"testing"

	"github.com/arkeonetwork/arkeo/common"
	"github.com/arkeonetwork/arkeo/common/cosmos"
	"github.com/arkeonetwork/arkeo/x/arkeo/configs"
	"github.com/arkeonetwork/arkeo/x/arkeo/types"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/hd"
	cKeys "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/std"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	var err error
	ctx, k, sk := SetupKeeperWithStaking(t)
	ctx = ctx.WithBlockHeight(20)

	s := newMsgServer(k, sk)

	// setup
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	std.RegisterInterfaces(interfaceRegistry)
	module.NewBasicManager().RegisterInterfaces(interfaceRegistry)
	types.RegisterInterfaces(interfaceRegistry)
	cdc := codec.NewProtoCodec(interfaceRegistry)

	pubkey := types.GetRandomPubKey()
	acc := types.GetRandomBech32Addr()
	service := common.BTCService
	kb := cKeys.NewInMemory(cdc)
	info, _, err := kb.NewMnemonic("whatever", cKeys.English, `m/44'/931'/0'/0/0`, "", hd.Secp256k1)
	require.NoError(t, err)
	pk, err := info.GetPubKey()
	require.NoError(t, err)
	client, err := common.NewPubKeyFromCrypto(pk)
	require.NoError(t, err)
	rate, err := cosmos.ParseCoin("10uarkeo")
	require.NoError(t, err)

	contract := types.NewContract(pubkey, service, client)
	contract.Duration = 100
	contract.Rate = rate
	contract.Height = 10
	contract.Nonce = 0
	contract.MeterType = types.MeterType_PAY_PER_CALL
	contract.Deposit = cosmos.NewInt(contract.Duration * contract.Rate.Amount.Int64())
	contract.Id = 1
	require.NoError(t, k.SetContract(ctx, contract))

	// happy path

	msg := types.MsgClaimContractIncome{
		ContractId: contract.Id,
		Creator:    acc,
		Nonce:      20,
	}

	message := msg.GetBytesToSign()
	msg.Signature, _, err = kb.Sign("whatever", message)
	require.NoError(t, err)
	require.NoError(t, s.ClaimContractIncomeValidate(ctx, &msg))

	// check closed contract
	ctx = ctx.WithBlockHeight(ctx.BlockHeight() + contract.Duration)
	err = s.ClaimContractIncomeValidate(ctx, &msg)
	require.ErrorIs(t, err, types.ErrClaimContractIncomeClosed)
}

func TestHandlePayPerCall(t *testing.T) {
	ctx, k, sk := SetupKeeperWithStaking(t)

	s := newMsgServer(k, sk)

	// setup
	pubkey := types.GetRandomPubKey()
	acc, err := pubkey.GetMyAddress()
	require.NoError(t, err)
	service := common.BTCService
	client := types.GetRandomPubKey()
	require.NoError(t, k.MintToModule(ctx, types.ModuleName, getCoin(common.Tokens(10*100*2))))
	require.NoError(t, k.SendFromModuleToModule(ctx, types.ModuleName, types.ContractName, getCoins(10*100)))
	rate, err := cosmos.ParseCoin("10uarkeo")
	require.NoError(t, err)

	contract := types.NewContract(pubkey, service, client)
	contract.Duration = 100
	contract.Rate = rate
	contract.MeterType = types.MeterType_PAY_PER_CALL
	contract.Deposit = cosmos.NewInt(contract.Duration * contract.Rate.Amount.Int64())
	contract.Id = 2
	require.NoError(t, k.SetContract(ctx, contract))

	// happy path
	msg := types.MsgClaimContractIncome{
		ContractId: contract.Id,
		Creator:    acc,
		Nonce:      20,
	}
	require.NoError(t, s.ClaimContractIncomeHandle(ctx, &msg))

	require.Equal(t, k.GetBalance(ctx, acc).AmountOf(configs.Denom).Int64(), int64(180))
	require.Equal(t, k.GetBalanceOfModule(ctx, types.ContractName, configs.Denom).Int64(), int64(800))
	require.Equal(t, k.GetBalanceOfModule(ctx, types.ReserveName, configs.Denom).Int64(), int64(20))

	// repeat the same thing and ensure we don't pay providers twice
	require.NoError(t, s.ClaimContractIncomeHandle(ctx, &msg))
	require.Equal(t, k.GetBalance(ctx, acc).AmountOf(configs.Denom).Int64(), int64(180))
	require.Equal(t, k.GetBalanceOfModule(ctx, types.ContractName, configs.Denom).Int64(), int64(800))
	require.Equal(t, k.GetBalanceOfModule(ctx, types.ReserveName, configs.Denom).Int64(), int64(20))

	// increase the nonce and get slightly more funds for the provider
	msg.Nonce = 25
	require.NoError(t, s.ClaimContractIncomeHandle(ctx, &msg))
	acct := k.GetBalance(ctx, acc).AmountOf(configs.Denom).Int64()
	require.Equal(t, acct, int64(225))
	cname := k.GetBalanceOfModule(ctx, types.ContractName, configs.Denom).Int64()
	require.Equal(t, cname, int64(750))
	rname := k.GetBalanceOfModule(ctx, types.ReserveName, configs.Denom).Int64()
	require.Equal(t, rname, int64(25))
	require.Equal(t, rname+cname+acct, contract.Rate.Amount.Int64()*contract.Duration)

	// ensure provider cannot take more than what is deposited into the account, overspend the contract
	msg.Nonce = contract.Deposit.Int64() / contract.Rate.Amount.Int64() * 1000000000000
	require.NoError(t, s.ClaimContractIncomeHandle(ctx, &msg))
	acct = k.GetBalance(ctx, acc).AmountOf(configs.Denom).Int64()
	require.Equal(t, acct, int64(900))
	cname = k.GetBalanceOfModule(ctx, types.ContractName, configs.Denom).Int64()
	require.Equal(t, cname, int64(0))
	rname = k.GetBalanceOfModule(ctx, types.ReserveName, configs.Denom).Int64()
	require.Equal(t, rname, int64(100))
	require.Equal(t, rname+cname+acct, contract.Rate.Amount.Int64()*contract.Duration)
}

func TestHandlePayPerBlock(t *testing.T) {
	ctx, k, sk := SetupKeeperWithStaking(t)
	ctx = ctx.WithBlockHeight(20)

	s := newMsgServer(k, sk)

	// setup
	pubkey := types.GetRandomPubKey()
	acc, err := pubkey.GetMyAddress()
	require.NoError(t, err)
	service := common.BTCService
	client := types.GetRandomPubKey()
	require.NoError(t, k.MintToModule(ctx, types.ModuleName, getCoin(common.Tokens(10*100*2))))
	require.NoError(t, k.SendFromModuleToModule(ctx, types.ModuleName, types.ContractName, getCoins(10*100)))
	rate, err := cosmos.ParseCoin("10uarkeo")
	require.NoError(t, err)

	contract := types.NewContract(pubkey, service, client)
	contract.Duration = 100
	contract.Height = 10
	contract.Rate = rate
	contract.MeterType = types.MeterType_PAY_PER_BLOCK
	contract.Deposit = cosmos.NewInt(contract.Duration * contract.Rate.Amount.Int64())
	contract.Id = 3
	require.NoError(t, k.SetContract(ctx, contract))

	// happy path
	msg := types.MsgClaimContractIncome{
		ContractId: contract.Id,
		Creator:    acc,
		Nonce:      20,
	}
	require.NoError(t, s.ClaimContractIncomeHandle(ctx, &msg))

	require.Equal(t, k.GetBalance(ctx, acc).AmountOf(configs.Denom).Int64(), int64(90))
	require.Equal(t, k.GetBalanceOfModule(ctx, types.ContractName, configs.Denom).Int64(), int64(900))
	require.Equal(t, k.GetBalanceOfModule(ctx, types.ReserveName, configs.Denom).Int64(), int64(10))

	// repeat the same thing and ensure we don't pay providers twice
	require.NoError(t, s.ClaimContractIncomeHandle(ctx, &msg))
	require.Equal(t, k.GetBalance(ctx, acc).AmountOf(configs.Denom).Int64(), int64(90))
	require.Equal(t, k.GetBalanceOfModule(ctx, types.ContractName, configs.Denom).Int64(), int64(900))
	require.Equal(t, k.GetBalanceOfModule(ctx, types.ReserveName, configs.Denom).Int64(), int64(10))

	// increase the nonce and get slightly more funds for the provider
	ctx = ctx.WithBlockHeight(30)
	require.NoError(t, s.ClaimContractIncomeHandle(ctx, &msg))
	acct := k.GetBalance(ctx, acc).AmountOf(configs.Denom).Int64()
	require.Equal(t, acct, int64(180))
	cname := k.GetBalanceOfModule(ctx, types.ContractName, configs.Denom).Int64()
	require.Equal(t, cname, int64(800))
	rname := k.GetBalanceOfModule(ctx, types.ReserveName, configs.Denom).Int64()
	require.Equal(t, rname, int64(20))
	require.Equal(t, rname+cname+acct, contract.Rate.Amount.Int64()*contract.Duration)

	// ensure provider cannot take more than what is deposited into the account, overspend the contract
	ctx = ctx.WithBlockHeight(30000000)
	require.NoError(t, s.ClaimContractIncomeHandle(ctx, &msg))
	acct = k.GetBalance(ctx, acc).AmountOf(configs.Denom).Int64()
	require.Equal(t, acct, int64(900))
	cname = k.GetBalanceOfModule(ctx, types.ContractName, configs.Denom).Int64()
	require.Equal(t, cname, int64(0))
	rname = k.GetBalanceOfModule(ctx, types.ReserveName, configs.Denom).Int64()
	require.Equal(t, rname, int64(100))
	require.Equal(t, rname+cname+acct, contract.Rate.Amount.Int64()*contract.Duration)
}
