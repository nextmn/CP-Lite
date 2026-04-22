// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package sr4mec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/nextmn/cp-lite/internal/config"

	"github.com/nextmn/json-api/jsonapi"
	"github.com/nextmn/json-api/jsonapi/n4tosrv6"
	"github.com/nextmn/rfc9433/encoding"
)

type Ctrl struct {
	client    http.Client
	userAgent string

	sessions    map[netip.Addr]*PduSessionN3
	currentTeid uint32 // dumb shared counter
	slicesSR    map[config.SliceName]config.SliceSR
}

func NewCtrl(bindAddr netip.AddrPort, userAgent string, slicesSR map[config.SliceName]config.SliceSR) *Ctrl {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{
		// Force using "rest" interface IP Address
		LocalAddr: &net.TCPAddr{IP: bindAddr.Addr().AsSlice()},
		// Same parameters as http.DefaultTransport's Dialer
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext

	return &Ctrl{
		client:      http.Client{Transport: t},
		userAgent:   userAgent,
		currentTeid: 1,
		slicesSR:    slicesSR,
		sessions:    make(map[netip.Addr]*PduSessionN3),
	}
}

func (c *Ctrl) CreateSessionUplink(ctx context.Context, ueCtrl jsonapi.ControlURI, ueIpAddr netip.Addr, gnbCtrl jsonapi.ControlURI, dnn config.SliceName) (*PduSessionN3, error) {
	teid := c.currentTeid
	c.currentTeid++
	conf, ok := c.slicesSR[dnn]
	if !ok {
		return nil, fmt.Errorf("No SR config for slice %s", dnn)
	}
	session := &PduSessionN3{
		UeIpAddr:    ueIpAddr,
		UplinkFteid: &jsonapi.Fteid{Addr: conf.PsEstablishment.SrgwGtp4, Teid: teid},
	}
	c.sessions[ueIpAddr] = session // store it for later

	srh, err := n4tosrv6.NewSRH(conf.PsEstablishment.UplinkSegments)
	if err != nil {
		return nil, err
	}
	rule := n4tosrv6.Rule{
		Enabled: true,
		Type:    "uplink",
		Match: n4tosrv6.Match{
			Header: &n4tosrv6.GtpHeader{
				OuterIpSrc: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}, // we don't care
				FTeid:      *session.UplinkFteid,
				InnerIpSrc: &ueIpAddr,
			},
			Payload: &n4tosrv6.Payload{
				Dst: conf.Service,
			},
		},
		Action: n4tosrv6.Action{
			SRH: *srh,
		},
	}
	data, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, conf.PsEstablishment.Srgw.JoinPath("rules").String(), bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Add("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/json; charget=UTF-8")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 201 {
		loc := resp.Header.Get("Location")
		uloc, err := url.Parse(loc)
		if err != nil {
			return nil, err
		}
		session.UplinkRule = uloc
		return session, nil

	}

	return nil, fmt.Errorf("missing Location in response")
}

func (c *Ctrl) CreateSessionDownlink(ctx context.Context, ueCtrl jsonapi.ControlURI, ueIp netip.Addr, dnn config.SliceName, gnbCtrl jsonapi.ControlURI, gnbFteid jsonapi.Fteid, precedence uint32) (*PduSessionN3, error) {
	conf, ok := c.slicesSR[dnn]
	if !ok {
		return nil, fmt.Errorf("No SR config for slice %s", dnn)
	}
	// check for existing session
	session, ok := c.sessions[ueIp]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	session.DownlinkFteid = &gnbFteid
	seglist := make([]string, len(conf.PsEstablishment.DownlinkSegments))
	copy(seglist, conf.PsEstablishment.DownlinkSegments)
	prefix, err := netip.ParsePrefix(conf.PsEstablishment.DownlinkSegments[0])
	if err != nil {
		return nil, err
	}
	dst := encoding.NewMGTP4IPv6Dst(prefix, gnbFteid.Addr.As4(), encoding.NewArgsMobSession(0, false, false, gnbFteid.Teid))
	dstB, err := dst.Marshal()
	if err != nil {
		return nil, err
	}
	dstIp, ok := netip.AddrFromSlice(dstB)
	if !ok {
		return nil, fmt.Errorf("cannot marshal")
	}
	seglist[0] = dstIp.String()
	srh, err := n4tosrv6.NewSRH(seglist)
	if err != nil {
		return nil, err
	}
	rule := n4tosrv6.Rule{
		Enabled: true,
		Type:    "downlink",
		Match: n4tosrv6.Match{
			Payload: &n4tosrv6.Payload{
				Dst: ueIp,
			},
		},
		Action: n4tosrv6.Action{
			SRH: *srh,
		},
	}
	data, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, conf.PsEstablishment.Anchor.JoinPath("rules").String(), bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Add("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/json; charget=UTF-8")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 201 {
		loc := resp.Header.Get("Location")
		uloc, err := url.Parse(loc)
		if err != nil {
			return nil, err
		}
		session.UplinkRule = uloc
		return session, nil

	}
	return session, nil
}
