// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
)

type WithContext struct {
	// not exported because must not be modified
	ctx context.Context
}

func (wc *WithContext) InitContext(ctx context.Context) error {
	if ctx == nil {
		return ErrNilCtx
	}
	wc.ctx = ctx
	return nil
}

func (wc *WithContext) Context() context.Context {
	if wc.ctx != nil {
		return wc.ctx
	}
	return context.Background()
}
