//go:build linux

package client

import "net"

type packetReader interface{ ReadPacket() ([]byte, error) }

func newPacketReader(conn net.Conn) packetReader      { return newNewlineReader(conn) }
func writePacket(conn net.Conn, payload []byte) error { return writeNewlinePacket(conn, payload) }
