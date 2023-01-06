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
)

func init() {
	register("prestateTracer", NewTreeAccess)
}

type TreeAccess struct {
	Contract    common.Address
	Opcode      vm.OpCode
	StorageSlot common.Hash
	MPTDepth    int
}

type TreeAccessLogger struct {
	env *vm.EVM

	accesses []TreeAccess
	output   []byte
	err      error

	interrupt uint32 // Atomic flag to signal execution interruption
	reason    error  // Textual reason for the interruption
}

func NewTreeAccess(ctx *tracers.Context, config json.RawMessage) (tracers.Tracer, error) {
	ta := &TreeAccessLogger{}
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
	}
	log.Printf("Contract %s, %s, storage slot %s has depth %d", contract.Address(), op.String(), storageSlot, len(proof))

	l.accesses = append(l.accesses, TreeAccess{
		Contract:    contract.Address(),
		Opcode:      op,
		StorageSlot: storageSlot,
		MPTDepth:    len(proof),
	})
}

// CaptureFault implements the EVMLogger interface to trace an execution fault
// while running an opcode.
func (l *TreeAccessLogger) CaptureFault(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, depth int, err error) {
}

// CaptureEnd is called after the call finishes to finalize the tracing.
func (l *TreeAccessLogger) CaptureEnd(output []byte, gasUsed uint64, err error) {
	l.output = output
	l.err = err
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
