package types

import (
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/trie/utils"
	"github.com/gballet/go-verkle"
	"github.com/holiman/uint256"
)

type ContractCode interface {
	GetAtPos(pos uint64) (byte, error)
	GetAtRange(start, end uint64) ([]byte, error)
	GetAll() ([]byte, error)
	GetSize() uint64
}

type ContractCodeResolver interface {
	Get(addr common.Address) ContractCode
}

type SingleContractCodeResolver struct {
	ssc *SingleContractCode
}

var _ = ContractCodeResolver(&SingleContractCodeResolver{})

func NewSingleContractCodeResolver(fullCode []byte, address common.Address, allowedRanges [][2]uint64) ContractCodeResolver {
	return &SingleContractCodeResolver{
		ssc: &SingleContractCode{
			addr:          address,
			fullCode:      fullCode,
			allowedRanges: allowedRanges,
		},
	}
}

func (r *SingleContractCodeResolver) Get(addr common.Address) ContractCode {
	return r.ssc
}

type SingleContractCode struct {
	addr          common.Address
	fullCode      []byte
	allowedRanges [][2]uint64
}

var _ = ContractCode(&SingleContractCode{})

func (r *SingleContractCode) GetAtPos(pos uint64) (byte, error) {
	code, err := r.GetAtRange(pos, pos)
	if err != nil {
		return 0, err
	}
	return code[0], nil
}

func (r *SingleContractCode) GetAtRange(start uint64, end uint64) ([]byte, error) {
	if start > end {
		return nil, fmt.Errorf("start can't be bigger than end")
	}
	for _, allowedRange := range r.allowedRanges {
		if start >= allowedRange[0] || end <= allowedRange[1] {
			return r.fullCode[start : end+1], nil
		}
	}
	return nil, fmt.Errorf("invalid asked range [%d, %d] isn't part of any allowed range", start, end)
}

// GetAll only makes sense in transactions that are contract creations; it will probably fail
// in other cases. It'd be a better idea to make a new interface to handle this case more elegantly and avoid
// corner cases.
func (r *SingleContractCode) GetAll() ([]byte, error) {
	// TODO(jsign): commenting the code below for now; it's correct but until we have a better way to
	// avoid asking for the "full code" to do gas accounting on normal txn executions we need to allow this.
	// if len(r.allowedRanges) != 1 {
	// 	return nil, fmt.Errorf("invalid number of allowed ranges")
	// }
	// if r.allowedRanges[0][0] != 0 || r.allowedRanges[0][1] != uint64(len(r.fullCode)-1) {
	// 	return nil, fmt.Errorf("invalid allowed range")
	// }
	return r.fullCode, nil
}

func (r *SingleContractCode) GetSize() uint64 {
	return uint64(len(r.fullCode))
}

type TreeContractCodeResolver struct {
	tree verkle.VerkleNode
}

// Get implements ContractCodeResolver.
func (t *TreeContractCodeResolver) Get(addr common.Address) ContractCode {
	return &TreeContractCode{
		addrPoint: utils.EvaluateAddressPoint(addr[:]),
		tree:      t.tree,
	}
}

func NewTreeContractResolver(tree verkle.VerkleNode) ContractCodeResolver {
	return &TreeContractCodeResolver{
		tree: tree,
	}
}

//GetTreeKeyCodeChunkWithEvaluatedAddress

type TreeContractCode struct {
	addrPoint *verkle.Point
	tree      verkle.VerkleNode
	// TODO(jsign): we can cache chunks.
}

// GetAll implements ContractCode.
func (t *TreeContractCode) GetAll() ([]byte, error) {
	// TODO(jsign): we need to think on removing this API, it doesn't make sense.
	// For now is needed since we require getting all code for gas accounting.
	return nil, nil
}

// GetAtPos implements ContractCode.
func (t *TreeContractCode) GetAtPos(pos uint64) (byte, error) {
	chunkBytes, err := t.getChunk(pos)
	if err != nil {
		return 0, fmt.Errorf("get chunk: %s", err)
	}
	return chunkBytes[pos%31+1], nil
}

// GetAtRange implements ContractCode.
func (t *TreeContractCode) GetAtRange(start uint64, end uint64) ([]byte, error) {
	firstChunk := start / 31
	lastChunk := end / 31
	alignedBytecodeBytes := make([]byte, (lastChunk-firstChunk+1)*31)
	for i := firstChunk; i <= lastChunk; i++ {
		chunkBytes, err := t.getChunk(i)
		if err != nil {
			return nil, fmt.Errorf("get chunk: %s", err)
		}
		copy(alignedBytecodeBytes[(i-firstChunk)*31:], chunkBytes[1:])
	}

	byteCodeStart := start % 31
	byteCodeEnd := len(alignedBytecodeBytes) - (31 - int(end%31)) + 1
	return alignedBytecodeBytes[byteCodeStart:byteCodeEnd], nil
}

func (t *TreeContractCode) getChunk(pos uint64) ([]byte, error) {
	chunkNum := uint256.NewInt(pos / 31)
	chunkAddr := utils.GetTreeKeyCodeChunkWithEvaluatedAddress(t.addrPoint, chunkNum)
	chunkBytes, err := t.tree.Get(chunkAddr, nil)
	if err != nil {
		return nil, fmt.Errorf("get chunk bytes from tree: %s", err)
	}
	return chunkBytes, nil
}

var zero = uint256.NewInt(0)

// GetSize implements ContractCode.
func (t *TreeContractCode) GetSize() uint64 {
	codeSizeAddr := utils.GetTreeKeyWithEvaluatedAddess(t.addrPoint, zero, utils.CodeSizeLeafKey)
	codeSizeBytes, err := t.tree.Get(codeSizeAddr, nil)
	if err != nil {
		// TODO(jsign): change API signature.
		panic(err)
	}
	return binary.BigEndian.Uint64(codeSizeBytes)
}
