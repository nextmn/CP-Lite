// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package app

import (
	"context"
	"time"

	"github.com/nextmn/cp-lite/internal/amf"
	"github.com/nextmn/cp-lite/internal/config"
	"github.com/nextmn/cp-lite/internal/smf"
)

type Setup struct {
	config *config.CPConfig
	amf    *amf.Amf
	smf    *smf.Smf
}

func NewSetup(config *config.CPConfig) *Setup {
	smf := smf.NewSmf(config.Pfcp, config.Slices, config.Areas)
	return &Setup{
		config: config,
		amf:    amf.NewAmf(config.Control.BindAddr, config.Control.Uri, "go-github-nextmn-cp-lite", smf),
		smf:    smf,
	}
}

func (s *Setup) Run(ctx context.Context) error {
	if err := s.smf.Start(ctx); err != nil {
		return err
	}
	if err := s.amf.Start(ctx); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		ctxShutdown, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()
		s.amf.WaitShutdown(ctxShutdown)
		s.smf.WaitShutdown(ctxShutdown)
	}
	return nil
}
