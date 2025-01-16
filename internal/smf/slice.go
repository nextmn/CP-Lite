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

func NewSlicesMap(slices map[string]config.Slice, areas map[string]config.Area) *SlicesMap {
	m := SlicesMap{}
	for k, slice := range slices {
		upfs := make([]netip.Addr, len(slice.Upfs))
		for i, upf := range slice.Upfs {
			upfs[i] = upf.NodeID
		}

		paths := make(map[string][]netip.Addr)
		for area_name, area := range areas {
			if path, exists := area.Paths[k]; exists {
				paths[area_name] = path
			}
		}

		sl := NewSlice(slice.Pool, upfs, paths)
		m.Store(k, sl)
	}
	return &m
}

type Slice struct {
	Upfs     []netip.Addr
	Pool     *UeIpPool
	sessions *SessionsMap
	Paths    map[string][]netip.Addr
}

func NewSlice(pool netip.Prefix, upfs []netip.Addr, paths map[string][]netip.Addr) *Slice {
	return &Slice{
		Pool:     NewUeIpPool(pool),
		Upfs:     upfs,
		sessions: NewSessionsMap(),
		Paths:    paths,
	}
}
