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
	s map[netip.Addr]*PduSessionN3
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
		if session, ok := sessions.s[ueAddr]; ok {
			return session, nil
		}
	}
	return nil, ErrPDUSessionNotFound
}

func (s *SessionsMap) Add(ueCtrl jsonapi.ControlURI, session *PduSessionN3) {
	s.Lock()
	defer s.Unlock()
	m, ok := s.m[ueCtrl]
	if !ok {
		s.m[ueCtrl] = &Sessions{
			s: map[netip.Addr]*PduSessionN3{
				session.UeIpAddr: session,
			},
		}
	} else {
		m.s[session.UeIpAddr] = session
	}
}

func (s *SessionsMap) SetNextDownlinkFteid(ueCtrl jsonapi.ControlURI, ueAddr netip.Addr, fteid *jsonapi.Fteid) error {
	s.Lock()
	defer s.Unlock()
	if sessions, ok := s.m[ueCtrl]; ok {
		if session, ok := sessions.s[ueAddr]; ok {
			session.NextDownlinkFteid = fteid
			return nil
		}
	}
	return ErrPDUSessionNotFound
}

func (s *SessionsMap) GetNextDownlinkFteid(ueCtrl jsonapi.ControlURI, ueAddr netip.Addr) (*jsonapi.Fteid, error) {
	s.Lock()
	defer s.Unlock()
	if sessions, ok := s.m[ueCtrl]; ok {
		if session, ok := sessions.s[ueAddr]; ok {
			return session.NextDownlinkFteid, nil
		}
	}
	return nil, ErrPDUSessionNotFound
}

func (s *SessionsMap) SetUplinkFteid(ueCtrl jsonapi.ControlURI, ueAddr netip.Addr, fteid *jsonapi.Fteid) error {
	s.Lock()
	defer s.Unlock()
	if sessions, ok := s.m[ueCtrl]; ok {
		if session, ok := sessions.s[ueAddr]; ok {
			session.UplinkFteid = fteid
			return nil
		}
	}
	return ErrPDUSessionNotFound
}

func (s *SessionsMap) SetIndirectForwardingRequired(ueCtrl jsonapi.ControlURI, ueAddr netip.Addr, value bool) error {
	s.Lock()
	defer s.Unlock()
	if sessions, ok := s.m[ueCtrl]; ok {
		if session, ok := sessions.s[ueAddr]; ok {
			session.IndirectForwardingRequired = value
			return nil
		}
	}
	return ErrPDUSessionNotFound
}

func (s *SessionsMap) GetIndirectForwardingRequired(ueCtrl jsonapi.ControlURI, ueAddr netip.Addr) (bool, error) {
	s.RLock()
	defer s.RUnlock()
	if sessions, ok := s.m[ueCtrl]; ok {
		if session, ok := sessions.s[ueAddr]; ok {
			return session.IndirectForwardingRequired, nil
		}
	}
	return false, ErrPDUSessionNotFound
}
