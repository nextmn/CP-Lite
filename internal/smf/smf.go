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
	Areas   AreasMap
	srv     *pfcp.PFCPEntityCP
	started bool
	closed  chan struct{}

	// not exported because must not be modified
	ctx context.Context
}

func NewSmf(addr netip.Addr, slices map[string]config.Slice, areas map[string]config.Area) *Smf {
	s := NewSlicesMap(slices, areas)
	upfs := NewUpfsMap(slices)
	return &Smf{
		srv:    pfcp.NewPFCPEntityCP(addr.String(), addr),
		slices: s,
		upfs:   upfs,
		Areas:  NewAreasMap(areas),
		closed: make(chan struct{}),
		ctx:    nil,
	}
}

func (smf *Smf) Start(ctx context.Context) error {
	if smf.started {
		return ErrSmfAlreadyStarted
	}
	if ctx == nil {
		return ErrNilCtx
	}
	smf.ctx = ctx
	logrus.Info("Starting PFCP Server")
	go func() {
		defer func() {
			smf.started = false
			close(smf.closed)
		}()
		if err := smf.srv.ListenAndServeContext(ctx); err != nil {
			logrus.WithError(err).Info("PFCP server stopped")
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
		if err := upf.Associate(ctx, association); err != nil {
			failure = err
			return false
		}
		return true
	})
	if failure != nil {
		return failure
	}
	logrus.Info("PFCP Associations complete")
	smf.started = true
	return nil
}

func (smf *Smf) Context() context.Context {
	if smf.ctx != nil {
		return smf.ctx
	}
	return context.Background()
}

func (smf *Smf) CreateSessionDownlink(ueCtrl jsonapi.ControlURI, ueIp netip.Addr, dnn string, gnbCtrl jsonapi.ControlURI, gnbFteid jsonapi.Fteid) (*PduSessionN3, error) {
	return smf.CreateSessionDownlinkContext(smf.ctx, ueCtrl, ueIp, dnn, gnbCtrl, gnbFteid)
}

func (smf *Smf) CreateSessionDownlinkContext(ctx context.Context, ueCtrl jsonapi.ControlURI, ueIp netip.Addr, dnn string, gnbCtrl jsonapi.ControlURI, gnbFteid jsonapi.Fteid) (*PduSessionN3, error) {
	if !smf.started {
		return nil, ErrSmfNotStarted
	}
	if ctx == nil {
		return nil, ErrNilCtx
	}
	select {
	case <-ctx.Done():
		// if ctx is over, abort
		return nil, ctx.Err()
	case <-smf.ctx.Done():
		// if smf.ctx is over, abort
		return nil, smf.ctx.Err()
	default:
	}
	// check for existing session
	s, ok := smf.slices.Load(dnn)
	if !ok {
		return nil, ErrDnnNotFound
	}
	slice := s.(*Slice)
	session, err := slice.sessions.Get(ueCtrl, ueIp)
	if err != nil {
		return nil, err
	}
	session.DownlinkFteid = &gnbFteid
	if len(slice.Upfs) == 0 {
		return nil, ErrUpfNotFound
	}
	last_fteid := session.DownlinkFteid

	area, ok := smf.Areas.Area(gnbCtrl)
	if !ok {
		return nil, ErrAreaNotFound
	}

	path, ok := slice.Paths[area]
	if !ok {
		return nil, ErrPathNotFound
	}

	for i, gtpInterface := range path {
		upf_any, ok := smf.upfs.Load(gtpInterface.NodeID)
		if !ok {
			return nil, ErrUpfNotFound
		}
		upf := upf_any.(*Upf)

		var far_id uint32
		if i == len(slice.Upfs)-1 {
			far_id = upf.UpdateDownlinkAnchor(session.UeIpAddr, dnn, last_fteid)
		} else {
			last_fteid, far_id, err = upf.UpdateDownlinkIntermediateContext(ctx, session.UeIpAddr, dnn, gtpInterface.InterfaceAddr, last_fteid)
			if err != nil {
				return nil, err
			}
		}
		session.DlFarId = far_id
		if err := upf.UpdateSession(session.UeIpAddr); err != nil {
			return nil, err
		}
	}
	return session, nil
}

func (smf *Smf) CreateSessionUplink(ueCtrl jsonapi.ControlURI, gnbCtrl jsonapi.ControlURI, dnn string) (*PduSessionN3, error) {
	return smf.CreateSessionUplinkContext(smf.ctx, ueCtrl, gnbCtrl, dnn)
}

