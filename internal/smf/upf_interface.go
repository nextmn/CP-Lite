// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"net/netip"

	"github.com/nextmn/cp-lite/internal/config"
)

type UpfInterface struct {
	Teids *TEIDsPool
	Type  string
}

func NewUpfInterface(t string) *UpfInterface {
	return &UpfInterface{
		Teids: NewTEIDsPool(),
		Type:  t,
	}
}
func NewUpfInterfaceMap(ifaces []config.Interface) map[netip.Addr]*UpfInterface {
	r := make(map[netip.Addr]*UpfInterface)
	for _, v := range ifaces {
		r[v.Addr] = NewUpfInterface(v.Type)
	}
	return r
}
