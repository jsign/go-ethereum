package types

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

type ContractCode interface {
	GetAtPos(pos uint64) (byte, error)
	GetAtRange(start, end uint64) ([]byte, error)
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

func (r *SingleContractCode) GetSize() uint64 {
	return uint64(len(r.fullCode))
}
