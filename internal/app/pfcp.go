// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package app

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"github.com/nextmn/cp-lite/internal/config"

	pfcp "github.com/nextmn/go-pfcp-networking/pfcp"
	pfcpapi "github.com/nextmn/go-pfcp-networking/pfcp/api"

	"github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
)

type PFCPServer struct {
	srv          *pfcp.PFCPEntityCP
	slices       map[string]config.Slice
	associations map[netip.Addr]pfcpapi.PFCPAssociationInterface
}

func NewPFCPServer(addr netip.Addr, slices map[string]config.Slice) *PFCPServer {
	return &PFCPServer{
		srv:          pfcp.NewPFCPEntityCP(addr.String(), addr.String()),
		slices:       slices,
		associations: make(map[netip.Addr]pfcpapi.PFCPAssociationInterface),
	}
}

func (p *PFCPServer) Start(ctx context.Context) error {
	logrus.Info("PFCP Server started")
	go func() {
		err := p.srv.ListenAndServeContext(ctx)
		if err != nil {
			logrus.WithError(err).Trace("PFCP server stopped")
		}
	}()
	ctxTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	done := false
	for !done {
		select {
		case <-time.After(10 * time.Millisecond): //FIXME: this should not be required
			if p.srv.RecoveryTimeStamp() != nil {
				done = true
				break
			}
		case <-ctxTimeout.Done():
			return ctx.Err()

		}
	}
	for _, slice := range p.slices {
		for _, upf := range slice.Upfs {
			association, err := p.srv.NewEstablishedPFCPAssociation(ie.NewNodeIDHeuristic(upf.NodeID.String()))
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"upf": upf.NodeID,
				}).Error("Could not perform PFCP association")
				return err
			}
			p.associations[upf.NodeID] = association
		}
	}
	logrus.Info("PFCP Associations complete")
	return nil
}

func (p *PFCPServer) CreateSession(ue netip.Addr, uplinkTeid uint32, downlinkTeid uint32, upfI netip.Addr, upfIn3 netip.Addr, gNB netip.Addr, slice string) error {
	a, ok := p.associations[upfI]
	if !ok {
		return fmt.Errorf("Could not create PFCP Session: not associated with UPF")
	}
	// TODO: don't hardcode pdr/far ids
	pdrIes := []*ie.IE{
		// uplink
		ie.NewCreatePDR(ie.NewPDRID(1), ie.NewPrecedence(255),
			ie.NewPDI(
				ie.NewSourceInterface(ie.SrcInterfaceAccess),
				ie.NewFTEID(0x01, uplinkTeid, upfIn3.AsSlice(), nil, 0), // ipv4: 0x01
				ie.NewNetworkInstance(slice),
				ie.NewUEIPAddress(0x02, ue.String(), "", 0, 0), // ipv4: 0x02
			),
			ie.NewOuterHeaderRemoval(0x00, 0), // remove gtp-u/udp/ipv4: 0x00
			ie.NewFARID(1),
		),
		// downlink
		ie.NewCreatePDR(ie.NewPDRID(2), ie.NewPrecedence(255),
			ie.NewPDI(ie.NewSourceInterface(ie.SrcInterfaceCore),
				ie.NewNetworkInstance(slice),
				ie.NewUEIPAddress(0x02, ue.String(), "", 0, 0), // ipv4: 0x02
			),
			ie.NewFARID(2),
		),
	}
	farIes := []*ie.IE{
		// uplink
		ie.NewCreateFAR(ie.NewFARID(1),
			ie.NewApplyAction(0x02), // FORW
			ie.NewForwardingParameters(
				ie.NewDestinationInterface(ie.DstInterfaceCore),
				ie.NewNetworkInstance(slice),
			),
		),
		// downlink
		ie.NewCreateFAR(ie.NewFARID(2),
			ie.NewApplyAction(0x02), // FORW
			ie.NewForwardingParameters(
				ie.NewDestinationInterface(ie.DstInterfaceAccess),
				ie.NewNetworkInstance(slice),
				ie.NewOuterHeaderCreation(
					0x0100, // GTP/UDP/IPv4
					downlinkTeid,
					gNB.String(),
					"", 0, 0, 0,
				),
			),
		),
	}
	pdrs, err, _, _ := pfcp.NewPDRMap(pdrIes)
	if err != nil {
		return err
	}
	fars, err, _, _ := pfcp.NewFARMap(farIes)
	if err != nil {
		return err
	}
	_, err = a.CreateSession(nil, pdrs, fars)
	if err != nil {
		return err
	}
	return nil
}
