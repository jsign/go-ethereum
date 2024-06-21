package live

import (
	"encoding/gob"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/eth/tracers"
	"github.com/ethereum/go-ethereum/log"
)

// For now the path is fixed. We can make it configurable later.
var pctracesPath = "/data/pctraces_live"

func init() {
	if err := os.MkdirAll(filepath.Join(pctracesPath, "code"), 0755); err != nil {
		panic(err)
	}
	tracers.LiveDirectory.Register("pctrace", newPCTracer)
}

type pcTracer struct {
	savedBytecodes map[common.Address]struct{}

	vm               *tracing.VMContext
	skip             bool
	currentTx        common.Hash
	pendingBytecodes map[common.Address]struct{}
	output           output
}
type output struct {
	ContractsPCs map[common.Address][]uint64
	ReceiptGas   uint64
	To           common.Address
}

func newPCTracer(_ json.RawMessage) (*tracing.Hooks, error) {
	t := &pcTracer{
		savedBytecodes:   map[common.Address]struct{}{},
		pendingBytecodes: map[common.Address]struct{}{},
		output:           output{ContractsPCs: map[common.Address][]uint64{}},
	}
	return &tracing.Hooks{
		OnTxStart: t.OnTxStart,
		OnTxEnd:   t.OnTxEnd,
		OnOpcode:  t.OnOpcode,
	}, nil
}

func (t *pcTracer) OnOpcode(pc uint64, op byte, gas, cost uint64, scope tracing.OpContext, rData []byte, depth int, err error) {
	if _, ok := t.savedBytecodes[scope.Address()]; !ok {
		t.pendingBytecodes[scope.Address()] = struct{}{}
	}
	endPC := pc
	if op >= byte(vm.PUSH1) && op <= byte(vm.PUSH32) {
		endPC += uint64(op - byte(vm.PUSH1) + 1)
	}
	for i := pc; i <= endPC; i++ {
		t.output.ContractsPCs[scope.Address()] = append(t.output.ContractsPCs[scope.Address()], i)
	}
}

func (t *pcTracer) OnTxStart(vm *tracing.VMContext, tx *types.Transaction, from common.Address) {
	t.skip = tx.To() == nil
	if t.skip {
		return
	}

	t.currentTx = tx.Hash()
	t.vm = vm
	clear(t.pendingBytecodes)

	t.output.ReceiptGas = 0
	t.output.To = *tx.To()
	clear(t.output.ContractsPCs)
}

func (t *pcTracer) OnTxEnd(receipt *types.Receipt, err error) {
	if t.skip || err != nil || len(t.output.ContractsPCs) == 0 {
		return
	}
	for addr := range t.pendingBytecodes {
		bytecode := t.vm.StateDB.GetCode(addr)
		if len(bytecode) == 0 {
			delete(t.output.ContractsPCs, addr)
			continue
		}
		if err := os.WriteFile(filepath.Join(pctracesPath, "code", addr.String()), bytecode, 0644); err != nil {
			log.Warn("failed to write bytecode", "addr", addr.String(), "err", err)
			return
		}
		t.savedBytecodes[addr] = struct{}{}
	}

	f, err := os.Create(filepath.Join(pctracesPath, t.currentTx.String()))
	if err != nil {
		log.Warn("failed to create file", "file", t.currentTx, "err", err)
		return
	}
	t.output.ReceiptGas = receipt.GasUsed
	if err := gob.NewEncoder(f).Encode(t.output); err != nil {
		log.Warn("failed to write pctraces", "txHash", t.currentTx, "err", err)
		return
	}
}
