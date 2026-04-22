// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package amf

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/nextmn/cp-lite/internal/common"
	"github.com/nextmn/cp-lite/internal/config"
	"github.com/nextmn/cp-lite/internal/smf"
	"github.com/nextmn/cp-lite/internal/sr4mec"

	"github.com/nextmn/json-api/healthcheck"
	"github.com/nextmn/json-api/jsonapi"
	"github.com/nextmn/logrus-formatter/ginlogger"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type Amf struct {
	common.WithContext

	n2        N2
	control   jsonapi.ControlURI
	client    http.Client
	userAgent string
	smf       *smf.Smf
	srCtrl    *sr4mec.Ctrl
	srv       *http.Server
	closed    chan struct{}
	emulation config.Emulation
}

func NewAmf(bindAddr netip.AddrPort, control jsonapi.ControlURI, areas map[config.AreaName]config.Area, userAgent string, smf *smf.Smf, srctrl *sr4mec.Ctrl, emulation config.Emulation) *Amf {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = (&net.Dialer{
		// Force using "rest" interface IP Address
		LocalAddr: &net.TCPAddr{IP: bindAddr.Addr().AsSlice()},
		// Same parameters as http.DefaultTransport's Dialer
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext

	amf := Amf{
		n2:        NewN2(areas),
		control:   control,
		client:    http.Client{Transport: t},
		userAgent: userAgent,
		smf:       smf,
		srCtrl:    srctrl,
		closed:    make(chan struct{}),
		emulation: emulation,
	}
	gin.SetMode(gin.ReleaseMode)
	r := ginlogger.Default()
	r.GET("/status", Status)

	// PDU Sessions
	r.POST("/ps/establishment-request", amf.EstablishmentRequest)
	r.POST("/ps/n2-establishment-response", amf.N2EstablishmentResponse)
	r.POST("/ps/handover-required", amf.HandoverRequired)
	r.POST("/ps/handover-request-ack", amf.HandoverRequestAck)
	r.POST("/ps/handover-notify", amf.HandoverNotify)

	logrus.WithFields(logrus.Fields{"http-addr": bindAddr}).Info("HTTP Server created")
	amf.srv = &http.Server{
		Addr:    bindAddr.String(),
		Handler: r,
	}

	return &amf
}

func (amf *Amf) Start(ctx context.Context) error {
	amf.InitContext(ctx)
	l, err := net.Listen("tcp", amf.srv.Addr)
	if err != nil {
		return err
	}
	go func(ln net.Listener) {
		logrus.Info("Starting HTTP Server")
		if err := amf.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logrus.WithError(err).Error("Http Server error")
		}
	}(l)
	go func(ctx context.Context) {
		defer close(amf.closed)
		<-ctx.Done()
		ctxShutdown, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		if err := amf.srv.Shutdown(ctxShutdown); err == nil {
			logrus.Info("HTTP Server Shutdown")
		}
	}(ctx)
	return nil
}

func (amf *Amf) WaitShutdown(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-amf.closed:
		return nil
	}
}

// get status of the controller
func Status(c *gin.Context) {
	status := healthcheck.Status{
		Ready: true,
	}
	c.Header("Cache-Control", "no-cache")
	c.JSON(http.StatusOK, status)
}
