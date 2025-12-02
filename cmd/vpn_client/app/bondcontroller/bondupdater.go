// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package bondcontroller

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"

	"github.com/gardener/vpn2/pkg/constants"
	"github.com/gardener/vpn2/pkg/vpn_client"
)

type bondUpdater struct {
	pinger      vpn_client.Pinger
	bondDevice  string
	activeSlave int
	slaves      []net.IP

	log    logr.Logger
	ticker *time.Ticker
}

func (b *bondUpdater) Run(ctx context.Context) error {
	// set tap0 as initial active slave
	err := b.setActiveSlave(0)
	if err != nil {
		return fmt.Errorf("failed to set initial active slave: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-b.ticker.C:
			err = b.determineActiveSlave()
			if err != nil {
				b.log.Error(err, "failed to determine active slave")
				continue
			}
			err = b.pingActiveSlave()
			if err != nil {
				err = b.switchActiveSlave()
				if err != nil {
					b.log.Error(err, "failed to switch active slave")
				}
				continue
			}
			// OK we have connectivity to the vpn server. Let's see if we can reach at least one peer.
			err = b.pingPeers()
			if err != nil {
				b.log.Error(err, "no peer reachable, network partition likely")
				err = b.switchActiveSlave()
				if err != nil {
					b.log.Error(err, "failed to switch active slave")
				}
				continue
			}
		}
	}
}

func (b *bondUpdater) determineActiveSlave() error {
	bondLink, err := netlink.LinkByName(b.bondDevice)
	if err != nil {
		return err
	}
	bond := bondLink.(*netlink.Bond)

	for i := range b.slaves {
		slaveLink, err2 := netlink.LinkByName(fmt.Sprintf("tap%d", i))
		if err2 != nil {
			return err2
		}
		if bond.ActiveSlave == slaveLink.Attrs().Index {
			if b.activeSlave != i {
				b.log.Info("detected change of active slave", "from", fmt.Sprintf("tap%d", b.activeSlave), "to", fmt.Sprintf("tap%d", i))
			}
			b.activeSlave = i
			return nil
		}
	}

	return fmt.Errorf("no active slave found for bond device %s", b.bondDevice)
}

func (b *bondUpdater) pingActiveSlave() error {
	ip := b.slaves[b.activeSlave]
	err := b.pinger.Ping(ip)
	if err != nil {
		b.log.Info("vpn server not healthy", "ip", ip, "error", err)
		return err
	}
	return nil
}

func (b *bondUpdater) pingPeers() error {
	var peers []netlink.Link
	linkList, err := netlink.LinkList()
	if err != nil {
		return fmt.Errorf("failed to list links: %w", err)
	}
	for _, link := range linkList {
		if strings.HasPrefix(link.Attrs().Name, fmt.Sprintf("%sip6tnl", constants.BondDevice)) {
			peers = append(peers, link)
		}
	}
	if len(peers) == 0 {
		return fmt.Errorf("no peer ip6tnl tunnel links found")
	}
	for _, peer := range peers {
		peerTunnel := peer.(*netlink.Ip6tnl)
		err = b.pinger.Ping(peerTunnel.Remote)
		if err != nil {
			b.log.Info("peer not healthy", "ip", peerTunnel.Remote, "error", err)
			continue
		} else {
			// Could reach at least one peer
			return nil
		}
	}

	return fmt.Errorf("no peer could be reached")
}

func (b *bondUpdater) setActiveSlave(slaveIndex int) error {
	b.log.Info("setting active slave", "to", fmt.Sprintf("tap%d", slaveIndex))
	bondLink, err := netlink.LinkByName(b.bondDevice)
	if err != nil {
		return err
	}
	bond := bondLink.(*netlink.Bond)
	nextSlaveLink, err := netlink.LinkByName(fmt.Sprintf("tap%d", slaveIndex))
	if err != nil {
		return err
	}
	err = netlink.LinkSetBondSlaveActive(nextSlaveLink, bond)
	if err != nil {
		return err
	}
	b.activeSlave = slaveIndex
	return nil
}

func (b *bondUpdater) switchActiveSlave() error {
	// switch to next slave
	nextSlave := (b.activeSlave + 1) % len(b.slaves)
	b.log.Info("switching bond active slave", "from", fmt.Sprintf("tap%d", b.activeSlave), "to", fmt.Sprintf("tap%d", nextSlave))
	return b.setActiveSlave(nextSlave)
}
