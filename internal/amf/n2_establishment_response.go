// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package amf

import (
	"encoding/json/v2"
	"net/http"

	"github.com/nextmn/cp-lite/internal/config"

	"github.com/nextmn/json-api/jsonapi"
	"github.com/nextmn/json-api/jsonapi/n1n2"

	"github.com/sirupsen/logrus"
)

func (amf *Amf) N2EstablishmentResponse(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.Header().Set("Cache-Control", "no-cache")
	var ps n1n2.N2PduSessionRespMsg
	if err := json.UnmarshalRead(req.Body, &ps); err != nil {
		logrus.WithError(err).Error("could not deserialize")
		w.WriteHeader(http.StatusBadRequest)
		json.MarshalWrite(w, jsonapi.MessageWithError{Message: "could not deserialize", Error: err})
		return
	}
	go func() {
		if amf.sr4mecEnabled(config.SliceName(ps.UeInfo.Header.Dnn)) {
			amf.HandleN2EstablishmentResponseSR4MEC(ps)
		} else {
			amf.HandleN2EstablishmentResponse(ps)
		}
	}()
	w.WriteHeader(http.StatusAccepted)
	json.MarshalWrite(w, jsonapi.Message{Message: "please refer to logs for more information"})
}

func (amf *Amf) HandleN2EstablishmentResponseSR4MEC(ps n1n2.N2PduSessionRespMsg) {
	ctx := amf.Context()
	pduSession, err := amf.srCtrl.CreateSessionDownlink(ctx, ps.UeInfo.Header.Ue, ps.UeInfo.Addr, config.SliceName(ps.UeInfo.Header.Dnn), ps.UeInfo.Header.Gnb, ps.DownlinkFteid)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"ue-ip-addr": ps.UeInfo.Addr,
			"ue":         ps.UeInfo.Header.Ue,
			"gnb":        ps.UeInfo.Header.Gnb,
			"dnn":        ps.UeInfo.Header.Dnn,
		}).Error("could not create downlink path")
		return
	}
	logrus.WithFields(logrus.Fields{
		"ue":                ps.UeInfo.Header.Ue.String(),
		"gnb":               ps.UeInfo.Header.Gnb.String(),
		"ip-addr":           ps.UeInfo.Addr,
		"gtp-upf":           pduSession.UplinkFteid.Addr,
		"gtp-uplink-teid":   pduSession.UplinkFteid.Teid,
		"gtp-gnb":           pduSession.DownlinkFteid.Addr,
		"gtp-downlink-teid": pduSession.DownlinkFteid.Teid,
		"dnn":               ps.UeInfo.Header.Dnn,
	}).Info("New PDU Session Established")
}

func (amf *Amf) HandleN2EstablishmentResponse(ps n1n2.N2PduSessionRespMsg) {
	ctx := amf.Context()
	pduSession, err := amf.smf.CreateSessionDownlink(ctx, ps.UeInfo.Header.Ue, ps.UeInfo.Addr, config.SliceName(ps.UeInfo.Header.Dnn), ps.UeInfo.Header.Gnb, ps.DownlinkFteid, 255)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"ue-ip-addr": ps.UeInfo.Addr,
			"ue":         ps.UeInfo.Header.Ue,
			"gnb":        ps.UeInfo.Header.Gnb,
			"dnn":        ps.UeInfo.Header.Dnn,
		}).Error("could not create downlink path")
		return
	}
	logrus.WithFields(logrus.Fields{
		"ue":                ps.UeInfo.Header.Ue.String(),
		"gnb":               ps.UeInfo.Header.Gnb.String(),
		"ip-addr":           ps.UeInfo.Addr,
		"gtp-upf":           pduSession.UplinkFteid.Addr,
		"gtp-uplink-teid":   pduSession.UplinkFteid.Teid,
		"gtp-gnb":           pduSession.DownlinkFteid.Addr,
		"gtp-downlink-teid": pduSession.DownlinkFteid.Teid,
		"dnn":               ps.UeInfo.Header.Dnn,
	}).Info("New PDU Session Established")
}
