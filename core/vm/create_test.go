package vm_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

func blockCtx() vm.BlockContext {
	return vm.BlockContext{
		CanTransfer: func(db vm.StateDB, addr common.Address, amount *big.Int) bool {
			return db.GetBalance(addr).Cmp(amount) >= 0
		},
		Transfer: func(db vm.StateDB, from, to common.Address, amount *big.Int) {
			db.SubBalance(from, amount)
			db.AddBalance(to, amount)
		},
		GetHash:     func(uint64) common.Hash { return common.Hash{} },
		Coinbase:    common.Address{},
		GasLimit:    30_000_000,
		BlockNumber: big.NewInt(1),
		BaseFee:     big.NewInt(0),
		Time:        big.NewInt(1), // your fork uses *big.Int here
	}
}

func TestCreate_TopLevel_MaxNonce(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db), nil)

	caller := common.HexToAddress("0xCA11ER")
	statedb.AddBalance(caller, big.NewInt(1e18))
	statedb.SetNonce(caller, math.MaxUint64)

	e := vm.NewEVM(blockCtx(), vm.TxContext{Origin: caller}, statedb, params.AllEthashProtocolChanges, vm.Config{})

	gas := uint64(1_000_000)
	ret, addr, left, err := e.Create(vm.AccountRef(caller), []byte{0x00}, gas, big.NewInt(0))
	require.Error(t, err)
	require.Equal(t, vm.ErrNonceMax, err)
	require.Nil(t, ret)
	require.Equal(t, gas, left)                                        // full child gas returned
	require.Equal(t, uint64(math.MaxUint64), statedb.GetNonce(caller)) // no increment
	require.Len(t, statedb.GetCode(addr), 0)                           // nothing deployed
}

func TestCreate_Internal_MaxNonce(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db), nil)

	creator := common.HexToAddress("0xC0DECAFE")
	statedb.AddBalance(creator, big.NewInt(1e18))
	statedb.CreateAccount(creator)
	statedb.SetNonce(creator, math.MaxUint64)

	// runtime: PUSH1 0 (size) PUSH1 0 (offset) PUSH1 0 (value) CREATE STOP
	code := []byte{0x60, 0x00, 0x60, 0x00, 0x60, 0x00, 0xF0, 0x00}
	statedb.SetCode(creator, code)

	e := vm.NewEVM(blockCtx(), vm.TxContext{Origin: creator}, statedb, params.AllEthashProtocolChanges, vm.Config{})

	gas := uint64(200_000)
	_, _, err := e.Call(vm.AccountRef(creator), creator, nil, gas, big.NewInt(0)) // (ret, left, err)
	require.NoError(t, err)                                                       // CALL succeeded; internal CREATE failed with 0 result
	require.Equal(t, uint64(math.MaxUint64), statedb.GetNonce(creator))
	require.Equal(t, code, statedb.GetCode(creator))
}

func TestCreate_Boundary_SucceedsAtMaxMinus1(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db), nil)

	caller := common.HexToAddress("0xB0UNDARY")
	statedb.AddBalance(caller, big.NewInt(1e18))
	statedb.SetNonce(caller, math.MaxUint64-1)

	e := vm.NewEVM(blockCtx(), vm.TxContext{Origin: caller}, statedb, params.AllEthashProtocolChanges, vm.Config{})

	gas := uint64(1_000_000)

	// Initcode that returns 1 byte of runtime code (0x00):
	// PUSH1 0x00 ; PUSH1 0x00 ; MSTORE ; PUSH1 0x01 ; PUSH1 0x00 ; RETURN
	initcode := []byte{0x60, 0x00, 0x60, 0x00, 0x52, 0x60, 0x01, 0x60, 0x00, 0xF3}

	ret, addr, left, err := e.Create(vm.AccountRef(caller), initcode, gas, big.NewInt(0))
	require.NoError(t, err)

	// Some gas must be consumed by initcode + code deposit
	require.Less(t, left, gas)

	// Nonce incremented to MaxUint64
	require.Equal(t, uint64(math.MaxUint64), statedb.GetNonce(caller))

	// Deployed runtime code length is exactly 1 byte
	require.Len(t, statedb.GetCode(addr), 1)

	// (Optional) ret is the runtime code returned by constructor; should also be 1 byte
	require.Len(t, ret, 1)
}
