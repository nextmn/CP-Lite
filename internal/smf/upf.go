// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"context"
	"net/netip"
	"sync"

	"github.com/nextmn/cp-lite/internal/common"
	"github.com/nextmn/cp-lite/internal/config"

	pfcp "github.com/nextmn/go-pfcp-networking/pfcp"
	pfcpapi "github.com/nextmn/go-pfcp-networking/pfcp/api"
	"github.com/nextmn/json-api/jsonapi"

	"github.com/wmnsk/go-pfcp/ie"
)

type UpfsMap struct {
	sync.Map
}

func NewUpfsMap(slices map[string]config.Slice) *UpfsMap {
	m := UpfsMap{}
	for _, slice := range slices {
		for _, upf := range slice.Upfs {
			if _, ok := m.Load(upf.NodeID); ok {
				// upf used in more than a single slice
				continue
			}
			m.Store(upf.NodeID, NewUpf(upf.Interfaces))
		}
	}
	return &m
}

type Upf struct {
	common.WithContext
	association pfcpapi.PFCPAssociationInterface
	interfaces  map[netip.Addr]*UpfInterface
	sessions    map[netip.Addr]*Pfcprules
}

func NewUpf(interfaces []config.Interface) *Upf {
	upf := Upf{
		interfaces: NewUpfInterfaceMap(interfaces),
		sessions:   make(map[netip.Addr]*Pfcprules),
	}
	return &upf
}

func (upf *Upf) Associate(ctx context.Context, a pfcpapi.PFCPAssociationInterface) error {
	if err := upf.InitContext(ctx); err != nil {
		return err
	}
	// Initialize TeidPools
	for _, iface := range upf.interfaces {
		if err := iface.Teids.InitContext(ctx); err != nil {
			return err
		}
	}
	upf.association = a
	return nil
}

func (upf *Upf) Rules(ueIp netip.Addr) *Pfcprules {
	rules, ok := upf.sessions[ueIp]
	if !ok {
		rules = NewPfcpRules()
		upf.sessions[ueIp] = rules
	}
	return rules
}

func (upf *Upf) NextListenFteid(listenInterface netip.Addr) (*jsonapi.Fteid, error) {
	return upf.NextListenFteidContext(upf.Context(), listenInterface)
}

func (upf *Upf) NextListenFteidContext(ctx context.Context, listenInterface netip.Addr) (*jsonapi.Fteid, error) {
	upfCtx := upf.Context()
	if ctx == nil {
		return nil, ErrNilCtx
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-upfCtx.Done():
		return nil, upfCtx.Err()
	default:
	}
	iface, ok := upf.interfaces[listenInterface]
	if !ok {
		return nil, ErrInterfaceNotFound
	}
	teid, err := iface.Teids.Next(ctx)
	if err != nil {
		return nil, err
	}
	return &jsonapi.Fteid{
		Addr: listenInterface,
		Teid: teid,
	}, nil
}

func (upf *Upf) CreateUplinkIntermediate(ueIp netip.Addr, dnn string, listenInterface netip.Addr, forwardFteid *jsonapi.Fteid) (*jsonapi.Fteid, error) {
	return upf.CreateUplinkIntermediateContext(upf.Context(), ueIp, dnn, listenInterface, forwardFteid)
}

func (upf *Upf) CreateUplinkIntermediateContext(ctx context.Context, ueIp netip.Addr, dnn string, listenInterface netip.Addr, forwardFteid *jsonapi.Fteid) (*jsonapi.Fteid, error) {
	if ctx == nil {
		return nil, ErrNilCtx
	}
	upfCtx := upf.Context()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-upfCtx.Done():
		return nil, upfCtx.Err()
	default:
	}
	listenFteid, err := upf.NextListenFteidContext(ctx, listenInterface)
	if err != nil {
		return nil, err
	}
	upf.CreateUplinkIntermediateWithFteid(ueIp, dnn, listenFteid, forwardFteid)
	return listenFteid, nil
}

func (upf *Upf) CreateUplinkIntermediateWithFteid(ueIp netip.Addr, dnn string, listenFteid *jsonapi.Fteid, forwardFteid *jsonapi.Fteid) {
	r := upf.Rules(ueIp)
	r.Lock()
	defer r.Unlock()
	r.currentpdrid += 1
	r.currentfarid += 1

	r.createpdrs = append(r.createpdrs, ie.NewCreatePDR(ie.NewPDRID(r.currentpdrid), ie.NewPrecedence(255),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceAccess),
			ie.NewFTEID(FteidTypeIPv4, listenFteid.Teid, listenFteid.Addr.AsSlice(), nil, 0),
			ie.NewNetworkInstance(dnn),
			ie.NewUEIPAddress(UEIpAddrTypeIPv4Source, ueIp.String(), "", 0, 0),
		),
		ie.NewOuterHeaderRemoval(OuterHeaderRemoveGtpuUdpIpv4, 0),
		ie.NewFARID(r.currentfarid),
	))
	r.createfars = append(r.createfars, ie.NewCreateFAR(ie.NewFARID(r.currentfarid),
		ie.NewApplyAction(ApplyActionForw),
		ie.NewForwardingParameters(
			ie.NewDestinationInterface(ie.DstInterfaceCore),
			ie.NewNetworkInstance(dnn),
			ie.NewOuterHeaderCreation(
				OuterHeaderCreationGtpuUdpIpv4,
				forwardFteid.Teid,
				forwardFteid.Addr.String(),
				"", 0, 0, 0,
			),
		),
	))
}

