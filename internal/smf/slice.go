// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"net/netip"
	"sync"

	"github.com/nextmn/cp-lite/internal/config"
)

type SlicesMap struct {
	sync.Map // slice name: Slice
}

func NewSlicesMap(slices map[string]config.Slice) *SlicesMap {
	m := SlicesMap{}
	for k, slice := range slices {
		upfs := make([]netip.Addr, len(slice.Upfs))
		for i, upf := range slice.Upfs {
			upfs[i] = upf.NodeID
		}
		sl := NewSlice(slice.Pool, upfs)
		m.Store(k, sl)
	}
	return &m
}

type Slice struct {
	Upfs     []netip.Addr
	Pool     *UeIpPool
	sessions *SessionsMap
}

func NewSlice(pool netip.Prefix, upfs []netip.Addr) *Slice {
	return &Slice{
		Pool:     NewUeIpPool(pool),
		Upfs:     upfs,
		sessions: NewSessionsMap(),
	}
}
