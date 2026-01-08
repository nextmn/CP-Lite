// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"errors"
)

var (
	ErrDnnNotFound        = errors.New("DNN not found")
	ErrPDUSessionNotFound = errors.New("PDU Session not found")
	ErrAreaNotFound       = errors.New("RAN Area not found for this gNB")
	ErrPathNotFound       = errors.New("Path not found for this RAN Area")

	ErrUpfNotAssociated    = errors.New("UPF not associated")
	ErrUpfNotFound         = errors.New("UPF not found")
	ErrInterfaceNotFound   = errors.New("interface not found")
	ErrNoPFCPRule          = errors.New("no PFCP rule to push")
	ErrNoIpAvailableInPool = errors.New("no IP address available in pool")

	ErrNilCtx            = errors.New("nil context")
	ErrSmfNotStarted     = errors.New("SMF not started")
	ErrSmfAlreadyStarted = errors.New("SMF already started")
)
