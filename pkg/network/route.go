// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0
package network

import (
	"fmt"
	"net"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
)

func getDefaultRoute() (*netlink.Route, error) {
	_, defaultIPv4, _ := net.ParseCIDR("0.0.0.0/0")
	_, defaultIPv6, _ := net.ParseCIDR("::/0")

	routes, err := netlink.RouteList(nil, netlink.FAMILY_ALL)
	if err != nil {
		return nil, fmt.Errorf("failed to list network routes: %w", err)
	}

	var defaultRoute *netlink.Route
	for _, route := range routes {
		if route.Dst != nil {
			if route.Dst.String() == defaultIPv4.String() || route.Dst.String() == defaultIPv6.String() {
				defaultRoute = &route
				break
			}
		}
	}

	if defaultRoute == nil {
		return nil, fmt.Errorf("failed to find default route")
	}

	return defaultRoute, nil
}

func routeForNetwork(net *net.IPNet, device netlink.Link) netlink.Route {
	// ip route replace $net dev $device
	route := netlink.Route{
		Dst:       net,
		LinkIndex: device.Attrs().Index,
	}
	return route
}

func ReplaceRoute(log logr.Logger, ipnet *net.IPNet, dev netlink.Link) error {
	route := routeForNetwork(ipnet, dev)
	log.Info("replacing route", "route", route, "ipnet", ipnet)
	if err := netlink.RouteReplace(&route); err != nil {
		return fmt.Errorf("error replacing route for %s: %w", ipnet, err)
	}
	return nil
}

// ReplaceRouteViaDevice installs or replaces a route to dst that forwards via the named device in
// the given routing table. Use unix.RT_TABLE_MAIN (254) for the main routing table.
func ReplaceRouteViaDevice(dst *net.IPNet, linkName string, tableID int) error {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return fmt.Errorf("failed to find link %s: %w", linkName, err)
	}
	route := &netlink.Route{
		Dst:       dst,
		LinkIndex: link.Attrs().Index,
		Table:     tableID,
	}
	if err := netlink.RouteReplace(route); err != nil {
		return fmt.Errorf("error replacing route for %s via %s table %d: %w", dst, linkName, tableID, err)
	}
	return nil
}

// AddFWmarkRule adds a policy routing rule that steers packets marked with the given fwmark into
// the given routing table. The priority must be unique among ip rules for the address family.
func AddFWmarkRule(mark, tableID, priority int, ipv6 bool) error {
	family := netlink.FAMILY_V4
	if ipv6 {
		family = netlink.FAMILY_V6
	}
	rule := netlink.NewRule()
	rule.Family = family
	rule.Mark = uint32(mark) //nolint:gosec // mark is a small non-negative constant
	rule.Table = tableID
	rule.Priority = priority
	if err := netlink.RuleAdd(rule); err != nil {
		return fmt.Errorf("error adding ip rule fwmark %d table %d: %w", mark, tableID, err)
	}
	return nil
}

// DeleteFWmarkRule removes the policy routing rule that matches the given fwmark and priority for
// the address family. Using both mark and priority avoids ambiguity when two rules with the same
// mark exist transiently during an atomic add-then-delete replace.
func DeleteFWmarkRule(mark, priority int, ipv6 bool) error {
	family := netlink.FAMILY_V4
	if ipv6 {
		family = netlink.FAMILY_V6
	}
	rules, err := netlink.RuleList(family)
	if err != nil {
		return fmt.Errorf("error listing ip rules: %w", err)
	}
	for _, r := range rules {
		if r.Mark == uint32(mark) && r.Priority == priority { //nolint:gosec // mark is a small non-negative constant
			if err := netlink.RuleDel(&r); err != nil {
				return fmt.Errorf("error deleting ip rule fwmark %d: %w", mark, err)
			}
			return nil
		}
	}
	return nil
}
