// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"fmt"
	"net"
	"slices"

	"github.com/vishvananda/netlink"

	"github.com/gardener/vpn2/pkg/constants"
)

// The High-Availability VPN divides the VPN network in several subnets.
// Assuming the VPN network is using the default CIDR `fd8f:6d53:b97a:1::0/96`, these subnets are used:
// - For the underlying VPN tunnel for each VPN server (the VPN index)
//   - subnet for VPN index 0: `fd8f:6d53:b97a:1::100:0/112`
//   - subnet for VPN index 1: `fd8f:6d53:b97a:1::101:0/112`
// - subnet for the bonding network: `fd8f:6d53:b97a:1::0/104`
//   - IP of shoot client 0: `fd8f:6d53:b97a:1::b:0`
//   - IP of shoot client 1: `fd8f:6d53:b97a:1::b:1`
//   - IPs of seed clients are in the range `fd8f:6d53:b97a:1::a:1` to `fd8f:6d53:b97a:1::a:ffff`

const (
	addrLen             = 128
	bondPrefixSize      = 104
	vpnTunnelPrefixSize = 112
	bondStartSeed       = 0xa
	bondStartShoot      = 0xb
	startIndexSeed      = 1
	endIndexSeed        = 0xffff
)

func BondingShootClientAddress(vpnNetwork *net.IPNet, vpnClientIndex int) *net.IPNet {
	ip := BondingShootClientIP(vpnNetwork, vpnClientIndex)
	return BondingAddressForClient(ip)
}

func BondingAddressForClient(ip net.IP) *net.IPNet {
	return &net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(bondPrefixSize, addrLen),
	}
}

func AllBondingShootClientIPs(vpnNetwork *net.IPNet, haVPNClients int) []net.IP {
	ips := make([]net.IP, haVPNClients)
	for i := 0; i < haVPNClients; i++ {
		ips[i] = BondingShootClientIP(vpnNetwork, i)
	}
	return ips
}

func AllBondingServerIPs(vpnNetwork *net.IPNet, haVPNServers int) []net.IP {
	ips := make([]net.IP, haVPNServers)
	for i := 0; i < haVPNServers; i++ {
		cidr := HAVPNTunnelNetwork(vpnNetwork.IP, i)
		ips[i] = cidr.IP
		ips[i][15] = 1
	}
	return ips
}

func BondingShootClientIP(vpnNetwork *net.IPNet, index int) net.IP {
	ip := slices.Clone(vpnNetwork.IP.To16())
	ip[15] = byte(index)
	ip[14] = 0
	ip[13] = byte(bondStartShoot)
	return ip
}

func BondingSeedClientRange(vpnNetworkIP net.IP) (base net.IP, startIndex, endIndex int) {
	base = slices.Clone(vpnNetworkIP.To16())
	base[15] = 0
	base[14] = 0
	base[13] = byte(bondStartSeed)
	startIndex = startIndexSeed
	endIndex = endIndexSeed
	return
}

func ClientIndexFromBondingShootClientIP(clientIP net.IP) int {
	return int(clientIP[len(clientIP)-1])
}

func BondIP6TunnelLinkName(index int) string {
	return fmt.Sprintf("%sip6tnl%d", constants.BondDevice, index)
}

func HAVPNTunnelNetwork(vpnNetworkIP net.IP, vpnIndex int) CIDR {
	base := slices.Clone(vpnNetworkIP.To16())
	base[15] = 0
	base[14] = 0
	base[13] = byte(vpnIndex)
	base[12] = 1

	return CIDR{
		IP:   base,
		Mask: net.CIDRMask(vpnTunnelPrefixSize, addrLen),
	}
}

// MoveIPs moves all IP addresses from source link to target link that are contained in the given CIDR.
func MoveIPs(cidr, src, tgt string, flags []string) error {
	_, subnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	srcLink, err := netlink.LinkByName(src)
	if err != nil {
		return fmt.Errorf("failed to get source link %s: %w", src, err)
	}

	tgtLink, err := netlink.LinkByName(tgt)
	if err != nil {
		return fmt.Errorf("failed to get target link %s: %w", tgt, err)
	}

	srcIPs, err := netlink.AddrList(srcLink, netlink.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("failed to list addresses of link %s: %w", srcLink.Attrs().Name, err)
	}

	tgtIPs, err := netlink.AddrList(tgtLink, netlink.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("failed to list addresses of link %s: %w", tgtLink.Attrs().Name, err)
	}

	// Clean up existing IPs in target link that are in the subnet
	for _, addrToDel := range tgtIPs {
		if subnet.Contains(addrToDel.IP) {
			err = netlink.AddrDel(tgtLink, &addrToDel)
			if err != nil {
				return fmt.Errorf("failed to delete existing IP address %s from link %s: %w", addrToDel.String(), tgt, err)
			}
		}
	}

	// Move IPs from source link to target link
	for _, addrToDel := range srcIPs {
		if subnet.Contains(addrToDel.IP) {
			// Copy addrToDel to avoid modifying the original
			addrToAdd := addrToDel

			// Set flags if provided
			if len(flags) > 0 {
				addrToAdd.Flags = IPAddrFlagsFromString(flags)
			}

			// Add IP to target link
			err = netlink.AddrAdd(tgtLink, &addrToAdd)
			if err != nil {
				return fmt.Errorf("failed to add IP address %s to link %s: %w", addrToAdd.String(), tgt, err)
			}

			// Remove IP from source link
			err = netlink.AddrDel(srcLink, &addrToDel)
			if err != nil {
				return fmt.Errorf("failed to delete IP address %s from link %s: %w", addrToDel.String(), src, err)
			}
		}

	}
	return nil
}
