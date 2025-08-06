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

// Minimal block context with transfer hooks
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
		Time:        big.NewInt(1),
	}
}

func TestCreate_TopLevel_MaxNonce(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db), nil)

	caller := common.HexToAddress("0xCA11ER")
	statedb.AddBalance(caller, big.NewInt(1e18))
	statedb.SetNonce(caller, math.MaxUint64)

	evm := vm.NewEVM(blockCtx(), vm.TxContext{Origin: caller}, statedb, params.AllEthashProtocolChanges, vm.Config{})

	gas := uint64(1_000_000)
	ret, addr, left, err := evm.Create(vm.AccountRef(caller), []byte{0x00}, gas, big.NewInt(0))
	require.Error(t, err)
	require.Equal(t, vm.ErrNonceMax, err) // explicit EIP-2681 error
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

	evm := vm.NewEVM(blockCtx(), vm.TxContext{Origin: creator}, statedb, params.AllEthashProtocolChanges, vm.Config{})

	gas := uint64(200_000)
	// Call into the creator's code; internal CREATE should fail and not run initcode.
	_, _, err := evm.Call(vm.AccountRef(creator), creator, nil, gas, big.NewInt(0))
	require.NoError(t, err)                                             // external CALL ok; internal CREATE failed
	require.Equal(t, uint64(math.MaxUint64), statedb.GetNonce(creator)) // unchanged
	require.Equal(t, code, statedb.GetCode(creator))                    // no new code created
}

func TestCreate_Boundary_SucceedsAtMaxMinus1(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db), nil)

	caller := common.HexToAddress("0xB0UNDARY")
	statedb.AddBalance(caller, big.NewInt(1e18))
	statedb.SetNonce(caller, math.MaxUint64-1)

	evm := vm.NewEVM(blockCtx(), vm.TxContext{Origin: caller}, statedb, params.AllEthashProtocolChanges, vm.Config{})

	gas := uint64(1_000_000)
	ret, addr, left, err := evm.Create(vm.AccountRef(caller), []byte{0x00}, gas, big.NewInt(0))
	require.NoError(t, err)
	require.NotNil(t, ret)                                             // creation ran
	require.Less(t, left, gas)                                         // some gas consumed
	require.Equal(t, uint64(math.MaxUint64), statedb.GetNonce(caller)) // incremented to max
	_ = addr
}
