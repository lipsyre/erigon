// Copyright 2024 The Erigon Authors
// This file is part of Erigon.
//
// Erigon is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Erigon is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Erigon. If not, see <http://www.gnu.org/licenses/>.

package calltracer

import (
	"encoding/binary"
	"sort"

	"github.com/holiman/uint256"

	libcommon "github.com/erigontech/erigon-lib/common"
	"github.com/erigontech/erigon-lib/common/length"
	"github.com/erigontech/erigon-lib/kv"

	"github.com/erigontech/erigon/core/types"
	"github.com/erigontech/erigon/core/vm"
)

type CallTracer struct {
	froms map[libcommon.Address]struct{}
	tos   map[libcommon.Address]struct{}
}

func NewCallTracer() *CallTracer {
	return &CallTracer{
		froms: make(map[libcommon.Address]struct{}),
		tos:   make(map[libcommon.Address]struct{}),
	}
}

func (ct *CallTracer) CaptureTxStart(gasLimit uint64) {}
func (ct *CallTracer) CaptureTxEnd(restGas uint64)    {}

// CaptureStart and CaptureEnter also capture SELFDESTRUCT opcode invocations
func (ct *CallTracer) captureStartOrEnter(from, to libcommon.Address) {
	ct.froms[from] = struct{}{}
	ct.froms[to] = struct{}{}
}

func (ct *CallTracer) CaptureStart(env *vm.EVM, from libcommon.Address, to libcommon.Address, precompile bool, create bool, input []byte, gas uint64, value *uint256.Int, code []byte) {
	ct.captureStartOrEnter(from, to)
}
func (ct *CallTracer) CaptureEnter(typ vm.OpCode, from libcommon.Address, to libcommon.Address, precompile bool, create bool, input []byte, gas uint64, value *uint256.Int, code []byte) {
	ct.captureStartOrEnter(from, to)
}
func (ct *CallTracer) CaptureState(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, rData []byte, depth int, err error) {
}
func (ct *CallTracer) CaptureFault(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, depth int, err error) {
}
func (ct *CallTracer) CaptureEnd(output []byte, usedGas uint64, err error) {
}
func (ct *CallTracer) CaptureExit(output []byte, usedGas uint64, err error) {
}

func (ct *CallTracer) WriteToDb(tx kv.Putter, block *types.Block, vmConfig vm.Config) error {
	ct.tos[block.Coinbase()] = struct{}{}
	for _, uncle := range block.Uncles() {
		ct.tos[uncle.Coinbase] = struct{}{}
	}
	list := make(libcommon.Addresses, len(ct.froms)+len(ct.tos))
	i := 0
	for addr := range ct.froms {
		copy(list[i][:], addr[:])
		i++
	}
	for addr := range ct.tos {
		copy(list[i][:], addr[:])
		i++
	}
	sort.Sort(list)
	// List may contain duplicates
	var blockNumEnc [8]byte
	binary.BigEndian.PutUint64(blockNumEnc[:], block.Number().Uint64())
	var prev libcommon.Address
	for j, addr := range list {
		if j > 0 && prev == addr {
			continue
		}
		var v [length.Addr + 1]byte
		copy(v[:], addr[:])
		if _, ok := ct.froms[addr]; ok {
			v[length.Addr] |= 1
		}
		if _, ok := ct.tos[addr]; ok {
			v[length.Addr] |= 2
		}
		if j == 0 {
			if err := tx.Append(kv.CallTraceSet, blockNumEnc[:], v[:]); err != nil {
				return err
			}
		} else {
			if err := tx.AppendDup(kv.CallTraceSet, blockNumEnc[:], v[:]); err != nil {
				return err
			}
		}
		copy(prev[:], addr[:])
	}
	return nil
}
