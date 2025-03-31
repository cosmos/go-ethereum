// Copyright 2019 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package vm

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

var activators = map[string]func(*JumpTable){
	"ethereum_5656": enable5656,
	"ethereum_6780": enable6780,
	"ethereum_3855": enable3855,
	"ethereum_3860": enable3860,
	"ethereum_3529": enable3529,
	"ethereum_3198": enable3198,
	"ethereum_2929": enable2929,
	"ethereum_2200": enable2200,
	"ethereum_1884": enable1884,
	"ethereum_1344": enable1344,
	"ethereum_1153": enable1153,
}

// EnableEIP enables the given EIP on the config.
// This operation writes in-place, and callers need to ensure that the globally
// defined jump tables are not polluted.
func EnableEIP(eipName string, jt *JumpTable) error {
	enablerFn, ok := activators[eipName]
	if !ok {
		return fmt.Errorf("undefined eip %s", eipName)
	}
	enablerFn(jt)
	return nil
}

// ValidateEIPName checks if an EIP name is valid or not. The allowed
// name structure is a string that can be represented as "chainName" + "_" + "int".
func ValidateEIPName(eipName string) error {
	eipSplit := strings.Split(eipName, "_")
	if len(eipSplit) != 2 {
		return fmt.Errorf("eip name does not conform to structure 'chainName_Number'")
	}
	if _, err := strconv.Atoi(eipSplit[1]); err != nil {
		return fmt.Errorf("eip number should be convertible to int")
	}
	return nil
}

// ExistsEipActivator return true if the given EIP
// name is associated with an activator function.
// Return false otherwise.
func ExistsEipActivator(eipName string) bool {
	_, ok := activators[eipName]
	return ok
}

// ActivateableEips returns the sorted slice of EIP names
// that can be activated.
func ActivateableEips() []string {
	var names []string
	if len(activators) > 0 {
		for k := range activators {
			names = append(names, k)
		}
		sort.Strings(names)
	}
	return names
}

// enable1884 applies EIP-1884 to the given jump table:
// - Increase cost of BALANCE to 700
// - Increase cost of EXTCODEHASH to 700
// - Increase cost of SLOAD to 800
// - Define SELFBALANCE, with cost GasFastStep (5)
func enable1884(jt *JumpTable) {
	// Gas cost changes
	jt[SLOAD].constantGas = params.SloadGasEIP1884
	jt[BALANCE].constantGas = params.BalanceGasEIP1884
	jt[EXTCODEHASH].constantGas = params.ExtcodeHashGasEIP1884

	// New opcode
	jt[SELFBALANCE] = &operation{
		execute:     opSelfBalance,
		constantGas: GasFastStep,
		minStack:    minStack(0, 1),
		maxStack:    maxStack(0, 1),
	}
}

func opSelfBalance(pc *uint64, interpreter *EVMInterpreter, scope *ScopeContext) ([]byte, error) {
	balance := interpreter.evm.StateDB.GetBalance(scope.Contract.Address())
	scope.Stack.Push(balance)
	return nil, nil
}

// enable1344 applies EIP-1344 (ChainID Opcode)
// - Adds an opcode that returns the current chain’s EIP-155 unique identifier
func enable1344(jt *JumpTable) {
	// New opcode
	jt[CHAINID] = &operation{
		execute:     opChainID,
		constantGas: GasQuickStep,
		minStack:    minStack(0, 1),
		maxStack:    maxStack(0, 1),
	}
}

// opChainID implements CHAINID opcode
func opChainID(pc *uint64, interpreter *EVMInterpreter, scope *ScopeContext) ([]byte, error) {
	chainId, _ := uint256.FromBig(interpreter.evm.chainConfig.ChainID)
	scope.Stack.Push(chainId)
	return nil, nil
}

// enable2200 applies EIP-2200 (Rebalance net-metered SSTORE)
func enable2200(jt *JumpTable) {
	jt[SLOAD].constantGas = params.SloadGasEIP2200
	jt[SSTORE].dynamicGas = gasSStoreEIP2200
}

