// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"github.com/caarlos0/env/v10"
	"github.com/go-logr/logr"

	"github.com/gardener/vpn2/pkg/network"
)

type BondController struct {
	VPNNetwork   network.CIDR `env:"VPN_NETWORK"`
	HAVPNServers int          `env:"HA_VPN_SERVERS"`
}

func GetBondControllerConfig(log logr.Logger) (BondController, error) {
	cfg := BondController{}
	if err := env.Parse(&cfg); err != nil {
		return cfg, err
	}
	if cfg.VPNNetwork.String() == "" {
		var err error
		cfg.VPNNetwork, err = getVPNNetworkDefault()
		if err != nil {
			return BondController{}, err
		}
	}
	if err := validateVPNNetworkCIDR(cfg.VPNNetwork); err != nil {
		return BondController{}, err
	}

	if cfg.HAVPNServers == 0 {
		return BondController{}, fmt.Errorf("HA_VPN_SERVERS must be set and greater than 0")
	}

	log.Info("config parsed", "config", cfg)
	return cfg, nil
}
