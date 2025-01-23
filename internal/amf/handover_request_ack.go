// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package amf

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/nextmn/json-api/jsonapi"
	"github.com/nextmn/json-api/jsonapi/n1n2"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func (amf *Amf) HandoverRequestAck(c *gin.Context) {
	var m n1n2.HandoverRequestAck
	if err := c.BindJSON(&m); err != nil {
		logrus.WithError(err).Error("could not deserialize")
		c.JSON(http.StatusBadRequest, jsonapi.MessageWithError{Message: "could not deserialize", Error: err})
		return
	}
	logrus.WithFields(logrus.Fields{
		"ue":         m.UeCtrl.String(),
		"gnb-source": m.SourcegNB.String(),
		"gnb-target": m.TargetgNB.String(),
	}).Info("New Handover Request Ack")
	go amf.HandleHandoverRequestAck(m)
	c.JSON(http.StatusAccepted, jsonapi.Message{Message: "please refer to logs for more information"})
}

// Handover Request Ack is send by the target gNB to the Control Plane.
// Upon reception of Handover Request Ack, the Control Plane:
// 1. if indirect forwarding is used: configure UPF-i with a DL rule to target gNB (existing DL rule to source gNB is preserved until Handover Notify reception)
// 2. send Handover Command to source gNB
func (amf *Amf) HandleHandoverRequestAck(m n1n2.HandoverRequestAck) {
	ctx := amf.Context()
	// TODO: if UPF-i change, push new DL rules

	sourceArea, ok := amf.smf.Areas.Area(m.SourcegNB)
	if !ok {
		logrus.WithFields(logrus.Fields{
			"source-gnb": m.SourcegNB,
		}).Error("Unknown Area for source gNB")
		return
	}
	targetArea, ok := amf.smf.Areas.Area(m.TargetgNB)
	if !ok {
		logrus.WithFields(logrus.Fields{
			"target-gnb": m.TargetgNB,
		}).Error("Unknown Area for target gNB")
		return
	}

	// send Handover Command to source gNB with "forwarding rule to targetGNB" (direct forwarding)
	sessions := make([]n1n2.Session, len(m.Sessions))
	for i, s := range m.Sessions {
		indirectForwardingRequired, err := amf.smf.GetSessionIndirectForwardingRequired(m.UeCtrl, s.Addr, s.Dnn)
		if err != nil {
			// TODO: notify of failure
			continue
		}
		if indirectForwardingRequired {
			dl, err := amf.smf.GetSessionDownlinkFteid(m.UeCtrl, s.Addr, s.Dnn)
			if err != nil {
				// TODO: notify of failure
				continue
			}
			upfiFwTarget, err := amf.smf.SessionFirstUpf(m.UeCtrl, s.Addr, s.Dnn, m.TargetgNB)
			if err != nil {
				// TODO: notify failure
				continue
			}
			upfiFwSource, err := amf.smf.SessionFirstUpf(m.UeCtrl, s.Addr, s.Dnn, m.TargetgNB)
			if err != nil {
				// TODO: notify failure
				continue
			}
			if s.DownlinkFteid == nil {
				// TODO: notify failure
				continue
			}
			// store DownlinkFteid to update the DL path upon reception of Handover Notfify
			if err := amf.smf.StoreNextDownlinkFteid(m.UeCtrl, s.Addr, s.Dnn, s.DownlinkFteid); err != nil {
				// TODO: notify of failure
				continue
			}
			// push new (temporary) DL rule on target UPF-i only (FAR: to target gNB) [DL-TI]
			fwFteidTarget, err := amf.smf.CreateSessionDownlinkFWUpfIContext(ctx, m.UeCtrl, s.Addr, s.Dnn, upfiFwTarget, *s.DownlinkFteid)
			if err != nil {
				// TODO: notify failure
				continue
			}
			if sourceArea != targetArea {
				// push (temporary) forwarding rule on source UPF-i only (FAR: to <DL-TI>))
				fwFteidSource, err := amf.smf.CreateSessionDownlinkFWUpfIContext(ctx, m.UeCtrl, s.Addr, s.Dnn, upfiFwSource, *fwFteidTarget)
				if err != nil {
					// TODO: notify failure
					continue
				}
				sessions[i] = n1n2.Session{
					Addr:                 s.Addr,
					Dnn:                  s.Dnn,
					UplinkFteid:          s.UplinkFteid,
					DownlinkFteid:        dl,
					ForwardDownlinkFteid: fwFteidSource,
				}
			} else {
				sessions[i] = n1n2.Session{
					Addr:                 s.Addr,
					Dnn:                  s.Dnn,
					UplinkFteid:          s.UplinkFteid,
					DownlinkFteid:        dl,
					ForwardDownlinkFteid: fwFteidTarget,
				}
			}
		} else {
			// direct forwarding: no modification of UPF-i: forward directly to target gNB
			dl, err := amf.smf.GetSessionDownlinkFteid(m.UeCtrl, s.Addr, s.Dnn)
			if err != nil {
				// TODO: notify of failure
				continue
			}
			sessions[i] = n1n2.Session{
				Addr:                 s.Addr,
				Dnn:                  s.Dnn,
				UplinkFteid:          s.UplinkFteid,
				DownlinkFteid:        dl,
				ForwardDownlinkFteid: s.DownlinkFteid,
			}
			// we store the DL FTEID: upon reception of Handover Notify, UPF-i will be updated to use it
			if err := amf.smf.StoreNextDownlinkFteid(m.UeCtrl, s.Addr, s.Dnn, s.DownlinkFteid); err != nil {
				// TODO: notify of failure
				continue
			}
		}
	}

	// forward to UE
	resp := n1n2.HandoverCommand{
		Cp:        m.Cp,
		TargetGnb: m.TargetgNB,
		SourceGnb: m.SourcegNB,
		UeCtrl:    m.UeCtrl,
		Sessions:  sessions,
	}

	reqBody, err := json.Marshal(resp)
	if err != nil {
		logrus.WithError(err).Error("Could not marshal n1n2.HandoverRequest")
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.SourcegNB.JoinPath("ps/handover-command").String(), bytes.NewBuffer(reqBody))
	if err != nil {
		logrus.WithError(err).Error("Could not create request for ps/handover-command")
		return
	}
	req.Header.Set("User-Agent", amf.userAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	if _, err := amf.client.Do(req); err != nil {
		logrus.WithError(err).Error("Could not send ps/handover-command")
	}
}
