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
// 4. Release rules for the old DL path (to source gNB)
func (amf *Amf) HandleHandoverNotify(m n1n2.HandoverNotify) {
	ctx := amf.Context()
	for _, session := range m.Sessions {
		// Note: for the moment, we are only doing direct forwarding (step 2)
		if err := amf.smf.UpdateSessionDownlinkContext(ctx, m.UeCtrl, session.Addr, session.Dnn, m.SourceGnb); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"ue":          m.UeCtrl.String(),
				"pdu-session": session.Addr,
				"dnn":         session.Dnn,
				"gnb-source":  m.SourceGnb,
			}).Error("Handover Notify: could not update session downlink path")
		}

	}
}
