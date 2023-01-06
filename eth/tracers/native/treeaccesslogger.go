package native

import (
	"encoding/json"
	"log"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	statedb "github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/tracers"
	"github.com/ethereum/go-ethereum/trie/utils"
	"github.com/holiman/uint256"
)

func init() {
	register("treeAccessLogger", NewTreeAccess)
}

var codeStorageDelta = uint256.NewInt(0).Sub(utils.CodeOffset, utils.HeaderStorageOffset)

type VKTBranchAccess int

const (
	VKTAccessWriteFirstTime VKTBranchAccess = iota
	VKTAccessWriteFree
	VKTAccessWriteHot
	VKTAccessReadFirstTime
	VKTAccessReadFree
	VKTAccessReadHot
)

type TreeAccess struct {
	Contract common.Address
	MPTDepth int

	VKTBranchAccess VKTBranchAccess
}

type TreeAccessLogger struct {
	env *vm.EVM

	accesses               []TreeAccess
	vktBranchReadAccesses  map[common.Address]map[string]struct{}
	vktBranchWriteAccesses map[common.Address]map[string]struct{}

	interrupt uint32 // Atomic flag to signal execution interruption
	reason    error  // Textual reason for the interruption
}

func NewTreeAccess(ctx *tracers.Context, config json.RawMessage) (tracers.Tracer, error) {
	ta := &TreeAccessLogger{
		vktBranchReadAccesses:  make(map[common.Address]map[string]struct{}),
		vktBranchWriteAccesses: make(map[common.Address]map[string]struct{}),
	}
	return ta, nil
}

// CaptureStart implements the EVMLogger interface to initialize the tracing operation.
func (l *TreeAccessLogger) CaptureStart(env *vm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
	l.env = env
}

// CaptureState logs a new structured log message and pushes it out to the environment
//
// CaptureState also tracks SLOAD/SSTORE ops to track storage change.
func (l *TreeAccessLogger) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
	// If tracing was interrupted, set the error and stop
	if atomic.LoadUint32(&l.interrupt) > 0 {
		return
	}

	// Ignore opcodes that don't access the tree.
	if !(op == vm.SLOAD || op == vm.SSTORE) {
		return
	}

	contract := scope.Contract
	stack := scope.Stack
	stackData := stack.Data()
	stackLen := len(stackData)

	var storageSlot common.Hash
	// capture SLOAD opcodes and record the read entry in the local storage
	if op == vm.SLOAD && stackLen >= 1 {
		storageSlot = common.Hash(stackData[stackLen-1].Bytes32())
	} else if op == vm.SSTORE && stackLen >= 2 {
		storageSlot = common.Hash(stackData[stackLen-1].Bytes32())
	} else {
		return
	}

	realStateDB := l.env.StateDB.(*statedb.StateDB)
	proof, err := realStateDB.GetStorageProof(contract.Address(), storageSlot)
	if err != nil {
		log.Printf("Error getting the proof: %s", err)
		return
	}

	var branchAccess VKTBranchAccess

	// VKT locality analysis
	if op == vm.SLOAD {
		// Optimized case where we get storage slots "for free"
		if storageSlot.Big().Cmp(codeStorageDelta.ToBig()) < 0 {
			branchAccess = VKTAccessReadFree
		} else {
			contractAddr := contract.Address()
			if _, ok := l.vktBranchReadAccesses[contractAddr]; !ok {
				l.vktBranchReadAccesses[contractAddr] = map[string]struct{}{}
			}
			vktKey, err := utils.GetTreeKeyStorageSlot(contractAddr.Bytes(), storageSlot.Big())
			if err != nil {
				log.Printf("Error getting VKT key: %s", err)
				return
			}
			stem := string(vktKey[:31])
			if _, ok := l.vktBranchReadAccesses[contractAddr][stem]; ok {
				branchAccess = VKTAccessReadHot
			} else {
				l.vktBranchReadAccesses[contractAddr][stem] = struct{}{}
				branchAccess = VKTAccessReadFirstTime
			}
		}
	}
	if op == vm.SSTORE {
		// Optimized case where we get storage slots "for free"
		if storageSlot.Big().Cmp(codeStorageDelta.ToBig()) < 0 {
			branchAccess = VKTAccessWriteFree
		} else {
			contractAddr := contract.Address()
			if _, ok := l.vktBranchWriteAccesses[contractAddr]; !ok {
				l.vktBranchWriteAccesses[contractAddr] = map[string]struct{}{}
			}
			vktKey, err := utils.GetTreeKeyStorageSlot(contractAddr.Bytes(), storageSlot.Big())
			if err != nil {
				log.Printf("Error getting VKT key: %s", err)
				return
			}
			stem := string(vktKey[:31])
			if _, ok := l.vktBranchWriteAccesses[contractAddr][stem]; ok {
				branchAccess = VKTAccessWriteHot
			} else {
				l.vktBranchWriteAccesses[contractAddr][stem] = struct{}{}
				branchAccess = VKTAccessWriteFirstTime
			}
		}
	}

	l.accesses = append(l.accesses, TreeAccess{
		Contract:        contract.Address(),
		MPTDepth:        len(proof),
		VKTBranchAccess: branchAccess,
	})
	// last := l.accesses[len(l.accesses)-1]
	// log.Printf("Contract %s:%s -> (MPT depth %d) (VKTBranchAccess is %d)", last.Contract, storageSlot, last.MPTDepth, last.VKTBranchAccess)
}

// CaptureFault implements the EVMLogger interface to trace an execution fault
// while running an opcode.
func (l *TreeAccessLogger) CaptureFault(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, depth int, err error) {
}

// CaptureEnd is called after the call finishes to finalize the tracing.
func (l *TreeAccessLogger) CaptureEnd(output []byte, gasUsed uint64, err error) {
}

func (l *TreeAccessLogger) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
}

func (l *TreeAccessLogger) CaptureExit(output []byte, gasUsed uint64, err error) {
}

func (l *TreeAccessLogger) CaptureTxStart(gasLimit uint64) {
}

func (l *TreeAccessLogger) CaptureTxEnd(restGas uint64) {
}

func (l *TreeAccessLogger) Stop(err error) {
	l.reason = err
	atomic.StoreUint32(&l.interrupt, 1)
}

func (l *TreeAccessLogger) GetResult() (json.RawMessage, error) {
	return json.Marshal(l.accesses)
}
