// Copyright Louis Royer and the NextMN contributors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.
// SPDX-License-Identifier: MIT
package config

import (
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"github.com/nextmn/json-api/jsonapi"

	"gopkg.in/yaml.v3"
)

type SliceName string
type AreaName string

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
	Control   Control             `yaml:"control"`
	Pfcp      netip.Addr          `yaml:"pfcp"`
	Slices    map[SliceName]Slice `yaml:"slices"`
	Areas     map[AreaName]Area   `yaml:"areas"`
	Emulation Emulation           `yaml:"emulation"`
	Logger    *Logger             `yaml:"logger,omitempty"`
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
	OneWayDelay time.Duration                `yaml:"one-way-delay"`
	Gnbs        []jsonapi.ControlURI         `yaml:"gnbs"`
	Paths       map[SliceName][]GTPInterface `yaml:"paths"`
}

type GTPInterface struct {
	NodeID        netip.Addr `yaml:"node-id"`
	InterfaceAddr netip.Addr `yaml:"interface-addr"`
}

type Emulation struct {
	HandoverNotify time.Duration `yaml:"handover-notify"`
	N4SR4MEC       N4SR4MEC      `yaml:"n4-sr4mec"`
}

type N4SR4MEC struct {
	Control Control               `yaml:"control"`
	Enabled bool                  `yaml:"enabled"`
	Slices  map[SliceName]SliceSR `yaml:"slices"`
}

type SliceSR struct {
	MigrationAPosteriori bool          `yaml:"migration-a-posteriori"`
	MigrationDelay       time.Duration `yaml:"migration-delay"`
	Service              netip.Addr    `yaml:"service"`
	PsEstablishment      SRConfig      `yaml:"ps-establishment"`
	HandoverMigration    SRConfig      `yaml:"handover-migration"`
}

type SRConfig struct {
	Srgw             jsonapi.ControlURI `yaml:"srgw"`
	SrgwGtp4         netip.Addr         `yaml:"srgw-gtp4"`
	Anchor           jsonapi.ControlURI `yaml:"anchor"`
	UplinkSegments   []string           `yaml:"uplink-segments"`
	DownlinkSegments []string           `yaml:"downlink-segments"`
}
