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
	closed  chan struct{}

	// not exported because must not be modified
	ctx context.Context
}

func NewSmf(addr netip.Addr, slices map[string]config.Slice) *Smf {
	s := NewSlicesMap(slices)
	upfs := NewUpfsMap(slices)
	return &Smf{
		srv:    pfcp.NewPFCPEntityCP(addr.String(), addr),
		slices: s,
		upfs:   upfs,
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

func (smf *Smf) CreateSessionDownlink(ueCtrl jsonapi.ControlURI, ueIp netip.Addr, dnn string, gnb netip.Addr, gnb_teid uint32) (*PduSessionN3, error) {
	return smf.CreateSessionDownlinkContext(smf.ctx, ueCtrl, ueIp, dnn, gnb, gnb_teid)
}

func (smf *Smf) CreateSessionDownlinkContext(ctx context.Context, ueCtrl jsonapi.ControlURI, ueIp netip.Addr, dnn string, gnb netip.Addr, gnb_teid uint32) (*PduSessionN3, error) {
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
			upf_iface, err = upf.GetN9()
		}
		if err != nil {
			return nil, err
		}
		if i == len(slice.Upfs)-1 {
			upf.UpdateDownlinkAnchor(session.UeIpAddr, dnn, last_fteid)
		} else {
			last_fteid, err = upf.UpdateDownlinkIntermediateContext(ctx, session.UeIpAddr, dnn, upf_iface, last_fteid)
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
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"upf":   upfa_ctrl,
				"iface": "n3",
				"type":  "anchor",
			}).Error("Could not prepare session uplink path")
			return nil, err
		}
	} else {
		upfa_iface, err = upfa.GetN9()
		if err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"upf":   upfa_ctrl,
				"iface": "n9",
				"type":  "anchor",
			}).Error("Could not prepare session uplink path")
			return nil, err
		}
	}
	last_fteid, err := upfa.CreateUplinkAnchorContext(ctx, ueIpAddr, dnn, upfa_iface)
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
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"upf":   upf_ctrl,
					"iface": "n3",
					"type":  "inter",
				}).Error("Could not prepare session uplink path")
				return nil, err
			}
		} else {
			upf_iface, err = upf.GetN9()
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"upf":   upf_ctrl,
					"iface": "n9",
					"type":  "inter",
				}).Error("Could not prepare session uplink path")
				return nil, err
			}
		}
		last_fteid, err = upf.CreateUplinkIntermediateContext(ctx, ueIpAddr, dnn, upf_iface, last_fteid)
		if err != nil {
			logrus.WithError(err).Error("Could not create uplink intermediate")
			return nil, err
		}
		if err := upf.CreateSession(ueIpAddr); err != nil {
			logrus.WithError(err).Error("Could not create session uplink")
			return nil, err
		}
	}

	session := PduSessionN3{
		UeIpAddr:    ueIpAddr,
		UplinkFteid: last_fteid,
	}
	// store session
	slice.sessions.Add(ueCtrl, &session)
	return &session, nil
}

func (smf *Smf) WaitShutdown(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-smf.closed:
		return nil
	}
}
