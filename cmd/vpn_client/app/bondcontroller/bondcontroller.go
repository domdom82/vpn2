// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bondcontroller

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"

	"github.com/gardener/vpn2/pkg/config"
	"github.com/gardener/vpn2/pkg/constants"
	"github.com/gardener/vpn2/pkg/network"
	"github.com/gardener/vpn2/pkg/utils"
	"github.com/gardener/vpn2/pkg/vpn_client"
)

const Name = "bond-controller"

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   Name,
		Short: Name,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			log, err := utils.InitRun(cmd, Name)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithCancel(cmd.Context())
			return run(ctx, cancel, log)
		},
	}

	return cmd
}

func run(ctx context.Context, _ context.CancelFunc, log logr.Logger) error {
	cfg, err := config.GetBondControllerConfig(log)
	if err != nil {
		return err
	}

	serverIPs := network.AllBondingServerIPs(cfg.VPNNetwork.ToIPNet(), cfg.HAVPNServers)

	updater := &bondUpdater{
		pinger: &vpn_client.IcmpPinger{
			Log:     log.WithName("bondPing"),
			Timeout: 1 * time.Second,
			Retries: 0,
		},
		bondDevice: constants.BondDevice,
		slaves:     serverIPs,
		ticker:     time.NewTicker(constants.BondControllerUpdateInterval),
		log:        log.WithName("bondUpdater"),
	}

	return updater.Run(ctx)
}
