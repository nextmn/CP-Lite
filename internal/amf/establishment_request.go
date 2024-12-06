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

	pduSession, err := amf.smf.CreateSessionUplink(c, ps.Ue, ps.Gnb, ps.Dnn)
	if err != nil {
		c.JSON(http.StatusInternalServerError, jsonapi.MessageWithError{Message: "could not create pdu session uplink", Error: err})
		return
	}

	// send PseAccept to UE
	n2PsReq := n1n2.N2PduSessionReqMsg{
		Cp: amf.control,
		UeInfo: n1n2.PduSessionEstabAcceptMsg{
			Header: ps,
			Addr:   pduSession.UeIpAddr,
		},
		Upf:        pduSession.UplinkFteid.Addr,
		UplinkTeid: pduSession.UplinkFteid.Teid,
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
	req.Header.Set("User-Agent", amf.userAgent)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	resp, err := amf.client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, jsonapi.MessageWithError{Message: "no http response", Error: err})
		return
	}
	defer resp.Body.Close()
}