func (smf *Smf) CreateSessionUplinkContext(ctx context.Context, ueCtrl jsonapi.ControlURI, gnbCtrl jsonapi.ControlURI, dnn string) (*PduSessionN3, error) {
	if !smf.started {
		return nil, ErrSmfNotStarted
	}
	if ctx == nil {
		return nil, ErrNilCtx
	}
	select {
	case <-ctx.Done():
		// if ctx is over, abort
		return nil, ctx.Err()
	case <-smf.ctx.Done():
		// if smf.ctx is over, abort
		return nil, smf.ctx.Err()
	default:
	}
	// check for existing session
	s, ok := smf.slices.Load(dnn)
	if !ok {
		return nil, ErrDnnNotFound
	}
	slice := s.(*Slice)
	// create ue ip addr
	ueIpAddr, err := slice.Pool.Next()
	if err != nil {
		return nil, err
	}
	// create new session
	// 1. check path
	area, ok := smf.Areas.Area(gnbCtrl)
	if !ok {
		return nil, ErrAreaNotFound
	}

	path, ok := slice.Paths[area]
	if !ok {
		return nil, ErrPathNotFound
	}

	if len(path) == 0 {
		return nil, ErrUpfNotFound
	}
	// 2. init anchor
	upfaInterface := path[len(path)-1]
	upfa_any, ok := smf.upfs.Load(upfaInterface.NodeID)
	if !ok {
		return nil, ErrUpfNotFound
	}
	upfa := upfa_any.(*Upf)
	last_fteid, err := upfa.CreateUplinkAnchorContext(ctx, ueIpAddr, dnn, upfaInterface.InterfaceAddr)
	if err != nil {
		return nil, err
	}
	if err := upfa.CreateSession(ueIpAddr); err != nil {
		return nil, err
	}

	// 3. init path from anchor
	for i := len(path) - 2; i >= 0; i-- {
		gtpInterface := path[i]
		upf_any, ok := smf.upfs.Load(gtpInterface.NodeID)
		if !ok {
			return nil, ErrUpfNotFound
		}
		upf := upf_any.(*Upf)
		last_fteid, err = upf.CreateUplinkIntermediateContext(ctx, ueIpAddr, dnn, gtpInterface.InterfaceAddr, last_fteid)
		if err != nil {
			logrus.WithError(err).Error("Could not create uplink intermediate")
			return nil, err
		}
		if err := upf.CreateSession(ueIpAddr); err != nil {
			logrus.WithError(err).Error("Could not create session uplink")
			return nil, err
		}
	}

	session, err := slice.sessions.Get(ueCtrl, ueIpAddr)
	if err != nil {
		// store session
		session = &PduSessionN3{
			UeIpAddr:    ueIpAddr,
			UplinkFteid: last_fteid,
		}
		slice.sessions.Add(ueCtrl, session)
	} else {
		// update session
		if err := slice.sessions.SetUplinkFteid(ueCtrl, ueIpAddr, last_fteid); err != nil {
			return nil, err
		}
	}
	return session, nil
}

func (smf *Smf) WaitShutdown(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-smf.closed:
		return nil
	}
}

func (smf *Smf) GetSessionUplinkFteid(ueCtrl jsonapi.ControlURI, ueAddr netip.Addr, dnn string) (*jsonapi.Fteid, error) {
	slice, ok := smf.slices.Load(dnn)
	if !ok {
		return nil, ErrDnnNotFound
	}
	session, err := slice.(*Slice).sessions.Get(ueCtrl, ueAddr)
	if err != nil {
		return nil, err
	}
	return session.UplinkFteid, nil
}

func (smf *Smf) SetSessionIndirectForwardingRequired(ueCtrl jsonapi.ControlURI, ueAddr netip.Addr, dnn string, value bool) error {
	slice, ok := smf.slices.Load(dnn)
	if !ok {
		return ErrDnnNotFound
	}
	return slice.(*Slice).sessions.SetIndirectForwardingRequired(ueCtrl, ueAddr, value)
}

func (smf *Smf) GetSessionDownlinkFteid(ueCtrl jsonapi.ControlURI, ueAddr netip.Addr, dnn string) (*jsonapi.Fteid, error) {
	slice, ok := smf.slices.Load(dnn)
	if !ok {
		return nil, ErrDnnNotFound
	}
	session, err := slice.(*Slice).sessions.Get(ueCtrl, ueAddr)
	if err != nil {
		return nil, err
	}
	return session.DownlinkFteid, nil
}

func (smf *Smf) StoreNextDownlinkFteid(ueCtrl jsonapi.ControlURI, ueAddr netip.Addr, dnn string, fteid *jsonapi.Fteid) error {
	slice, ok := smf.slices.Load(dnn)
	if !ok {
		return ErrDnnNotFound
	}
	return slice.(*Slice).sessions.SetNextDownlinkFteid(ueCtrl, ueAddr, fteid)
}

func (smf *Smf) UpdateSessionDownlink(ueCtrl jsonapi.ControlURI, ueAddr netip.Addr, dnn string, oldGnbCtrl jsonapi.ControlURI) error {
	return smf.UpdateSessionDownlinkContext(smf.ctx, ueCtrl, ueAddr, dnn, oldGnbCtrl)
}

// Updates Session to NextDownlinkFteid
func (smf *Smf) UpdateSessionDownlinkContext(ctx context.Context, ueCtrl jsonapi.ControlURI, ueAddr netip.Addr, dnn string, oldGnbCtrl jsonapi.ControlURI) error {
	s, ok := smf.slices.Load(dnn)
	if !ok {
		return ErrDnnNotFound
	}
	slice := s.(*Slice)

	session, err := slice.sessions.Get(ueCtrl, ueAddr)
	if err != nil {
		return err
	}

	area, ok := smf.Areas.Area(oldGnbCtrl)
	if !ok {
		return ErrAreaNotFound
	}

	path, ok := slice.Paths[area]
	if !ok {
		return ErrPathNotFound
	}

	if len(path) == 0 {
		return ErrUpfNotFound
	}
	upf_ctrl := path[0].NodeID // upf-i
	upf_any, ok := smf.upfs.Load(upf_ctrl)
	if !ok {
		return ErrUpfNotFound
	}
	upf := upf_any.(*Upf)
	upf.UpdateDownlinkIntermediateDirectForward(ueAddr, dnn, session.DlFarId, session.NextDownlinkFteid)

	return upf.UpdateSession(session.UeIpAddr)
}