func (upf *Upf) CreateUplinkAnchor(ueIp netip.Addr, dnn string, listenInterface netip.Addr) (*jsonapi.Fteid, error) {
	return upf.CreateUplinkAnchorContext(upf.Context(), ueIp, dnn, listenInterface)
}
func (upf *Upf) CreateUplinkAnchorContext(ctx context.Context, ueIp netip.Addr, dnn string, listenInterface netip.Addr) (*jsonapi.Fteid, error) {
	if ctx == nil {
		return nil, ErrNilCtx
	}
	listenFteid, err := upf.NextListenFteidContext(ctx, listenInterface)
	if err != nil {
		return nil, err
	}
	upf.CreateUplinkAnchorWithFteid(ueIp, dnn, listenFteid)
	return listenFteid, nil
}

func (upf *Upf) CreateUplinkAnchorWithFteid(ueIp netip.Addr, dnn string, listenFteid *jsonapi.Fteid) {
	r := upf.Rules(ueIp)
	r.Lock()
	defer r.Unlock()
	r.currentpdrid += 1
	r.currentfarid += 1

	r.createpdrs = append(r.createpdrs, ie.NewCreatePDR(ie.NewPDRID(r.currentpdrid), ie.NewPrecedence(255),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceAccess),
			ie.NewFTEID(FteidTypeIPv4, listenFteid.Teid, listenFteid.Addr.AsSlice(), nil, 0),
			ie.NewNetworkInstance(dnn),
			ie.NewUEIPAddress(UEIpAddrTypeIPv4Source, ueIp.String(), "", 0, 0),
		),
		ie.NewOuterHeaderRemoval(OuterHeaderRemoveGtpuUdpIpv4, 0),
		ie.NewFARID(r.currentfarid),
	))
	r.createfars = append(r.createfars, ie.NewCreateFAR(ie.NewFARID(r.currentfarid),
		ie.NewApplyAction(ApplyActionForw),
		ie.NewForwardingParameters(
			ie.NewDestinationInterface(ie.DstInterfaceCore),
			ie.NewNetworkInstance(dnn),
		),
	))
}

func (upf *Upf) UpdateDownlinkAnchor(ueIp netip.Addr, dnn string, forwardFteid *jsonapi.Fteid) uint32 {
	r := upf.Rules(ueIp)
	r.Lock()
	defer r.Unlock()
	r.currentpdrid += 1
	r.currentfarid += 1

	r.createpdrs = append(r.createpdrs, ie.NewCreatePDR(ie.NewPDRID(r.currentpdrid), ie.NewPrecedence(255),
		ie.NewPDI(ie.NewSourceInterface(ie.SrcInterfaceCore),
			ie.NewNetworkInstance(dnn),
			ie.NewUEIPAddress(UEIpAddrTypeIPv4Destination, ueIp.String(), "", 0, 0),
		),
		ie.NewFARID(r.currentfarid),
	),
	)
	r.createfars = append(r.createfars, ie.NewCreateFAR(ie.NewFARID(r.currentfarid),
		ie.NewApplyAction(ApplyActionForw),
		ie.NewForwardingParameters(
			ie.NewDestinationInterface(ie.DstInterfaceAccess),
			ie.NewNetworkInstance(dnn),
			ie.NewOuterHeaderCreation(
				OuterHeaderCreationGtpuUdpIpv4,
				forwardFteid.Teid,
				forwardFteid.Addr.String(),
				"", 0, 0, 0,
			),
		),
	))
	return r.currentfarid
}

func (upf *Upf) UpdateDownlinkIntermediateDirectForward(ueIp netip.Addr, dnn string, farid uint32, fteid *jsonapi.Fteid) {
	r := upf.Rules(ueIp)
	r.Lock()
	defer r.Unlock()
	r.updatefars = append(r.updatefars, ie.NewUpdateFAR(ie.NewFARID(farid),
		ie.NewApplyAction(ApplyActionForw),
		ie.NewUpdateForwardingParameters(
			ie.NewDestinationInterface(ie.DstInterfaceAccess),
			ie.NewNetworkInstance(dnn),
			ie.NewOuterHeaderCreation(
				OuterHeaderCreationGtpuUdpIpv4,
				fteid.Teid,
				fteid.Addr.String(),
				"", 0, 0, 0,
			),
		),
	))
}

