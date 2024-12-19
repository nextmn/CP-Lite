// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"net/netip"
	"sync"

	"github.com/nextmn/json-api/jsonapi"
)

type Sessions struct {
	s []*PduSessionN3
}

type SessionsMap struct {
	m map[jsonapi.ControlURI]*Sessions
	sync.RWMutex
}

func NewSessionsMap() *SessionsMap {
	return &SessionsMap{
		m: make(map[jsonapi.ControlURI]*Sessions),
	}
}

func (s *SessionsMap) Get(ueCtrl jsonapi.ControlURI, ueAddr netip.Addr) (*PduSessionN3, error) {
	s.RLock()
	defer s.RUnlock()
	if sessions, ok := s.m[ueCtrl]; ok {
		for _, session := range sessions.s {
			if session.UeIpAddr == ueAddr {
				return session, nil
			}
		}
	}
	return nil, ErrPDUSessionNotFound
}

func (s *SessionsMap) Add(ueCtrl jsonapi.ControlURI, session *PduSessionN3) {
	s.Lock()
	defer s.Unlock()
	m, ok := s.m[ueCtrl]
	if !ok {
		s.m[ueCtrl] = &Sessions{s: []*PduSessionN3{session}}
	} else {
		m.s = append(m.s, session)
	}
}

func (s *SessionsMap) SetNextDownlinkFteid(ueCtrl jsonapi.ControlURI, ueAddr netip.Addr, fteid *jsonapi.Fteid) error {
	s.Lock()
	defer s.Unlock()
	if sessions, ok := s.m[ueCtrl]; ok {
		for _, session := range sessions.s {
			if session.UeIpAddr == ueAddr {
				session.NextDownlinkFteid = fteid
			}
		}
	}
	return ErrPDUSessionNotFound
}
