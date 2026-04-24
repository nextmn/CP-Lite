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

	"github.com/sirupsen/logrus"
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

func (c *Ctrl) pushSingleRule(ctx context.Context, uri jsonapi.ControlURI, data []byte) (*url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri.JoinPath("rules").String(), bytes.NewBuffer(data))
	if err != nil {
		logrus.WithError(err).Error("could not create http request")
		return nil, err
	}
	req.Header.Add("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := c.client.Do(req)
	if err != nil {
		logrus.WithError(err).Error("Could not push rules: server not responding")
		return nil, fmt.Errorf("could not push rules: server not responding")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 400 {
		logrus.WithError(err).Error("HTTP Bad Request")
		return nil, fmt.Errorf("HTTP Bad request")
	} else if resp.StatusCode >= 500 {
		logrus.WithError(err).Error("HTTP internal error")
		return nil, fmt.Errorf("HTTP internal error")
	} else if resp.StatusCode == 201 {
		loc := resp.Header.Get("Location")
		uloc, err := url.Parse(loc)
		if err != nil {
			return nil, err
		}
		return uri.ResolveReference(uloc), nil
	}
	return nil, fmt.Errorf("no Location provided")
}
func (c *Ctrl) pushUpdateAction(ctx context.Context, url *url.URL, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url.JoinPath("update-action").String(), bytes.NewBuffer(data))
	if err != nil {
		logrus.WithError(err).Error("could not create http request")
		return err
	}
	req.Header.Add("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := c.client.Do(req)
	if err != nil {
		logrus.WithError(err).Error("Could not push rules: server not responding")
		return fmt.Errorf("could not push rules: server not responding")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 400 {
		logrus.WithError(err).Error("HTTP Bad Request")
		return fmt.Errorf("HTTP Bad request")
	} else if resp.StatusCode >= 500 {
		logrus.WithError(err).Error("HTTP internal error")
		return fmt.Errorf("HTTP internal error")
	}
	return nil
}

// Create an uplink for a new PDU Session (at establishment)
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

	uloc, err := c.pushSingleRule(ctx, conf.PsEstablishment.Srgw, data)
	if err != nil {
		return nil, err
	}
	session.UplinkRule = uloc
	return session, nil
}

// Create a downlink for a new PDU Session (at establishment, after receiving N2 response)
func (c *Ctrl) CreateSessionDownlink(ctx context.Context, ueCtrl jsonapi.ControlURI, ueIp netip.Addr, dnn config.SliceName, gnbCtrl jsonapi.ControlURI, gnbFteid jsonapi.Fteid) (*PduSessionN3, error) {
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
			SourceGtp4: &conf.PsEstablishment.SrgwGtp4,
			SRH:        *srh,
		},
	}
	data, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}

	uloc, err := c.pushSingleRule(ctx, conf.PsEstablishment.Anchor, data)
	if err != nil {
		return nil, err
	}
	session.DownlinkRule = uloc
	return session, nil
}

// Create an uplink for after HO required
func (c *Ctrl) CreateNewUplinkExistingSession(ctx context.Context, ueCtrl jsonapi.ControlURI, ueIpAddr netip.Addr, gnbCtrl jsonapi.ControlURI, dnn config.SliceName) (*PduSessionN3, error) {
	teid := c.currentTeid
	c.currentTeid++
	conf, ok := c.slicesSR[dnn]
	if !ok {
		return nil, fmt.Errorf("No SR config for slice %s", dnn)
	}
	// check for existing session
	session, ok := c.sessions[ueIpAddr]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}

	session.TargetUplinkFteid = &jsonapi.Fteid{Addr: conf.HandoverMigration.SrgwGtp4, Teid: teid}

	var segs []string
	if conf.MigrationAPosteriori {
		// preserve instance
		segs = conf.PsEstablishment.UplinkSegments
	} else {
		// immediate instance update
		segs = conf.HandoverMigration.UplinkSegments
	}
	srh, err := n4tosrv6.NewSRH(segs)
	if err != nil {
		return nil, err
	}
	rule := n4tosrv6.Rule{
		Enabled: true,
		Type:    "uplink",
		Match: n4tosrv6.Match{
			Header: &n4tosrv6.GtpHeader{
				OuterIpSrc: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}, // we don't care
				FTeid:      *session.TargetUplinkFteid,
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

	uloc, err := c.pushSingleRule(ctx, conf.HandoverMigration.Srgw, data)
	if err != nil {
		return nil, err
	}
	session.TargetUplinkRule = uloc
	return session, nil
}

// Create an forwarding rule (when receive HO Request Ack), and a target DL rule if not doing migration a posteriori
func (c *Ctrl) CreateForwarding(ctx context.Context, ueCtrl jsonapi.ControlURI, ueIpAddr netip.Addr, gnbCtrl jsonapi.ControlURI, dnn config.SliceName, dlFteid *jsonapi.Fteid) (*PduSessionN3, error) {
	teid := c.currentTeid
	c.currentTeid++
	conf, ok := c.slicesSR[dnn]
	if !ok {
		return nil, fmt.Errorf("No SR config for slice %s", dnn)
	}
	// check for existing session
	session, ok := c.sessions[ueIpAddr]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}

	session.ForwardingFteid = &jsonapi.Fteid{Addr: conf.HandoverMigration.SrgwGtp4, Teid: teid}
	session.TargetDownlinkFteid = dlFteid

	seglist := make([]string, len(conf.HandoverMigration.DownlinkSegments))
	copy(seglist, conf.HandoverMigration.DownlinkSegments)
	prefix, err := netip.ParsePrefix(conf.HandoverMigration.DownlinkSegments[0])
	if err != nil {
		return nil, err
	}
	dst := encoding.NewMGTP4IPv6Dst(prefix, session.TargetDownlinkFteid.Addr.As4(), encoding.NewArgsMobSession(0, false, false, session.TargetDownlinkFteid.Teid))
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
		Type:    "uplink",
		Match: n4tosrv6.Match{
			Header: &n4tosrv6.GtpHeader{
				OuterIpSrc: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}, // we don't care
				FTeid:      *session.ForwardingFteid,
				InnerIpSrc: &conf.Service,
			},
			Payload: &n4tosrv6.Payload{
				Dst: ueIpAddr,
			},
		},
		Action: n4tosrv6.Action{
			// TODO: srv6 netfunc don't handle this for uplink rules, but we cannot use "downlink" which don't consider Match.Header data,
			// but this is okay because it's only to create source address on gtp message, which we don't really need (current gNB implementation don't check source address)
			SourceGtp4: &conf.HandoverMigration.SrgwGtp4,
			SRH:        *srh,
		},
	}
	data, err := json.Marshal(rule)
	if err != nil {
		return nil, err
	}

	uloc, err := c.pushSingleRule(ctx, conf.PsEstablishment.Srgw, data)
	if err != nil {
		return nil, err
	}
	session.ForwardingRule = uloc

	if !conf.MigrationAPosteriori {
		rule = n4tosrv6.Rule{
			Enabled: true,
			Type:    "downlink",
			Match: n4tosrv6.Match{
				Payload: &n4tosrv6.Payload{
					Dst: ueIpAddr,
				},
			},
			Action: n4tosrv6.Action{
				SourceGtp4: &conf.HandoverMigration.SrgwGtp4,
				SRH:        *srh,
			},
		}
		data, err = json.Marshal(rule)
		if err != nil {
			return nil, err
		}
		uloc, err = c.pushSingleRule(ctx, conf.HandoverMigration.Anchor, data)
		if err != nil {
			return nil, err
		}
		session.TargetDownlinkRule = uloc
	}

	return session, nil
}

