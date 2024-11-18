// Copyright 2024 Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package config

import (
	"io/ioutil"
	"path/filepath"

	"github.com/nextmn/json-api/jsonapi"

	"gopkg.in/yaml.v3"
)

func ParseConf(file string) (*CPConfig, error) {
	var conf CPConfig
	path, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	yamlFile, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(yamlFile, &conf)
	if err != nil {
		return nil, err
	}
	return &conf, nil
}

type CPConfig struct {
	Control Control `yaml:"control"`
	Logger  *Logger `yaml:"logger,omitempty"`
}

type Control struct {
	Uri      jsonapi.ControlURI `yaml:"uri"`       // may contain domain name instead of ip address
	BindAddr string             `yaml:"bind-addr"` // in the form `ip:port`
}
