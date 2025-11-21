// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package vpn_client

import (
	"context"
	"fmt"
	"net"
	"os/exec"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/gardener/vpn2/pkg/config"
	"github.com/gardener/vpn2/pkg/constants"
	"github.com/gardener/vpn2/pkg/ippool"
	"github.com/gardener/vpn2/pkg/network"
)

func ConfigureBonding(ctx context.Context, log logr.Logger, cfg *config.VPNClient) error {
	var addr *net.IPNet

	if cfg.IsShootClient {
		addr = network.BondingShootClientAddress(cfg.VPNNetwork.ToIPNet(), cfg.VPNClientIndex)
	} else {
		manager, err := ippool.NewPodIPPoolManager(cfg.Namespace, cfg.PodLabelSelector)
		if err != nil {
			return err
		}
		broker, err := ippool.NewIPAddressBroker(manager, cfg)
		if err != nil {
			return err
		}

		log.Info("acquiring ip address for bonding from kube-api server")
		acquiredIP, err := broker.AcquireIP(ctx)
		if err != nil {
			return fmt.Errorf("failed to acquire ip: %w", err)
		}
		ip := net.ParseIP(acquiredIP)
		if ip == nil {
			return fmt.Errorf("acquired ip %s is not a valid ipv6 nor ipv4", ip)
		}
		addr = network.BondingAddressForClient(ip)
	}

	for i := range cfg.HAVPNServers {
		linkName := fmt.Sprintf("tap%d", i)
		log.Info("deleting existing tap device if any", "link", linkName)
		err := network.DeleteLinkByName(linkName)
		if err != nil {
			return err
		}

		log.Info("creating new tap device", "link", linkName)
		linkDev := &netlink.Tuntap{
			LinkAttrs: netlink.LinkAttrs{
				Name: linkName,
			},
			Mode: netlink.TUNTAP_MODE_TAP,
		}

		err = netlink.LinkAdd(linkDev)
		if err != nil {
			return err
		}
	}

	// check if bond device already exists and delete it if exists
	log.Info("deleting existing bond device if any", "link", constants.BondDevice)
	err := network.DeleteLinkByName(constants.BondDevice)
	if err != nil {
		return err
	}

	//tap0Link, err := netlink.LinkByName(constants.TapDevice)
	//if err != nil {
	//	return fmt.Errorf("failed to get link %s: %w", constants.TapDevice, err)
	//}

	// create bond device
	//linkAttrs := netlink.NewLinkAttrs()
	//bond := netlink.NewLinkBond(linkAttrs)
	//// use bonding
	//// - with active-backup mode
	//// - monitoring with use_carrier=1
	//// - using `primary tap0` to avoid ambiguity of selection if multiple devices are up (primary_reselect=always by default)
	//// - using `num_grat_arp 5` as safeguard on switching device
	//bond.Name = constants.BondDevice
	//bond.Mode = netlink.BOND_MODE_ACTIVE_BACKUP
	//bond.FailOverMac = netlink.BOND_FAIL_OVER_MAC_ACTIVE
	////bond.Miimon = 100
	////bond.UseCarrier = 1
	//bond.Primary = tap0Link.Attrs().Index
	//bond.NumPeerNotif = 5

	//bond.ArpIpTargets = ndiscIPs
	//bond.ArpInterval = 1000
	//bond.ArpValidate = netlink.BOND_ARP_VALIDATE_ACTIVE
	//bond.ArpAllTargets = netlink.BOND_ARP_ALL_TARGETS_ANY
	//
	//log.Info("creating new bond device", "link", constants.BondDevice)
	//if err = netlink.LinkAdd(bond); err != nil {
	//	return fmt.Errorf("failed to create %s link device: %w", constants.BondDevice, err)
	//}

	// netlink doesn't have the options we need for IPv6 NDP validation, hence we use exec.Command here
	// ip link add name bond0 type bond mode active-backup fail_over_mac active primary tap0 primary_reselect failure arp_interval 1000 arp_validate filter_active arp_all_targets any arp_missed_max 2 ns_ip6_target fd8f:6d53:b97a:1::100:1,fd8f:6d53:b97a:1::101:1
	err = exec.Command("ip", "link", "add",
		"name", constants.BondDevice,
		"type", "bond",
		"mode", "active-backup",
		"fail_over_mac", "active",
		"primary", "tap0",
		"primary_reselect", "failure",
		"arp_interval", "3000",
		"arp_validate", "all",
		"arp_all_targets", "all",
		"arp_missed_max", "2",
		"ns_ip6_target", "fd8f:6d53:b97a:1::100:1,fd8f:6d53:b97a:1::101:1",
	).Run()

	if err != nil {
		return fmt.Errorf("failed to create %s link device via ip command: %w", constants.BondDevice, err)
	}

	bond, err := netlink.LinkByName(constants.BondDevice)
	if err != nil {
		return fmt.Errorf("failed to get link %s: %w", constants.BondDevice, err)
	}

	for i := range cfg.HAVPNServers {
		linkName := fmt.Sprintf("tap%d", i)

		link, err := netlink.LinkByName(linkName)
		if err != nil {
			return fmt.Errorf("failed to get link %s: %w", linkName, err)
		}

		err = netlink.LinkSetMaster(link, bond)
		if err != nil {
			return fmt.Errorf("failed to set %s as master for link %s: %w", constants.BondDevice, linkName, err)
		}
	}

	log.Info("setting up bond device", "link", constants.BondDevice, "address", addr.String())
	err = netlink.LinkSetUp(bond)
	if err != nil {
		return fmt.Errorf("failed to up %s link: %w", constants.BondDevice, err)
	}
	err = netlink.AddrAdd(bond, &netlink.Addr{IPNet: addr, Flags: unix.IFA_F_NODAD})
	if err != nil {
		return fmt.Errorf("failed to add address %s to %s link: %w", addr, constants.BondDevice, err)
	}

	if !cfg.IsShootClient {
		for i := range cfg.HAVPNClients {
			// #nosec: G115 -- overflow unlikely (max value at least 2147483647 before overflow)
			ip6tnlName := network.BondIP6TunnelLinkName(int(i))
			// check if the link already exists and delete it if exists
			if err := network.DeleteLinkByName(ip6tnlName); err != nil {
				return fmt.Errorf("failed to delete link %s: %w", ip6tnlName, err)
			}
			// #nosec: G115 -- overflow unlikely (max value at least 2147483647 before overflow)
			if err := network.CreateTunnel(ip6tnlName, addr.IP, network.BondingShootClientIP(cfg.VPNNetwork.ToIPNet(), int(i))); err != nil {
				return fmt.Errorf("failed to create tunnel ip6-net link: %w", err)
			}
		}
	}

	return nil
}
