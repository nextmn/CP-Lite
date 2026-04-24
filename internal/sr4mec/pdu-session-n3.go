// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package sr4mec

import (
	"net/netip"
	"net/url"

	"github.com/nextmn/json-api/jsonapi"
)

type PduSessionN3 struct {
	UeIpAddr netip.Addr

	// At PS Establishment
	UplinkFteid   *jsonapi.Fteid
	UplinkRule    *url.URL
	DownlinkFteid *jsonapi.Fteid
	DownlinkRule  *url.URL

	// After HO Required
	TargetUplinkFteid *jsonapi.Fteid
	TargetUplinkRule  *url.URL

	// After HO Request Ack
	ForwardingFteid     *jsonapi.Fteid
	ForwardingRule      *url.URL
	TargetDownlinkFteid *jsonapi.Fteid
	TargetDownlinkRule  *url.URL
}
