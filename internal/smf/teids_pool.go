// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"context"
	"math/rand"
	"sync"
)

type TEIDsPool struct {
	teids map[uint32]struct{}
	sync.Mutex
}

func NewTEIDsPool() *TEIDsPool {
	return &TEIDsPool{
		teids: make(map[uint32]struct{}),
	}
}

func (t *TEIDsPool) Next(ctx context.Context) (uint32, error) {
	t.Lock()
	defer t.Unlock()
	var teid uint32 = 0
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
			teid = rand.Uint32()
			if teid == 0 {
				continue
			}
			if _, ok := t.teids[teid]; !ok {
				t.teids[teid] = struct{}{}
				return teid, nil
			}
		}
	}
}

func (t *TEIDsPool) Delete(teid uint32) {
	t.Lock()
	defer t.Unlock()
	delete(t.teids, teid)
}
