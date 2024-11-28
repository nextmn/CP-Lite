// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package app

import (
	"context"
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
		srv:    pfcp.NewPFCPEntityCP(addr.String(), addr.String()),
		slices: slices,
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
			association, err := p.srv.NewEstablishedPFCPAssociation(ie.NewNodeIDHeuristic(upf.String()))
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"upf": upf,
				}).Error("Could not perform PFCP association")
				return err
			}
			p.associations[upf] = association
		}
	}
	logrus.Info("PFCP Associations complete")
	return nil
}
