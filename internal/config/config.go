// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package config

import (
	"net/netip"
	"os"
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
	yamlFile, err := os.ReadFile(path)
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
	Control Control          `yaml:"control"`
	Pfcp    netip.Addr       `yaml:"pfcp"`
	Slices  map[string]Slice `yaml:"slices"`
	Areas   map[string]Area  `yaml:"areas"`
	Logger  *Logger          `yaml:"logger,omitempty"`
}

type Control struct {
	Uri      jsonapi.ControlURI `yaml:"uri"`       // may contain domain name instead of ip address
	BindAddr netip.AddrPort     `yaml:"bind-addr"` // in the form `ip:port`
}

type Slice struct {
	Pool netip.Prefix `yaml:"pool"`
	Upfs []Upf        `yaml:"upfs"`
}

type Upf struct {
	NodeID     netip.Addr  `yaml:"node-id"`
	Interfaces []Interface `yaml:"interfaces"`
}

type Interface struct {
	Type string     `yaml:"type"`
	Addr netip.Addr `yaml:"addr"`
}

type Area struct {
	Gnbs  []jsonapi.ControlURI      `yaml:"gnbs"`
	Paths map[string][]GTPInterface `yaml:"paths"`
}

type GTPInterface struct {
	NodeID        netip.Addr `yaml:"node-id"`
	InterfaceAddr netip.Addr `yaml:"interface-addr"`
}
