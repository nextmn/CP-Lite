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
	config *config.CPConfig
}

func NewSetup(config *config.CPConfig) *Setup {
	return &Setup{
		config: config,
	}
}
func (s *Setup) Init(ctx context.Context) error {
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