// enable2929 enables "EIP-2929: Gas cost increases for state access opcodes"
// https://eips.ethereum.org/EIPS/eip-2929
func enable2929(jt *JumpTable) {
	jt[SSTORE].dynamicGas = gasSStoreEIP2929

	jt[SLOAD].constantGas = 0
	jt[SLOAD].dynamicGas = gasSLoadEIP2929

	jt[EXTCODECOPY].constantGas = params.WarmStorageReadCostEIP2929
	jt[EXTCODECOPY].dynamicGas = gasExtCodeCopyEIP2929

	jt[EXTCODESIZE].constantGas = params.WarmStorageReadCostEIP2929
	jt[EXTCODESIZE].dynamicGas = gasEip2929AccountCheck

	jt[EXTCODEHASH].constantGas = params.WarmStorageReadCostEIP2929
	jt[EXTCODEHASH].dynamicGas = gasEip2929AccountCheck

	jt[BALANCE].constantGas = params.WarmStorageReadCostEIP2929
	jt[BALANCE].dynamicGas = gasEip2929AccountCheck

	jt[CALL].constantGas = params.WarmStorageReadCostEIP2929
	jt[CALL].dynamicGas = gasCallEIP2929

	jt[CALLCODE].constantGas = params.WarmStorageReadCostEIP2929
	jt[CALLCODE].dynamicGas = gasCallCodeEIP2929

	jt[STATICCALL].constantGas = params.WarmStorageReadCostEIP2929
	jt[STATICCALL].dynamicGas = gasStaticCallEIP2929

	jt[DELEGATECALL].constantGas = params.WarmStorageReadCostEIP2929
	jt[DELEGATECALL].dynamicGas = gasDelegateCallEIP2929

	// This was previously part of the dynamic cost, but we're using it as a constantGas
	// factor here
	jt[SELFDESTRUCT].constantGas = params.SelfdestructGasEIP150
	jt[SELFDESTRUCT].dynamicGas = gasSelfdestructEIP2929
}

// enable3529 enabled "EIP-3529: Reduction in refunds":
// - Removes refunds for selfdestructs
// - Reduces refunds for SSTORE
// - Reduces max refunds to 20% gas
func enable3529(jt *JumpTable) {
	jt[SSTORE].dynamicGas = gasSStoreEIP3529
	jt[SELFDESTRUCT].dynamicGas = gasSelfdestructEIP3529
}

// enable3198 applies EIP-3198 (BASEFEE Opcode)
// - Adds an opcode that returns the current block's base fee.
func enable3198(jt *JumpTable) {
	// New opcode
	jt[BASEFEE] = &operation{
		execute:     opBaseFee,
		constantGas: GasQuickStep,
		minStack:    minStack(0, 1),
		maxStack:    maxStack(0, 1),
	}
}

// enable1153 applies EIP-1153 "Transient Storage"
// - Adds TLOAD that reads from transient storage
// - Adds TSTORE that writes to transient storage
func enable1153(jt *JumpTable) {
	jt[TLOAD] = &operation{
		execute:     opTload,
		constantGas: params.WarmStorageReadCostEIP2929,
		minStack:    minStack(1, 1),
		maxStack:    maxStack(1, 1),
	}

	jt[TSTORE] = &operation{
		execute:     opTstore,
		constantGas: params.WarmStorageReadCostEIP2929,
		minStack:    minStack(2, 0),
		maxStack:    maxStack(2, 0),
	}
}

// opTload implements TLOAD opcode
func opTload(pc *uint64, interpreter *EVMInterpreter, scope *ScopeContext) ([]byte, error) {
	loc := scope.Stack.Peek()
	hash := common.Hash(loc.Bytes32())
	val := interpreter.evm.StateDB.GetTransientState(scope.Contract.Address(), hash)
	loc.SetBytes(val.Bytes())
	return nil, nil
}

// opTstore implements TSTORE opcode
func opTstore(pc *uint64, interpreter *EVMInterpreter, scope *ScopeContext) ([]byte, error) {
	if interpreter.readOnly {
		return nil, ErrWriteProtection
	}
	loc := scope.Stack.Pop()
	val := scope.Stack.Pop()
	interpreter.evm.StateDB.SetTransientState(scope.Contract.Address(), loc.Bytes32(), val.Bytes32())
	return nil, nil
}

