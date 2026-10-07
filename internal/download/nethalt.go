package download

import (
	"context"
	"net"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
)

type gatedConn struct {
	net.Conn
	halted func() bool
}

func (c gatedConn) Write(p []byte) (int, error) {
	if c.halted() {
		return 0, errNetworkDown
	}
	return c.Conn.Write(p)
}

type gatedPacketConn struct {
	net.PacketConn
	halted func() bool
}

func (c gatedPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if c.halted() {
		return 0, errNetworkDown
	}
	return c.PacketConn.WriteTo(p, addr)
}

func gateDial(dial dialFunc, halted func() bool) dialFunc {
	if dial == nil {
		dial = (&net.Dialer{Timeout: dialTimeout, KeepAlive: dialTimeout}).DialContext
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if halted() {
			return nil, errNetworkDown
		}
		conn, err := dial(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return gatedConn{Conn: conn, halted: halted}, nil
	}
}

// guardNetwork makes every channel a client announces on refuse traffic once
// halted says so. The library has no call that stops announcing short of
// closing the client, and a teardown keeps the old client open for as long as
// its jobs take to end; sockets opened before the cut are gated on write for
// the same reason, since trackers reuse them without dialling again.
func guardNetwork(tc *torrent.ClientConfig, halted func() bool) {
	tc.TrackerDialContext = gateDial(tc.TrackerDialContext, halted)
	tc.HTTPDialContext = gateDial(tc.HTTPDialContext, halted)

	listen := tc.TrackerListenPacket
	if listen == nil {
		listen = func(network, addr string) (net.PacketConn, error) { return net.ListenPacket(network, addr) }
	}
	tc.TrackerListenPacket = func(network, addr string) (net.PacketConn, error) {
		// The library panics when this hook fails, so a halted client gets a
		// socket that carries nothing.
		if halted() {
			return newBlockedPacketConn(), nil
		}
		pc, err := listen(network, addr)
		if err != nil {
			return nil, err
		}
		return gatedPacketConn{PacketConn: pc, halted: halted}, nil
	}

	merge := tc.MetainfoSourcesMerger
	if merge != nil {
		tc.MetainfoSourcesMerger = func(t *torrent.Torrent, mi *metainfo.MetaInfo) error {
			if halted() {
				return errNetworkDown
			}
			return merge(t, mi)
		}
	}

	nodes := tc.DhtStartingNodes
	if nodes != nil {
		tc.DhtStartingNodes = func(network string) dht.StartingNodesGetter {
			get := nodes(network)
			return func() ([]dht.Addr, error) {
				if halted() {
					return nil, errNetworkDown
				}
				return get()
			}
		}
	}

	configure := tc.ConfigureAnacrolixDhtServer
	tc.ConfigureAnacrolixDhtServer = func(cfg *dht.ServerConfig) {
		if configure != nil {
			configure(cfg)
		}
		if cfg.Conn != nil {
			cfg.Conn = gatedPacketConn{PacketConn: cfg.Conn, halted: halted}
		}
	}
}
