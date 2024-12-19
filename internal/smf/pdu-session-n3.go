// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"net/netip"

	"github.com/nextmn/json-api/jsonapi"
)

type PduSessionN3 struct {
	UeIpAddr          netip.Addr
	UplinkFteid       *jsonapi.Fteid
	DownlinkFteid     *jsonapi.Fteid
	NextDownlinkFteid *jsonapi.Fteid // Handover
	DlFarId           uint32
}
