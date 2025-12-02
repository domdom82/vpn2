// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"

	"github.com/caarlos0/env/v10"
	"github.com/go-logr/logr"
)

type TunnelController struct {
	PodName string `env:"POD_NAME"`
}

func GetTunnelControllerConfig(log logr.Logger) (TunnelController, error) {
	cfg := TunnelController{}
	if err := env.Parse(&cfg); err != nil {
		return cfg, err
	}

	if cfg.PodName == "" {
		return cfg, fmt.Errorf("POD_NAME is required")
	}

	log.Info("config parsed", "config", cfg)
	return cfg, nil
}
