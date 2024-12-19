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

func (amf *Amf) EstablishmentRequest(c *gin.Context) {
	var ps n1n2.PduSessionEstabReqMsg
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
	go amf.HandleEstablishmentRequest(ps)
	c.JSON(http.StatusAccepted, jsonapi.Message{Message: "please refer to logs for more information"})
}

func (amf *Amf) HandleEstablishmentRequest(ps n1n2.PduSessionEstabReqMsg) {
	ctx := amf.Context()
	// TODO: use ctx.WithTimeout()
	pduSession, err := amf.smf.CreateSessionUplinkContext(ctx, ps.Ue, ps.Gnb, ps.Dnn)
	if err != nil {
		logrus.WithError(err).Error("Could not create PDU Session Uplink")
	}

	// send PseAccept to UE
	n2PsReq := n1n2.N2PduSessionReqMsg{
		Cp: amf.control,
		UeInfo: n1n2.PduSessionEstabAcceptMsg{
			Header: ps,
			Addr:   pduSession.UeIpAddr,
		},
		UplinkFteid: *pduSession.UplinkFteid,
	}
	reqBody, err := json.Marshal(n2PsReq)
	if err != nil {
		logrus.WithError(err).Error("Could not marshal n1n2.N2PduSessionReqMsg")
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ps.Gnb.JoinPath("ps/n2-establishment-request").String(), bytes.NewBuffer(reqBody))
	if err != nil {
		logrus.WithError(err).Error("Could not create request for ps/n2-establishment-request")
		return
	}
	req.Header.Set("User-Agent", amf.userAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	if _, err := amf.client.Do(req); err != nil {
		logrus.WithError(err).Error("Could not send ps/n2-establishment-request")
	}
}