// Create/switch downlink rule
func (c *Ctrl) CreateNewDownlinkExistingSession(ctx context.Context, ueCtrl jsonapi.ControlURI, ueIpAddr netip.Addr, dnn config.SliceName) error {
	conf, ok := c.slicesSR[dnn]
	if !ok {
		return fmt.Errorf("No SR config for slice %s", dnn)
	}
	// check for existing session
	session, ok := c.sessions[ueIpAddr]
	if !ok {
		return fmt.Errorf("session not found")
	}

	if !conf.MigrationAPosteriori {
		return nil // nothing to do except removing old rules (and I don't have the time to code this)
	}
	// update action in source anchor
	seglist := make([]string, len(conf.HandoverMigration.DownlinkSegments))
	copy(seglist, conf.HandoverMigration.DownlinkSegments)
	prefix, err := netip.ParsePrefix(conf.HandoverMigration.DownlinkSegments[0])
	if err != nil {
		return err
	}
	dst := encoding.NewMGTP4IPv6Dst(prefix, session.TargetDownlinkFteid.Addr.As4(), encoding.NewArgsMobSession(0, false, false, session.TargetDownlinkFteid.Teid))
	dstB, err := dst.Marshal()
	if err != nil {
		return err
	}
	dstIp, ok := netip.AddrFromSlice(dstB)
	if !ok {
		return fmt.Errorf("cannot marshal")
	}
	seglist[0] = dstIp.String()
	srh, err := n4tosrv6.NewSRH(seglist)
	if err != nil {
		return err
	}
	action := n4tosrv6.Action{
		SourceGtp4: &conf.HandoverMigration.SrgwGtp4,
		SRH:        *srh,
	}
	data, err := json.Marshal(action)
	if err != nil {
		return err
	}
	err = c.pushUpdateAction(ctx, session.DownlinkRule, data)
	if err != nil {
		return err
	}

	// migration after delay
	ctxDelayMigration, cancel := context.WithTimeout(ctx, conf.MigrationDelay)
	defer cancel()
	select {
	case <-ctxDelayMigration.Done():
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	// migration
	// 1. new downlink rule on target edge
	rule := n4tosrv6.Rule{
		Enabled: true,
		Type:    "downlink",
		Match: n4tosrv6.Match{
			Payload: &n4tosrv6.Payload{
				Dst: ueIpAddr,
			},
		},
		Action: action,
	}
	data, err = json.Marshal(rule)
	if err != nil {
		return err
	}
	uloc, err := c.pushSingleRule(ctx, conf.HandoverMigration.Anchor, data)
	if err != nil {
		return err
	}
	session.TargetDownlinkRule = uloc

	srh, err = n4tosrv6.NewSRH(conf.HandoverMigration.UplinkSegments)
	if err != nil {
		return err
	}
	// 2. update uplink action on target area
	action = n4tosrv6.Action{
		SourceGtp4: &conf.HandoverMigration.SrgwGtp4,
		SRH:        *srh,
	}
	data, err = json.Marshal(rule)
	if err != nil {
		return err
	}
	err = c.pushUpdateAction(ctx, session.TargetUplinkRule, data)
	if err != nil {
		return err
	}
	// 3. remove after a timer
	// ...
	// I don't have time to code this

	return nil
}
