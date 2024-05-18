// Copyright 2014 The go-ethereum Authors
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

import "math"

const (
	set2BitsMask = uint64(0b11)
	set3BitsMask = uint64(0b111)
	set4BitsMask = uint64(0b1111)
	set5BitsMask = uint64(0b1_1111)
	set6BitsMask = uint64(0b11_1111)
	set7BitsMask = uint64(0b111_1111)
)

var masks [32 - 7]uint64

func init() {
	for i := 8; i <= 32; i++ {
		masks[i-8] = math.MaxUint64 >> (64 - i)
	}
}

// bitvec is a bit vector which maps bytes in a program.
// An unset bit means the byte is an opcode, a set bit means
// it's data (i.e. argument of PUSHxx).
type bitvec []uint64

func (bits bitvec) set1(pos uint64) {
	bits[pos/64] |= 1 << (pos % 64)
}

func (bits bitvec) setN(flag uint64, numbits uint64, pc uint64) {
	bitIdx := pc % 64
	bits[pc/64] |= flag << bitIdx
	if numbits+bitIdx > 64 {
		bits[pc/64+1] = flag >> (64 - bitIdx)
	}
}

// codeSegment checks if the position is in a code segment.
func (bv *bitvec) codeSegment(pos uint64) bool {
	return ((*bv)[pos/64])>>(pos%64)&1 == 0
}

// codeBitmap collects data locations in code.
func codeBitmap(code []byte) bitvec {
	// The bitmap is 8 bytes longer than necessary, in case the code
	// ends with a PUSH32, the algorithm will set bits on the
	// bitvector outside the bounds of the actual code.
	bits := make([]uint64, len(code)/64+1+1)
	return codeBitmapInternal(code, bits)
}

// codeBitmapInternal is the internal implementation of codeBitmap.
// It exists for the purpose of being able to run benchmark tests
// without dynamic allocations affecting the results.
func codeBitmapInternal(code []byte, bits bitvec) bitvec {
	for pc := uint64(0); pc < uint64(len(code)); {
		op := OpCode(code[pc])
		pc++
		if int8(op) < int8(PUSH1) { // If not PUSH (the int8(op) > int(PUSH32) is always false).
			continue
		}
		numbits := uint64(op - PUSH1 + 1)

		switch numbits {
		case 1:
			bits.set1(pc)
			pc += 1
		case 2:
			bits.setN(set2BitsMask, 2, pc)
			pc += 2
		case 3:
			bits.setN(set3BitsMask, 3, pc)
			pc += 3
		case 4:
			bits.setN(set4BitsMask, 4, pc)
			pc += 4
		case 5:
			bits.setN(set5BitsMask, 5, pc)
			pc += 5
		case 6:
			bits.setN(set6BitsMask, 6, pc)
			pc += 6
		case 7:
			bits.setN(set7BitsMask, 7, pc)
			pc += 7
		default:
			bits.setN(masks[numbits-8], numbits, pc)
			pc += numbits
		}
	}
	return bits
}
