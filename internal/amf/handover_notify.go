// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package amf

import (
	"net/http"

	"github.com/nextmn/json-api/jsonapi"
	"github.com/nextmn/json-api/jsonapi/n1n2"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

func (amf *Amf) HandoverNotify(c *gin.Context) {
	var m n1n2.HandoverNotify
	if err := c.BindJSON(&m); err != nil {
		logrus.WithError(err).Error("could not deserialize")
		c.JSON(http.StatusBadRequest, jsonapi.MessageWithError{Message: "could not deserialize", Error: err})
		return
	}
	logrus.WithFields(logrus.Fields{
		"ue":         m.UeCtrl.String(),
		"gnb-target": m.TargetGnb.String(),
		"gbn-source": m.SourceGnb.String(),
	}).Info("New Handover Confirm")
	go amf.HandleHandoverNotify(m)
	c.JSON(http.StatusAccepted, jsonapi.Message{Message: "please refer to logs for more information"})
}

// Handover Notify is send by the target gNB to the Control Plane.
// Upon the reception of Handover Notify, the Control Plane may:
// 1. update DL rule in the UPF-i if direct forwarding was used
// 2. create new DL rules if sourceArea != targetArea
// 3. release old DL rules if sourceArea != targetArea
// 4. release rules for the old UL path (from source upf-i to source upf-a) if target area != source area:
// 5. release forwarding DL rule in UPF-i if sourceArea != targetArea
func (amf *Amf) HandleHandoverNotify(m n1n2.HandoverNotify) {
	ctx := amf.Context()
	sourceArea, ok := amf.smf.Areas.Area(m.SourceGnb)
	if !ok {
		logrus.WithFields(logrus.Fields{
			"source-gnb": m.SourceGnb,
		}).Error("Unknown Area for source gNB")
		return
	}
	targetArea, ok := amf.smf.Areas.Area(m.TargetGnb)
	if !ok {
		logrus.WithFields(logrus.Fields{
			"target-gnb": m.TargetGnb,
		}).Error("Unknown Area for target gNB")
		return
	}
	for _, s := range m.Sessions {
		indirectForwardingRequired, err := amf.smf.GetSessionIndirectForwardingRequired(m.UeCtrl, s.Addr, s.Dnn)
		if err != nil {
			// TODO: notify of failure
			continue
		}
		// step 1: update DL rule (only update FAR) in the UPF-i if direct forwarding was used
		if !indirectForwardingRequired {
			if err := amf.smf.UpdateSessionDownlinkContext(ctx, m.UeCtrl, s.Addr, s.Dnn, m.SourceGnb); err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"ue":          m.UeCtrl.String(),
					"pdu-session": s.Addr,
					"dnn":         s.Dnn,
					"gnb-source":  m.SourceGnb,
				}).Error("Handover Notify: could not update session downlink path")
			}
		}
		if sourceArea != targetArea {
			// step 2. create new DL rules if sourceArea != targetArea
			nextDlFteid, err := amf.smf.GetNextDownlinkFteid(m.UeCtrl, s.Addr, s.Dnn)
			if err != nil {
				// TODO: notify of failure
			}
			_, err = amf.smf.CreateSessionDownlinkContext(ctx, m.UeCtrl, s.Addr, s.Dnn, m.TargetGnb, *nextDlFteid)
			if err != nil {
				// TODO: notify of failure
				continue
			}

			// step 3. TODO: release old DL rules if sourceArea != targetArea
			// NOTE: until this step is implemented, handover across areas will **NOT** work when sourceUPFA == targetUPFA
			// step 4. TODO: release rules for the old UL path (from source upf-i to source upf-a) if target area != source area:
			// step 5. TODO: release forwarding DL rule in UPF-i if sourceArea != targetArea
		}
		if indirectForwardingRequired {
			amf.smf.SetSessionIndirectForwardingRequired(m.UeCtrl, s.Addr, s.Dnn, false)
		}

	}
}
