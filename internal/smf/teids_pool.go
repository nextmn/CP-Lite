// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"context"
	"math/rand"
)

type TEID uint32 // TODO: replace with `type TEID = jsonapi.TEID`

const maxTEIDs = 0xFFFF_FFFF - 1 // number of TEIDs initially available in the pool

// TEIDsPool allows to generate and maintain TEIDs.
type TEIDsPool struct {
	teids map[TEID]struct{} // holds TEIDs currently in use

	ch chan struct{} // context aware mutex
}

// NewTEIDsPool creates a TEIDsPool.
func NewTEIDsPool() *TEIDsPool {
	pool := TEIDsPool{
		teids: make(map[TEID]struct{}),
		ch:    make(chan struct{}, 1),
	}
	pool.ch <- struct{}{} // unlock the pool
	return &pool
}

// Next provides a new random, but unused, TEID from the pool.
// It may hang if there is no more available TEID within the pool (until a TEID is released, or the context is done).
// TEIDs are generated at random, so it will take more time if a lot of TEIDs are already used.
// Current number of TEIDs Available/In use can be checked with `TEIDsAvailable()` / `TEIDsInUse()` and the SMF should consider using
// another UPF if number of TEIDsAvailable is too low.
func (t *TEIDsPool) Next(ctx context.Context) (TEID, error) {
	if ctx == nil {
		panic("nil context")
	}
	teid := TEID(0)
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
			teid = TEID(rand.Uint32())
			if teid == 0 {
				continue
			}
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-t.ch:
				// pool is locked
				if _, ok := t.teids[teid]; !ok {
					t.teids[teid] = struct{}{}
					t.ch <- struct{}{} // unlock the pool
					return teid, nil
				}
				t.ch <- struct{}{} // unlock the pool
			}
		}
	}
}

func (t *TEIDsPool) TEIDsAvailable() int {
	return maxTEIDs - len(t.teids)
}

func (t *TEIDsPool) TEIDsInUse() int {
	return len(t.teids)
}

// Release releases a TEID into the pool of available TEIDs.
func (t *TEIDsPool) Release(ctx context.Context, teid TEID) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.ch:
		// pool is locked
		delete(t.teids, teid)
		t.ch <- struct{}{} // unlock the pool
		return nil
	}
}
