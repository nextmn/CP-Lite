// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package smf

import (
	"context"
	"net/netip"
	"sync"

	"github.com/nextmn/cp-lite/internal/config"

	pfcp "github.com/nextmn/go-pfcp-networking/pfcp"
	pfcpapi "github.com/nextmn/go-pfcp-networking/pfcp/api"

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
	association pfcpapi.PFCPAssociationInterface
	interfaces  map[netip.Addr]*UpfInterface
	sessions    map[netip.Addr]*Pfcprules
}

func (upf *Upf) GetN3() (netip.Addr, error) {
	for addr, iface := range upf.interfaces {
		if iface.IsN3() {
			return addr, nil
		}
	}
	return netip.Addr{}, ErrInterfaceNotFound
}

func (upf *Upf) GetN9() (netip.Addr, error) {
	for addr, iface := range upf.interfaces {
		if iface.IsN9() {
			return addr, nil
		}
	}
	return netip.Addr{}, ErrInterfaceNotFound
}

func NewUpf(interfaces []config.Interface) *Upf {
	upf := Upf{
		interfaces: NewUpfInterfaceMap(interfaces),
		sessions:   make(map[netip.Addr]*Pfcprules),
	}
	return &upf
}

func (upf *Upf) Associate(a pfcpapi.PFCPAssociationInterface) {
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

func (upf *Upf) NextListenFteid(ctx context.Context, listenInterface netip.Addr) (*Fteid, error) {
	iface, ok := upf.interfaces[listenInterface]
	if !ok {
		return nil, ErrInterfaceNotFound
	}
	teid, err := iface.Teids.Next(ctx)
	if err != nil {
		return nil, err
	}
	return &Fteid{
		Addr: listenInterface,
		Teid: teid,
	}, nil
}

func (upf *Upf) CreateUplinkIntermediate(ctx context.Context, ueIp netip.Addr, dnn string, listenInterface netip.Addr, forwardFteid *Fteid) (*Fteid, error) {
	listenFteid, err := upf.NextListenFteid(ctx, listenInterface)
	if err != nil {
		return nil, err
	}
	upf.CreateUplinkIntermediateWithFteid(ueIp, dnn, listenFteid, forwardFteid)
	return listenFteid, nil
}

func (upf *Upf) CreateUplinkIntermediateWithFteid(ueIp netip.Addr, dnn string, listenFteid *Fteid, forwardFteid *Fteid) {
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
			ie.NewUEIPAddress(UEIpAddrTypeIPv4, ueIp.String(), "", 0, 0),
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
	// TODO: QER, to avoid wrong gtp size set by F5GC's UPF
}

func (upf *Upf) CreateUplinkAnchor(ctx context.Context, ueIp netip.Addr, dnn string, listenInterface netip.Addr) (*Fteid, error) {
	listenFteid, err := upf.NextListenFteid(ctx, listenInterface)
	if err != nil {
		return nil, err
	}
	upf.CreateUplinkAnchorWithFteid(ueIp, dnn, listenFteid)
	return listenFteid, nil
}

func (upf *Upf) CreateUplinkAnchorWithFteid(ueIp netip.Addr, dnn string, listenFteid *Fteid) {
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
			ie.NewUEIPAddress(UEIpAddrTypeIPv4, ueIp.String(), "", 0, 0),
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

func (upf *Upf) UpdateDownlinkAnchor(ueIp netip.Addr, dnn string, forwardFteid *Fteid) {
	r := upf.Rules(ueIp)
	r.Lock()
	defer r.Unlock()
	r.currentpdrid += 1
	r.currentfarid += 1

	r.createpdrs = append(r.createpdrs, ie.NewCreatePDR(ie.NewPDRID(r.currentpdrid), ie.NewPrecedence(255),
		ie.NewPDI(ie.NewSourceInterface(ie.SrcInterfaceCore),
			ie.NewNetworkInstance(dnn),
			ie.NewUEIPAddress(UEIpAddrTypeIPv4, ueIp.String(), "", 0, 0),
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
	// TODO: QER, to avoid wrong gtp size set by F5GC's UPF
}

func (upf *Upf) UpdateDownlinkIntermediate(ctx context.Context, ueIp netip.Addr, dnn string, listenInterface netip.Addr, forwardFteid *Fteid) (*Fteid, error) {
	listenFteid, err := upf.NextListenFteid(ctx, listenInterface)
	if err != nil {
		return nil, err
	}
	upf.UpdateDownlinkIntermediateWithFteid(ueIp, dnn, listenFteid, forwardFteid)
	return listenFteid, nil
}

func (upf *Upf) UpdateDownlinkIntermediateWithFteid(ueIp netip.Addr, dnn string, listenFteid *Fteid, forwardFteid *Fteid) {
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
			// Free5GC's UPF seems to drop GTP packets, when UE IP Address is present
			// and for downlink only
			// According to TS 129.244 version 17.4.0, section 7.5.2.2, table 7.5.2.2-2, p. 159:
			// "If present, this IE shall identify the UE IP address as
			// the source or destination IP address to match for the incoming packet"
			// Here, it seems that when the packet is GTP encapsulated, only the source address is checked.
			// This is a bug.
			// As a workaround, we don't add the UE IP Address in this case.
			// Free5GC's SMF implementation has a similar workaround:
			// https://github.com/free5gc/smf/blob/62925c4a5840f118b42f245b5fef492898f9f0bd/internal/context/datapath.go#L636-L640
			// and the bug seems to be referenced as "FR5GC-1029" (but I have no link to the details of the issue)
			// XXX: the following line may be uncommented if the bug is fixed:

			// ie.NewUEIPAddress(UEIpAddrTypeIPv4, ueIp.String(), "", 0, 0),
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
	// TODO: QER, to avoid wrong gtp size set by F5GC's UPF
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
	updatefars, err, _, _ := pfcp.NewFARMap(rules.updatefars)
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
