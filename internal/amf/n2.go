// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package amf

import (
	"time"

	"github.com/nextmn/cp-lite/internal/config"

	"github.com/nextmn/json-api/jsonapi"
)

type N2 struct {
	content map[jsonapi.ControlURI]time.Duration
}

func NewN2(areas map[config.AreaName]config.Area) N2 {
	n := N2{
		content: make(map[jsonapi.ControlURI]time.Duration),
	}
	for _, area := range areas {
		for _, uri := range area.Gnbs {
			n.content[uri] = area.OneWayDelay
		}
	}
	return n
}

// Returns one-way-delay to a given gNB (0ms if no one-way-delay is set).
func (n N2) OneWayDelay(uri jsonapi.ControlURI) time.Duration {
	return n.content[uri]
}
