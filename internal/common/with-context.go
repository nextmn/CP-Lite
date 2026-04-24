// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
)

// WithContext is used to attach a [context.Context] to a long-lived entyt.
// This entity will then use this Context for all operations (serving requests, etc.).
type WithContext struct {
	// not exported because must not be modified
	ctx context.Context
}

// InitContext initializes the [context.Context].
// The provided Context must be non-nil, else it will panic.
func (wc *WithContext) InitContext(ctx context.Context) {
	if ctx == nil {
		panic("nil context")
	}
	wc.ctx = ctx
}

// Context returns the attached [context.Context],
// or the Background Context if no Context has been attached yet.
func (wc *WithContext) Context() context.Context {
	if wc.ctx != nil {
		return wc.ctx
	}
	return context.Background()
}
