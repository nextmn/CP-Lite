// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/nextmn/cp-lite/internal/config"

	"github.com/nextmn/json-api/jsonapi"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

// TODO: move to jsonapi
type PduSessionEstabReqMsg struct {
	Ue  jsonapi.ControlURI `json:"ue"`
	Gnb jsonapi.ControlURI `json:"gnb"`
	Dnn string             `json:"dnn"`
}

// TODO: move to jsonapi
type PduSessionEstabAcceptMsg struct {
	Header PduSessionEstabReqMsg `json:"header"`
	Addr   netip.Addr            `json:"address"`
}

// TODO: move to jsonapi
type N2PduSessionReqMsg struct {
	Cp         jsonapi.ControlURI       `json:"cp"`
	UeInfo     PduSessionEstabAcceptMsg `json:"ue-info"`
	Upf        netip.Addr               `json:"upf"`
	UplinkTeid uint32                   `json:"uplink-teid"`
}

// TODO: move to jsonapi
type N2PduSessionRespMsg struct {
	UeInfo       PduSessionEstabAcceptMsg `json:"ue-info"`
	DownlinkTeid uint32                   `json:"downlink-teid"`
	Gnb          netip.Addr               `json:"gnb"`
}

type Pool struct {
	pool    netip.Prefix
	current netip.Addr
}

func NewPool(pool netip.Prefix) *Pool {
	return &Pool{
		pool:    pool,
		current: pool.Addr(),
	}
}

func (p *Pool) Next() (netip.Addr, error) {
	addr := p.current.Next()
	p.current = addr
	if !p.pool.Contains(addr) {
		return addr, fmt.Errorf("out of range")
	}
	return addr, nil
}

type PduSessions struct {
	PduSessionsMap sync.Map // key: UE 5G IP ; value: PduSession
	UpfMap         sync.Map // Upfipaddr : UpfTeids
	Client         http.Client
	Control        jsonapi.ControlURI
	UserAgent      string
	Slices         map[string]config.Slice
	Pools          map[string]*Pool
	pfcp           *PFCPServer
}

type PduSession struct {
	Upf          netip.Addr
	UplinkTeid   uint32
	Gnb          netip.Addr
	DownlinkTeid uint32
}

func NewPduSessions(control jsonapi.ControlURI, slices map[string]config.Slice, pfcp *PFCPServer, userAgent string) *PduSessions {
	pools := make(map[string]*Pool)
	for name, p := range slices {
		pools[name] = NewPool(p.Pool)
	}
	return &PduSessions{
		PduSessionsMap: sync.Map{},
		UpfMap:         sync.Map{},
		Client:         http.Client{},
		Control:        control,
		UserAgent:      userAgent,
		Slices:         slices,
		Pools:          pools,
		pfcp:           pfcp,
	}
}

type UpfTeids struct {
	Teids sync.Map // teid: ue 5G ipaddr
}

