// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

const (
	familyAll     = 0
	ScopeUniverse = 0
	ScopeLink     = 253
)

// flagTypes maps netlink IP address flags to their string representations.
// See https://github.com/iproute2/iproute2/blob/main/ip/ipaddress.c#L1393-L1413
var flagTypes = map[int]string{
	unix.IFA_F_SECONDARY:      "secondary",
	unix.IFA_F_NODAD:          "nodad",
	unix.IFA_F_HOMEADDRESS:    "home",
	unix.IFA_F_DEPRECATED:     "deprecated",
	unix.IFA_F_OPTIMISTIC:     "optimistic",
	unix.IFA_F_DADFAILED:      "dadfailed",
	unix.IFA_F_TENTATIVE:      "tentative",
	unix.IFA_F_PERMANENT:      "permanent",
	unix.IFA_F_MANAGETEMPADDR: "mngtmpaddr",
	unix.IFA_F_NOPREFIXROUTE:  "noprefixroute",
	unix.IFA_F_MCAUTOJOIN:     "autojoin",
	unix.IFA_F_STABLE_PRIVACY: "stable-privacy",
}

// DeleteLinkByName delete a link by name.
func DeleteLinkByName(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		var linkNotFoundError netlink.LinkNotFoundError
		if errors.As(err, &linkNotFoundError) {
			return nil
		}
		return fmt.Errorf("failed to get link %s for deletion: %w", name, err)
	}

	if err = netlink.LinkDel(link); err != nil {
		return fmt.Errorf("failed to delete link %s: %w", name, err)
	}
	return nil
}

// CreateTunnel creates an ip6tnl tunnel to allow IPv4 and IPv6 packages over IPv6 and sets it up.
func CreateTunnel(linkName string, local, remote net.IP) error {
	tunnel := &netlink.Ip6tnl{
		LinkAttrs: netlink.LinkAttrs{
			Name: linkName,
		},
		Local:  local,
		Remote: remote,
	}
	if err := netlink.LinkAdd(tunnel); err != nil {
		return fmt.Errorf("failed to add link %s: %w", linkName, err)
	}
	if err := netlink.LinkSetUp(tunnel); err != nil {
		return fmt.Errorf("failed to set up link %s: %w", linkName, err)
	}
	return nil
}

// GetLinkIPAddressesByName gets the IP addresses for the given link name and scope (`ScopeLink` or `ScopeUniversal`).
func GetLinkIPAddressesByName(name string, scope int) ([]net.IP, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get link %s: %w", name, err)
	}
	return getLinkIPAddresses(link, scope)
}

// getLinkIPAddresses gets the IP addresses for the given link and scope (`ScopeLink` or `ScopeUniversal`).
func getLinkIPAddresses(link netlink.Link, scope int) ([]net.IP, error) {
	addrs, err := netlink.AddrList(link, familyAll)
	if err != nil {
		return nil, fmt.Errorf("failed to list addresses of link %s: %w", link.Attrs().Name, err)
	}
	var ips []net.IP
	for _, addr := range addrs {
		if addr.Scope == scope {
			ips = append(ips, addr.IP)
		}
	}
	return ips, nil
}

// GetLinkIPAddrForIP gets the netlink.Addr for the given link name and IP address.
func GetLinkIPAddrForIP(name string, ip net.IP) (*netlink.Addr, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get link %s: %w", name, err)
	}
	addrs, err := netlink.AddrList(link, familyAll)
	if err != nil {
		return nil, fmt.Errorf("failed to list addresses of link %s: %w", link.Attrs().Name, err)
	}
	for _, addr := range addrs {
		if addr.IP.Equal(ip) {
			return &addr, nil
		}
	}
	return nil, fmt.Errorf("no address %s found on link %s", ip.String(), name)
}

// IPAddrFlagsToString converts IP address flags to a human-readable string.
func IPAddrFlagsToString(flags int) string {
	flagsStr := strings.Builder{}

	for flag, name := range flagTypes {
		if flags&flag != 0 {
			flagsStr.WriteString(name + " ")
		}
	}

	return strings.TrimSpace(flagsStr.String())
}

// IPAddrFlagsFromString converts a list of human-readable flag strings to their corresponding flag value.
func IPAddrFlagsFromString(flagsStr []string) int {
	flags := 0

	for flag, name := range flagTypes {
		for _, f := range flagsStr {
			if f == name {
				flags |= flag
			}
		}
	}

	return flags
}
