// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package moveip

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"

	"github.com/gardener/vpn2/pkg/network"
	"github.com/gardener/vpn2/pkg/utils"
)

const Name = "move-ip"

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
			defer cancel()
			return run(ctx, log, cmd)
		},
	}

	cmd.Flags().String("cidr", "", "The CIDR range containing the IP addresses to be moved")
	cmd.Flags().String("src", "", "The source device from where to move the IP addresses")
	cmd.Flags().String("tgt", "", "The target device to where the IP addresses should move")
	cmd.Flags().StringSlice("flags", []string{}, "A list of additional flags to put on the IP addresses")

	return cmd
}

func run(_ context.Context, log logr.Logger, cmd *cobra.Command) error {
	cidr, _ := cmd.Flags().GetString("cidr")
	src, _ := cmd.Flags().GetString("src")
	tgt, _ := cmd.Flags().GetString("tgt")
	flags, _ := cmd.Flags().GetStringSlice("flags")

	log.Info("Moving IP addresses", "cidr", cidr, "src", src, "tgt", tgt, "flags", flags)
	err := network.MoveIPs(cidr, src, tgt, flags)
	if err != nil {
		log.Error(err, "Failed to move IP addresses")
		return err
	}

	return nil
}
