// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"net/netip"
	"strings"

	"github.com/nextmn/cp-lite/internal/config"
)

type UpfInterface struct {
	Teids *TEIDsPool
	Types []string
}

func NewUpfInterface(t string) *UpfInterface {
	return &UpfInterface{
		Teids: NewTEIDsPool(),
		Types: []string{t},
	}
}
func NewUpfInterfaceMap(ifaces []config.Interface) map[netip.Addr]*UpfInterface {
	r := make(map[netip.Addr]*UpfInterface)
	for _, v := range ifaces {
		if i, ok := r[v.Addr]; ok {
			r[v.Addr].Types = append(i.Types, v.Type)
		} else {
			r[v.Addr] = NewUpfInterface(v.Type)
		}
	}
	return r
}

func (iface *UpfInterface) IsN3() bool {
	for _, t := range iface.Types {
		if strings.ToLower(t) == "n3" {
			return true
		}
	}
	return false
}

func (iface *UpfInterface) IsN9() bool {
	for _, t := range iface.Types {
		if strings.ToLower(t) == "n9" {
			return true
		}
	}
	return false
}