func (upf *Upf) UpdateDownlinkIntermediate(ueIp netip.Addr, dnn string, listenInterface netip.Addr, forwardFteid *jsonapi.Fteid) (*jsonapi.Fteid, uint32, error) {
	return upf.UpdateDownlinkIntermediateContext(upf.Context(), ueIp, dnn, listenInterface, forwardFteid)
}
func (upf *Upf) UpdateDownlinkIntermediateContext(ctx context.Context, ueIp netip.Addr, dnn string, listenInterface netip.Addr, forwardFteid *jsonapi.Fteid) (*jsonapi.Fteid, uint32, error) {
	if ctx == nil {
		return nil, 0, ErrNilCtx
	}
	listenFteid, err := upf.NextListenFteidContext(ctx, listenInterface)
	if err != nil {
		return nil, 0, err
	}
	return listenFteid, upf.UpdateDownlinkIntermediateWithFteid(ueIp, dnn, listenFteid, forwardFteid), nil
}

func (upf *Upf) UpdateDownlinkIntermediateWithFteid(ueIp netip.Addr, dnn string, listenFteid *jsonapi.Fteid, forwardFteid *jsonapi.Fteid) uint32 {
	r := upf.Rules(ueIp)
	r.Lock()
	defer r.Unlock()
	r.currentpdrid += 1
	r.currentfarid += 1

	r.createpdrs = append(r.createpdrs, ie.NewCreatePDR(ie.NewPDRID(r.currentpdrid), ie.NewPrecedence(255),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceCore),
			ie.NewFTEID(FteidTypeIPv4, listenFteid.Teid, listenFteid.Addr.AsSlice(), nil, 0),
			ie.NewNetworkInstance(dnn),
			ie.NewUEIPAddress(UEIpAddrTypeIPv4Destination, ueIp.String(), "", 0, 0),
		),
		ie.NewOuterHeaderRemoval(OuterHeaderRemoveGtpuUdpIpv4, 0),
		ie.NewFARID(r.currentfarid),
	),
	)
	r.createfars = append(r.createfars, ie.NewCreateFAR(ie.NewFARID(r.currentfarid),
		ie.NewApplyAction(ApplyActionForw),
		ie.NewForwardingParameters(
			ie.NewDestinationInterface(ie.DstInterfaceAccess),
			ie.NewNetworkInstance(dnn),
			ie.NewOuterHeaderCreation(
				OuterHeaderCreationGtpuUdpIpv4,
				forwardFteid.Teid,
				forwardFteid.Addr.String(),
				"", 0, 0, 0,
			),
		),
	))

	return r.currentfarid
}

func (upf *Upf) CreateSession(ue netip.Addr) error {
	rules, ok := upf.sessions[ue]
	if !ok {
		return ErrNoPFCPRule
	}
	rules.Lock()
	defer rules.Unlock()

	createpdrs, err, _, _ := pfcp.NewPDRMap(rules.createpdrs)
	if err != nil {
		return err
	}
	createfars, err, _, _ := pfcp.NewFARMap(rules.createfars)
	if err != nil {
		return err
	}
	if upf.association == nil {
		return ErrUpfNotAssociated
	}
	rules.session, err = upf.association.CreateSession(nil, createpdrs, createfars)
	if err != nil {
		return err
	}
	// clear
	rules.createpdrs = make([]*ie.IE, 0)
	rules.createfars = make([]*ie.IE, 0)
	return nil
}

func (upf *Upf) UpdateSession(ue netip.Addr) error {
	rules, ok := upf.sessions[ue]
	if !ok {
		return ErrNoPFCPRule
	}
	rules.Lock()
	defer rules.Unlock()
	if rules.session == nil {
		return ErrPDUSessionNotFound
	}
	createpdrs, err, _, _ := pfcp.NewPDRMap(rules.createpdrs)
	if err != nil {
		return err
	}
	createfars, err, _, _ := pfcp.NewFARMap(rules.createfars)
	if err != nil {
		return err
	}
	updatepdrs, err, _, _ := pfcp.NewPDRMap(rules.updatepdrs)
	if err != nil {
		return err
	}
	updatefars, err, _, _ := pfcp.NewFARMapUpdate(rules.updatefars)
	if err != nil {
		return err
	}
	if upf.association == nil {
		return ErrUpfNotAssociated
	}
	err = rules.session.AddUpdatePDRsFARs(createpdrs, createfars, updatepdrs, updatefars)
	if err != nil {
		return err
	}
	// clear
	rules.createpdrs = make([]*ie.IE, 0)
	rules.createfars = make([]*ie.IE, 0)
	rules.updatepdrs = make([]*ie.IE, 0)
	rules.updatefars = make([]*ie.IE, 0)

	return nil
}
