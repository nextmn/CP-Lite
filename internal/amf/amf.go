// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
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

	"github.com/nextmn/cp-lite/internal/smf"

	"github.com/nextmn/json-api/healthcheck"
	"github.com/nextmn/json-api/jsonapi"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type Amf struct {
	control   jsonapi.ControlURI
	client    http.Client
	userAgent string
	smf       *smf.Smf
	srv       *http.Server
	closed    chan struct{}

	// not exported because must not be modified
	ctx context.Context
}

func NewAmf(bindAddr netip.AddrPort, control jsonapi.ControlURI, userAgent string, smf *smf.Smf) *Amf {
	amf := Amf{
		control:   control,
		client:    http.Client{},
		userAgent: userAgent,
		smf:       smf,
		closed:    make(chan struct{}),
	}
	// TODO: gin.SetMode(gin.DebugMode) / gin.SetMode(gin.ReleaseMode) depending on log level
	r := gin.Default()
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
	if ctx == nil {
		return ErrNilCtx
	}
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
		select {
		case <-ctx.Done():
			ctxShutdown, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			if err := amf.srv.Shutdown(ctxShutdown); err != nil {
				logrus.WithError(err).Info("HTTP Server Shutdown")
			}
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

func (amf *Amf) Context() context.Context {
	if amf.ctx != nil {
		return amf.ctx
	}
	return context.Background()
}