// opBaseFee implements BASEFEE opcode
func opBaseFee(pc *uint64, interpreter *EVMInterpreter, scope *ScopeContext) ([]byte, error) {
	baseFee, _ := uint256.FromBig(interpreter.evm.Context.BaseFee)
	scope.Stack.Push(baseFee)
	return nil, nil
}

// enable3855 applies EIP-3855 (PUSH0 opcode)
func enable3855(jt *JumpTable) {
	// New opcode
	jt[PUSH0] = &operation{
		execute:     opPush0,
		constantGas: GasQuickStep,
		minStack:    minStack(0, 1),
		maxStack:    maxStack(0, 1),
	}
}

// opPush0 implements the PUSH0 opcode
func opPush0(pc *uint64, interpreter *EVMInterpreter, scope *ScopeContext) ([]byte, error) {
	scope.Stack.Push(new(uint256.Int))
	return nil, nil
}

// enable3860 enables "EIP-3860: Limit and meter initcode"
// https://eips.ethereum.org/EIPS/eip-3860
func enable3860(jt *JumpTable) {
	jt[CREATE].dynamicGas = gasCreateEip3860
	jt[CREATE2].dynamicGas = gasCreate2Eip3860
}

// enable5656 enables EIP-5656 (MCOPY opcode)
// https://eips.ethereum.org/EIPS/eip-5656
func enable5656(jt *JumpTable) {
	jt[MCOPY] = &operation{
		execute:     opMcopy,
		constantGas: GasFastestStep,
		dynamicGas:  gasMcopy,
		minStack:    minStack(3, 0),
		maxStack:    maxStack(3, 0),
		memorySize:  memoryMcopy,
	}
}

// opMcopy implements the MCOPY opcode (https://eips.ethereum.org/EIPS/eip-5656)
func opMcopy(pc *uint64, interpreter *EVMInterpreter, scope *ScopeContext) ([]byte, error) {
	var (
		dst    = scope.Stack.Pop()
		src    = scope.Stack.Pop()
		length = scope.Stack.Pop()
	)
	// These values are checked for overflow during memory expansion calculation
	// (the memorySize function on the opcode).
	scope.Memory.Copy(dst.Uint64(), src.Uint64(), length.Uint64())
	return nil, nil
}

// opBlobHash implements the BLOBHASH opcode
func opBlobHash(pc *uint64, interpreter *EVMInterpreter, scope *ScopeContext) ([]byte, error) {
	index := scope.Stack.Peek()
	if index.LtUint64(uint64(len(interpreter.evm.TxContext.BlobHashes))) {
		blobHash := interpreter.evm.TxContext.BlobHashes[index.Uint64()]
		index.SetBytes32(blobHash[:])
	} else {
		index.Clear()
	}
	return nil, nil
}

// opBlobBaseFee implements BLOBBASEFEE opcode
func opBlobBaseFee(pc *uint64, interpreter *EVMInterpreter, scope *ScopeContext) ([]byte, error) {
	blobBaseFee, _ := uint256.FromBig(interpreter.evm.Context.BlobBaseFee)
	scope.Stack.Push(blobBaseFee)
	return nil, nil
}

// enable4844 applies EIP-4844 (BLOBHASH opcode)
func enable4844(jt *JumpTable) {
	jt[BLOBHASH] = &operation{
		execute:     opBlobHash,
		constantGas: GasFastestStep,
		minStack:    minStack(1, 1),
		maxStack:    maxStack(1, 1),
	}
}

// enable7516 applies EIP-7516 (BLOBBASEFEE opcode)
func enable7516(jt *JumpTable) {
	jt[BLOBBASEFEE] = &operation{
		execute:     opBlobBaseFee,
		constantGas: GasQuickStep,
		minStack:    minStack(0, 1),
		maxStack:    maxStack(0, 1),
	}
}

// enable6780 applies EIP-6780 (deactivate SELFDESTRUCT)
func enable6780(jt *JumpTable) {
	jt[SELFDESTRUCT] = &operation{
		execute:     opSelfdestruct6780,
		dynamicGas:  gasSelfdestructEIP3529,
		constantGas: params.SelfdestructGasEIP150,
		minStack:    minStack(1, 0),
		maxStack:    maxStack(1, 0),
	}
}
