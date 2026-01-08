// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"sync"

	pfcpapi "github.com/nextmn/go-pfcp-networking/pfcp/api"

	"github.com/wmnsk/go-pfcp/ie"
)

type Pfcprules struct {
	createpdrs   []*ie.IE
	createfars   []*ie.IE
	updatepdrs   []*ie.IE
	updatefars   []*ie.IE
	currentpdrid uint16
	currentfarid uint32
	session      pfcpapi.PFCPSessionInterface

	sync.Mutex
}

func NewPfcpRules() *Pfcprules {
	return &Pfcprules{
		createpdrs: make([]*ie.IE, 0),
		createfars: make([]*ie.IE, 0),
		updatepdrs: make([]*ie.IE, 0),
		updatefars: make([]*ie.IE, 0),
	}
}
