// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
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
// 1. update DL rules if UPF-i have been updated during the handover
// 2. update DL rule in the UPF-i if direct forwarding was used
// 3. remove forwarding DL rule in UPF-i if indirect forwarding was used
// 4. release rules for the old DL path (from source upf-a to source gNB)
// 5. if target area != source area: release rules for the old UL path (from source upf-i to source upf-a)
func (amf *Amf) HandleHandoverNotify(m n1n2.HandoverNotify) {
	ctx := amf.Context()
	for _, s := range m.Sessions {
		indirectForwardingRequired, err := amf.smf.GetSessionIndirectForwardingRequired(m.UeCtrl, s.Addr, s.Dnn)
		if err != nil {
			// TODO: notify of failure
			continue
		}
		// step 1: TODO
		// step 2: update DL rule in the UPF-i if direct forwarding was used
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
		// step 3: TODO
		// step 4: TODO
		// step 5: TODO

	}
}
