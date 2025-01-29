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

func (amf *Amf) HandoverRequired(c *gin.Context) {
	var m n1n2.HandoverRequired
	if err := c.BindJSON(&m); err != nil {
		logrus.WithError(err).Error("could not deserialize")
		c.JSON(http.StatusBadRequest, jsonapi.MessageWithError{Message: "could not deserialize", Error: err})
		return
	}
	logrus.WithFields(logrus.Fields{
		"ue":         m.Ue.String(),
		"gnb-source": m.SourcegNB.String(),
		"gnb-target": m.TargetgNB.String(),
	}).Info("New Handover Required")
	go amf.HandleHandoverRequired(m)
	c.JSON(http.StatusAccepted, jsonapi.Message{Message: "please refer to logs for more information"})
}

// Handover Required is send by the source gNB to the Control Plane.
// Upon reception of Handover Required, the Control Plane
// 1. configure new UL path for each session
// 2. send an Handover Request to the target gNB with the configured UL FTEIDs
func (amf *Amf) HandleHandoverRequired(m n1n2.HandoverRequired) {
	ctx := amf.Context()

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

	// send handover-request to target with UPF-i FTEID
	sessions := make([]n1n2.Session, len(m.Sessions))
	for i, s := range m.Sessions {
		// store type of forwarding for later
		if m.IndirectForwarding {
			if err := amf.smf.SetSessionIndirectForwardingRequired(m.Ue, s.Addr, s.Dnn, true); err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"ue":      m.Ue,
					"ue-addr": s.Addr,
					"dnn":     s.Dnn,
				}).Error("Could not set Indirect Forwarding Required for handover")
				continue
			}
		}
		if sourceArea != targetArea {
			// we could recycle common UL rules, but this is harder than simply
			// create the target path (and delete the source path at the end of the handover)
			pduSessionN3, err := amf.smf.CreateSessionUplinkContext(ctx, m.Ue, s.Addr, m.TargetgNB, s.Dnn)
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"ue":         m.Ue,
					"ue-addr":    s.Addr,
					"dnn":        s.Dnn,
					"target-gnb": m.TargetgNB,
				}).Error("Could not establish new uplink path")
				continue
			}
			sessions[i] = n1n2.Session{
				Addr:        s.Addr,
				Dnn:         s.Dnn,
				UplinkFteid: pduSessionN3.UplinkFteid,
			}
		} else {
			// fully reuse existing path
			uplinkfteid, err := amf.smf.GetSessionUplinkFteid(m.Ue, s.Addr, s.Dnn)
			if err != nil {
				// TODO: notify gnb of failure
				logrus.WithError(err).WithFields(logrus.Fields{
					"ue":      m.Ue,
					"ue-addr": s.Addr,
					"dnn":     s.Dnn,
				}).Error("Could not find Uplink FTEID for handover")
				continue
			}
			sessions[i] = n1n2.Session{
				Addr:        s.Addr,
				Dnn:         s.Dnn,
				UplinkFteid: uplinkfteid,
			}
		}

	}
	// send PseAccept to UE
	resp := n1n2.HandoverRequest{
		// Header
		UeCtrl:    m.Ue,
		Cp:        m.Cp,
		TargetgNB: m.TargetgNB,

		// Handover Request
		SourcegNB: m.SourcegNB,
		Sessions:  sessions,
	}
	reqBody, err := json.Marshal(resp)
	if err != nil {
		logrus.WithError(err).Error("Could not marshal n1n2.HandoverRequest")
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.TargetgNB.JoinPath("ps/handover-request").String(), bytes.NewBuffer(reqBody))
	if err != nil {
		logrus.WithError(err).Error("Could not create request for ps/handover-request")
		return
	}
	req.Header.Set("User-Agent", amf.userAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	if _, err := amf.client.Do(req); err != nil {
		logrus.WithError(err).Error("Could not send ps/handover-request")
	}
}
