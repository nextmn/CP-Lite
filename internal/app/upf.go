// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package app

import (
	"context"
	"fmt"
	"net/netip"
	"sync"

	pfcp "github.com/nextmn/go-pfcp-networking/pfcp"
	pfcpapi "github.com/nextmn/go-pfcp-networking/pfcp/api"

	"github.com/wmnsk/go-pfcp/ie"
)

// PFCP Constants
const (
	FteidTypeIPv4                  = 0x01
	UEIpAddrTypeIPv4               = 0x02
	OuterHeaderRemoveGtpuUdpIpv4   = 0x00
	ApplyActionForw                = 0x02
	OuterHeaderCreationGtpuUdpIpv4 = 0x0100
)

type Fteid struct {
	Addr netip.Addr
	Teid uint32
}

type Upf struct {
	association pfcpapi.PFCPAssociationInterface
	interfaces  map[netip.Addr]*TEIDsPool
	sessions    map[netip.Addr]*pfcprules
}

func NewUpf(association pfcpapi.PFCPAssociationInterface, interfaces []netip.Addr) *Upf {
	upf := Upf{
		association: association,
		interfaces:  make(map[netip.Addr]*TEIDsPool),
		sessions:    make(map[netip.Addr]*pfcprules),
	}

	for _, i := range interfaces {
		upf.interfaces[i] = NewTEIDsPool()
	}

	return &upf
}

func (upf *Upf) Rules(ueIp netip.Addr) *pfcprules {
	rules, ok := upf.sessions[ueIp]
	if !ok {
		rules = newPfcpRules()
		upf.sessions[ueIp] = rules
	}
	return rules
}

type pfcprules struct {
	created      bool
	pdrs         []*ie.IE
	fars         []*ie.IE
	currentpdrid uint16
	currentfarid uint32

	sync.Mutex
}

func newPfcpRules() *pfcprules {
	return &pfcprules{
		pdrs: make([]*ie.IE, 0),
		fars: make([]*ie.IE, 0),
	}
}

func (upf *Upf) NextListenFteid(ctx context.Context, listenInterface netip.Addr) (*Fteid, error) {
	pool, ok := upf.interfaces[listenInterface]
	if !ok {
		return nil, fmt.Errorf("wrong interface")
	}
	teid, err := pool.Next(ctx)
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

	r.pdrs = append(r.pdrs, ie.NewCreatePDR(ie.NewPDRID(r.currentpdrid), ie.NewPrecedence(255),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceAccess),
			ie.NewFTEID(FteidTypeIPv4, listenFteid.Teid, listenFteid.Addr.AsSlice(), nil, 0),
			ie.NewNetworkInstance(dnn),
			ie.NewUEIPAddress(UEIpAddrTypeIPv4, ueIp.String(), "", 0, 0),
		),
		ie.NewOuterHeaderRemoval(OuterHeaderRemoveGtpuUdpIpv4, 0),
		ie.NewFARID(r.currentfarid),
	))
	r.fars = append(r.fars, ie.NewCreateFAR(ie.NewFARID(r.currentfarid),
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

	r.pdrs = append(r.pdrs, ie.NewCreatePDR(ie.NewPDRID(r.currentpdrid), ie.NewPrecedence(255),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceAccess),
			ie.NewFTEID(FteidTypeIPv4, listenFteid.Teid, listenFteid.Addr.AsSlice(), nil, 0),
			ie.NewNetworkInstance(dnn),
			ie.NewUEIPAddress(UEIpAddrTypeIPv4, ueIp.String(), "", 0, 0),
		),
		ie.NewOuterHeaderRemoval(OuterHeaderRemoveGtpuUdpIpv4, 0),
		ie.NewFARID(r.currentfarid),
	))
	r.fars = append(r.fars, ie.NewCreateFAR(ie.NewFARID(r.currentfarid),
		ie.NewApplyAction(ApplyActionForw),
		ie.NewForwardingParameters(
			ie.NewDestinationInterface(ie.DstInterfaceCore),
			ie.NewNetworkInstance(dnn),
		),
	))
}

func (upf *Upf) CreateDownlinkAnchor(ueIp netip.Addr, dnn string, forwardFteid *Fteid) {
	r := upf.Rules(ueIp)
	r.Lock()
	defer r.Unlock()
	r.currentpdrid += 1
	r.currentfarid += 1

	r.pdrs = append(r.pdrs, ie.NewCreatePDR(ie.NewPDRID(r.currentpdrid), ie.NewPrecedence(255),
		ie.NewPDI(ie.NewSourceInterface(ie.SrcInterfaceCore),
			ie.NewNetworkInstance(dnn),
			ie.NewUEIPAddress(UEIpAddrTypeIPv4, ueIp.String(), "", 0, 0),
		),
		ie.NewFARID(r.currentfarid),
	),
	)
	r.fars = append(r.fars, ie.NewCreateFAR(ie.NewFARID(r.currentfarid),
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
}

func (upf *Upf) CreateDownlinkIntermediate(ctx context.Context, ueIp netip.Addr, dnn string, listenInterface netip.Addr, forwardFteid *Fteid) (*Fteid, error) {
	listenFteid, err := upf.NextListenFteid(ctx, listenInterface)
	if err != nil {
		return nil, err
	}
	upf.CreateDownlinkIntermediateWithFteid(ueIp, dnn, listenFteid, forwardFteid)
	return listenFteid, nil
}

func (upf *Upf) CreateDownlinkIntermediateWithFteid(ueIp netip.Addr, dnn string, listenFteid *Fteid, forwardFteid *Fteid) {
	r := upf.Rules(ueIp)
	r.Lock()
	defer r.Unlock()
	r.currentpdrid += 1
	r.currentfarid += 1

	r.pdrs = append(r.pdrs, ie.NewCreatePDR(ie.NewPDRID(r.currentpdrid), ie.NewPrecedence(255),
		ie.NewPDI(
			ie.NewSourceInterface(ie.SrcInterfaceCore),
			ie.NewFTEID(FteidTypeIPv4, listenFteid.Teid, listenFteid.Addr.AsSlice(), nil, 0),
			ie.NewNetworkInstance(dnn),
			ie.NewUEIPAddress(UEIpAddrTypeIPv4, ueIp.String(), "", 0, 0),
		),
		ie.NewOuterHeaderRemoval(OuterHeaderRemoveGtpuUdpIpv4, 0),
		ie.NewFARID(r.currentfarid),
	),
	)
	r.fars = append(r.fars, ie.NewCreateFAR(ie.NewFARID(r.currentfarid),
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
}

func (upf *Upf) CreateSession(ue netip.Addr) error {
	rules, ok := upf.sessions[ue]
	if !ok {
		return fmt.Errorf("No rule for this UE")
	}
	rules.Lock()
	defer rules.Unlock()
	if rules.created {
		return fmt.Errorf("Session already created")
	}
	rules.created = true

	pdrs, err, _, _ := pfcp.NewPDRMap(rules.pdrs)
	if err != nil {
		return err
	}
	fars, err, _, _ := pfcp.NewFARMap(rules.fars)
	if err != nil {
		return err
	}
	_, err = upf.association.CreateSession(nil, pdrs, fars)
	if err != nil {
		return err
	}
	return nil
}
