// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

// PFCP Constants
const (
	FteidTypeIPv4                  = 0x01
	UEIpAddrTypeIPv4Source         = 0x02
	UEIpAddrTypeIPv4Destination    = 0x02 | 0x04 // S/D Flag = 1
	OuterHeaderRemoveGtpuUdpIpv4   = 0x00
	ApplyActionForw                = 0x02
	OuterHeaderCreationGtpuUdpIpv4 = 0x0100
)
