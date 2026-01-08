// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"net/netip"
)

type UeIpPool struct {
	pool    netip.Prefix
	current netip.Addr
}

func NewUeIpPool(pool netip.Prefix) *UeIpPool {
	return &UeIpPool{
		pool:    pool,
		current: pool.Addr(),
	}
}

func (p *UeIpPool) Next() (netip.Addr, error) {
	addr := p.current.Next()
	p.current = addr
	if !p.pool.Contains(addr) {
		return addr, ErrNoIpAvailableInPool
	}
	return addr, nil
}