func (p *PduSessions) EstablishmentRequest(c *gin.Context) {
	var ps PduSessionEstabReqMsg
	if err := c.BindJSON(&ps); err != nil {
		logrus.WithError(err).Error("could not deserialize")
		c.JSON(http.StatusBadRequest, jsonapi.MessageWithError{Message: "could not deserialize", Error: err})
		return
	}
	logrus.WithFields(logrus.Fields{
		"ue":  ps.Ue.String(),
		"gnb": ps.Gnb.String(),
		"dnn": ps.Dnn,
	}).Info("New PDU Session establishment Request")

	// allocate new ue ip addr
	pool, ok := p.Pools[ps.Dnn]
	if !ok {
		logrus.WithFields(logrus.Fields{
			"dnn": ps.Dnn,
		}).Error("unknown pool")
		c.JSON(http.StatusInternalServerError, jsonapi.MessageWithError{Message: "unknown pool", Error: nil})
		return
	}
	UeIpAddr, err := pool.Next()
	if err != nil {
		logrus.WithError(err).Error("no address available in pool")
		c.JSON(http.StatusInternalServerError, jsonapi.MessageWithError{Message: "no address available in pool", Error: err})
		return
	}

	upf := p.Slices[ps.Dnn].Upfs[0]
	upfTeids := &UpfTeids{}
	l, ok := p.UpfMap.Load(upf)
	if !ok {
		p.UpfMap.Store(upf, upfTeids)
	} else {
		upfTeids = l.(*UpfTeids)
	}
	ctxTimeout, cancel := context.WithTimeout(c, 100*time.Millisecond)
	defer cancel()
	done := false
	var teid uint32 = 0
	for !done {
		select {
		case <-ctxTimeout.Done():
			logrus.Error("could not create uplink TEID")
			c.JSON(http.StatusInternalServerError, jsonapi.MessageWithError{Message: "could not create uplink TEID", Error: nil})
			return
		default:
			teid = rand.Uint32()
			if teid == 0 {
				break // bad luck :(
			}
			if _, loaded := upfTeids.Teids.LoadOrStore(teid, UeIpAddr); !loaded {
				done = true
				break
			}
		}
	}
	// allocate uplink teid
	pduSession := PduSession{
		Upf:        upf,
		UplinkTeid: teid,
	}

	p.PduSessionsMap.Store(UeIpAddr, pduSession)

	// send PseAccept to UE
	n2PsReq := N2PduSessionReqMsg{
		Cp: p.Control,
		UeInfo: PduSessionEstabAcceptMsg{
			Header: ps,
			Addr:   UeIpAddr,
		},
		Upf:        pduSession.Upf,
		UplinkTeid: pduSession.UplinkTeid,
	}
	reqBody, err := json.Marshal(n2PsReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, jsonapi.MessageWithError{Message: "could not marshal json", Error: err})
		return
	}
	req, err := http.NewRequestWithContext(c, http.MethodPost, ps.Gnb.JoinPath("ps/n2-establishment-request").String(), bytes.NewBuffer(reqBody))
	if err != nil {
		c.JSON(http.StatusInternalServerError, jsonapi.MessageWithError{Message: "could not create request", Error: err})
		return
	}
	req.Header.Set("User-Agent", p.UserAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := p.Client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, jsonapi.MessageWithError{Message: "no http response", Error: err})
		return
	}
	defer resp.Body.Close()
}

func (p *PduSessions) N2EstablishmentResponse(c *gin.Context) {
	var ps N2PduSessionRespMsg
	if err := c.BindJSON(&ps); err != nil {
		logrus.WithError(err).Error("could not deserialize")
		c.JSON(http.StatusBadRequest, jsonapi.MessageWithError{Message: "could not deserialize", Error: err})
		return
	}
	pduSession, ok := p.PduSessionsMap.LoadAndDelete(ps.UeInfo.Addr)
	if !ok {
		logrus.Error("No PDU Session establishment procedure started for this UE")
		c.JSON(http.StatusInternalServerError, jsonapi.MessageWithError{Message: "no pdu session establishment procedure started for this UE", Error: nil})
		return
	}

	psStruct := pduSession.(PduSession)

	psStruct.DownlinkTeid = ps.DownlinkTeid
	psStruct.Gnb = ps.Gnb
	p.PduSessionsMap.Store(ps.UeInfo.Addr, psStruct)
	logrus.WithFields(logrus.Fields{
		"ue":                ps.UeInfo.Header.Ue.String(),
		"gnb":               ps.UeInfo.Header.Gnb.String(),
		"ip-addr":           ps.UeInfo.Addr,
		"gtp-upf":           psStruct.Upf,
		"gtp-uplink-teid":   psStruct.UplinkTeid,
		"gtp-downlink-teid": psStruct.DownlinkTeid,
		"gtp-gnb":           psStruct.Gnb,
		"dnn":               ps.UeInfo.Header.Dnn,
	}).Info("New PDU Session Established")

	err := p.pfcp.CreateSession(ps.UeInfo.Addr, psStruct.UplinkTeid, psStruct.DownlinkTeid, psStruct.Upf, psStruct.Gnb, ps.UeInfo.Header.Dnn)
	if err != nil {
		logrus.WithError(err).Error("Could not configure PDR/FAR in UPF")
		c.JSON(http.StatusInternalServerError, jsonapi.MessageWithError{Message: "could not configure PDR/FAR in UPF", Error: err})
		return
	}

}
