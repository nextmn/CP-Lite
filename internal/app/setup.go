// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT

package app

import (
	"context"

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
	smf := smf.NewSmf(config.Pfcp, config.Slices)
	return &Setup{
		config: config,
		amf:    amf.NewAmf(config.Control.BindAddr, config.Control.Uri, "go-github-nextmn-cp-lite", smf),
		smf:    smf,
	}
}
func (s *Setup) Init(ctx context.Context) error {
	if err := s.smf.Start(ctx); err != nil {
		return err
	}
	if err := s.amf.Start(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Setup) Run(ctx context.Context) error {
	defer s.Exit()
	if err := s.Init(ctx); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return nil
	}
}

func (s *Setup) Exit() error {
	return nil
}
