// Copyright Louis Royer and the NextMN contributors. All rights reserved.
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

func (upf *Upf) Associate(ctx context.Context, a pfcpapi.PFCPAssociationInterface) {
	upf.InitContext(ctx)
	upf.association = a
}

func (upf *Upf) Rules(ueIp netip.Addr) *Pfcprules {
	rules, ok := upf.sessions[ueIp]
	if !ok {
		rules = NewPfcpRules()
		upf.sessions[ueIp] = rules
	}
	return rules
}

type fteidErr struct {
	Fteid *jsonapi.Fteid
	Err   error
}

func (upf *Upf) nextListenFteid(listenInterface netip.Addr) <-chan fteidErr {
	ch := make(chan fteidErr)
	ctx := upf.Context()
	select {
	case <-ctx.Done():
		ch <- fteidErr{nil, ctx.Err()}
		close(ch)
	default:
		go func(ctx context.Context, listenInterface netip.Addr, c chan<- fteidErr) {
			defer close(c)
			iface, ok := upf.interfaces[listenInterface]
			if !ok {
				c <- fteidErr{nil, ErrInterfaceNotFound}
				return
			}
			teid, err := iface.Teids.Next(ctx)
			if err != nil {
				c <- fteidErr{nil, err}
				return
			}
			c <- fteidErr{&jsonapi.Fteid{
				Addr: listenInterface,
				Teid: teid,
			}, nil}
		}(ctx, listenInterface, ch)
	}
	return ch
}

func (upf *Upf) CreateUplinkIntermediate(ctx context.Context, ueIp netip.Addr, dnn string, listenInterface netip.Addr, forwardFteid *jsonapi.Fteid) (*jsonapi.Fteid, error) {
	if ctx == nil {
		panic("nil context")
	}
	upfCtx := upf.Context()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-upfCtx.Done():
		return nil, upfCtx.Err()
	case res := <-upf.nextListenFteid(listenInterface):
		if res.Err != nil {
			return nil, res.Err
		}
		listenFteid := res.Fteid
		upf.CreateUplinkIntermediateWithFteid(ueIp, dnn, listenFteid, forwardFteid)
		return listenFteid, nil
	}
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

func (upf *Upf) CreateUplinkAnchor(ctx context.Context, ueIp netip.Addr, dnn string, listenInterface netip.Addr) (*jsonapi.Fteid, error) {
	if ctx == nil {
		panic("nil context")
	}
	ctxUpf := upf.Context()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ctxUpf.Done():
		return nil, ctxUpf.Err()
	case res := <-upf.nextListenFteid(listenInterface):
		if res.Err != nil {
			return nil, res.Err
		}
		listenFteid := res.Fteid
		upf.CreateUplinkAnchorWithFteid(ueIp, dnn, listenFteid)
		return listenFteid, nil
	}
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

func (upf *Upf) UpdateDownlinkIntermediate(ctx context.Context, ueIp netip.Addr, dnn string, listenInterface netip.Addr, forwardFteid *jsonapi.Fteid) (*jsonapi.Fteid, uint32, error) {
	if ctx == nil {
		panic("nil context")
	}
	ctxUpf := upf.Context()
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	case <-ctxUpf.Done():
		return nil, 0, ctxUpf.Err()
	case res := <-upf.nextListenFteid(listenInterface):
		if res.Err != nil {
			return nil, 0, res.Err
		}
		listenFteid := res.Fteid
		return listenFteid, upf.UpdateDownlinkIntermediateWithFteid(ueIp, dnn, listenFteid, forwardFteid), nil
	}
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
