// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"context"
	"net/netip"
	"time"

	"github.com/nextmn/cp-lite/internal/config"

	pfcp "github.com/nextmn/go-pfcp-networking/pfcp"
	"github.com/nextmn/json-api/jsonapi"

	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

type UpfPath []netip.Addr

type Smf struct {
	upfs    *UpfsMap
	slices  *SlicesMap
	srv     *pfcp.PFCPEntityCP
	started bool
}

func NewSmf(addr netip.Addr, slices map[string]config.Slice) *Smf {
	s := NewSlicesMap(slices)
	upfs := NewUpfsMap(slices)
	return &Smf{
		srv:    pfcp.NewPFCPEntityCP(addr.String(), addr),
		slices: s,
		upfs:   upfs,
	}
}

func (smf *Smf) Start(ctx context.Context) error {
	logrus.Info("Starting PFCP Server")
	go func() {
		err := smf.srv.ListenAndServeContext(ctx)
		if err != nil {
			logrus.WithError(err).Trace("PFCP server stopped")
		}
	}()
	ctxTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if err := smf.srv.WaitReady(ctxTimeout); err != nil {
		logrus.WithError(err).Fatal("Could not start PFCP server")
		return err
	}
	var failure error
	smf.upfs.Range(func(key, value any) bool {
		nodeId := key.(netip.Addr)
		upf := value.(*Upf)
		association, err := smf.srv.NewEstablishedPFCPAssociation(ie.NewNodeIDHeuristic(nodeId.String()))
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"upf": nodeId,
			}).Error("Could not perform PFCP association")
			failure = err
			return false
		}
		upf.Associate(association)
		return true
	})
	if failure != nil {
		return failure
	}
	logrus.Info("PFCP Associations complete")
	smf.started = true
	return nil
}

func (smf *Smf) CreateSessionDownlink(ctx context.Context, ueCtrl jsonapi.ControlURI, dnn string, gnb netip.Addr, gnb_teid uint32) (*PduSessionN3, error) {
	// check for existing session
	s, ok := smf.slices.Load(dnn)
	if !ok {
		return nil, ErrDnnNotFound
	}
	slice := s.(*Slice)
	session_any, ok := slice.sessions.Load(ueCtrl)
	if !ok {
		return nil, ErrPDUSessionNotFound
	}
	session := session_any.(*PduSessionN3)
	session.DownlinkFteid = &Fteid{
		Addr: gnb,
		Teid: gnb_teid,
	}
	if len(slice.Upfs) == 0 {
		return nil, ErrUpfNotFound
	}
	last_fteid := session.DownlinkFteid
	for i, upf_ctrl := range slice.Upfs {
		upf_any, ok := smf.upfs.Load(upf_ctrl)
		if !ok {
			return nil, ErrUpfNotFound
		}
		upf := upf_any.(*Upf)
		var err error
		var upf_iface netip.Addr
		if i == 0 {
			upf_iface, err = upf.GetN3()
		} else if i != len(slice.Upfs)-1 {
			upf_iface, err = upf.GetN6()
		}
		if err != nil {
			return nil, err
		}
		if i == len(slice.Upfs)-1 {
			upf.UpdateDownlinkAnchor(session.UeIpAddr, dnn, last_fteid)
		} else {
			last_fteid, err = upf.UpdateDownlinkIntermediate(ctx, session.UeIpAddr, dnn, upf_iface, last_fteid)
			if err != nil {
				return nil, err
			}
		}
		if err := upf.UpdateSession(session.UeIpAddr); err != nil {
			return nil, err
		}
	}
	return session, nil
}
func (smf *Smf) CreateSessionUplink(ctx context.Context, ueCtrl jsonapi.ControlURI, gnbCtrl jsonapi.ControlURI, dnn string) (*PduSessionN3, error) {
	// check for existing session
	s, ok := smf.slices.Load(dnn)
	if !ok {
		return nil, ErrDnnNotFound
	}
	slice := s.(*Slice)
	_, ok = slice.sessions.Load(ueCtrl)
	if ok {
		return nil, ErrPDUSessionAlreadyExists
	}
	// create ue ip addr
	ueIpAddr, err := slice.Pool.Next()
	if err != nil {
		return nil, err
	}
	// create new session
	// 1. check path
	if len(slice.Upfs) == 0 {
		return nil, ErrUpfNotFound
	}
	// 2. init anchor
	upfa_ctrl := slice.Upfs[len(slice.Upfs)-1]
	upfa_any, ok := smf.upfs.Load(upfa_ctrl)
	if !ok {
		return nil, ErrUpfNotFound
	}
	upfa := upfa_any.(*Upf)
	var upfa_iface netip.Addr
	if len(slice.Upfs) == 1 {
		upfa_iface, err = upfa.GetN3()
	} else {
		upfa_iface, err = upfa.GetN6()
	}
	if err != nil {
		return nil, err
	}
	last_fteid, err := upfa.CreateUplinkAnchor(ctx, ueIpAddr, dnn, upfa_iface)
	if err != nil {
		return nil, err
	}
	if err := upfa.CreateSession(ueIpAddr); err != nil {
		return nil, err
	}

	// 3. init path from anchor
	for i := len(slice.Upfs) - 2; i >= 0; i-- {
		upf_ctrl := slice.Upfs[i]
		upf_any, ok := smf.upfs.Load(upf_ctrl)
		if !ok {
			return nil, ErrUpfNotFound
		}
		upf := upf_any.(*Upf)
		var upf_iface netip.Addr
		if i == 0 {
			upf_iface, err = upf.GetN3()
		} else {
			upf_iface, err = upf.GetN6()
		}
		if err != nil {
			return nil, err
		}
		last_fteid, err = upf.CreateUplinkIntermediate(ctx, ueIpAddr, dnn, upf_iface, last_fteid)
		if err != nil {
			return nil, err
		}
		if err := upf.CreateSession(ueIpAddr); err != nil {
			return nil, err
		}
	}

	session := PduSessionN3{
		UeIpAddr:    ueIpAddr,
		UplinkFteid: last_fteid,
	}
	// store session
	slice.sessions.Store(ueCtrl, &session)
	return &session, nil
}
