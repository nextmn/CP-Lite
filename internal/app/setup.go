// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package app

import (
	"context"

	"github.com/nextmn/cp-lite/internal/config"
)

type Setup struct {
	config           *config.CPConfig
	httpServerEntity *HttpServerEntity
}

func NewSetup(config *config.CPConfig) *Setup {
	return &Setup{
		config:           config,
		httpServerEntity: NewHttpServerEntity(config.Control.BindAddr),
	}
}
func (s *Setup) Init(ctx context.Context) error {
	if err := s.httpServerEntity.Start(); err != nil {
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
	s.httpServerEntity.Stop()
	return nil
}
