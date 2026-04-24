// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"slices"

	"github.com/nextmn/cp-lite/internal/config"

	"github.com/nextmn/json-api/jsonapi"
)

type AreasMap struct {
	content map[config.AreaName][]jsonapi.ControlURI
}

func NewAreasMap(areas map[config.AreaName]config.Area) AreasMap {
	m := AreasMap{
		content: make(map[config.AreaName][]jsonapi.ControlURI),
	}
	for k, area := range areas {
		m.content[k] = area.Gnbs
	}
	return m
}

func (a AreasMap) Area(gnb jsonapi.ControlURI) (config.AreaName, bool) {
	for name, area := range a.content {
		if slices.Contains(area, gnb) {
			return name, true
		}
	}
	return "", false
}

func (a AreasMap) Contains(areaName config.AreaName, gnb jsonapi.ControlURI) bool {
	if area, ok := a.content[areaName]; ok {
		if slices.Contains(area, gnb) {
			return true
		}
	}
	return false
}
