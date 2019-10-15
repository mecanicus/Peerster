package types

import "net"

type Message struct {
	Text string
}
type SimpleMessage struct {
	OriginalName  string
	RelayPeerAddr string
	Contents      string
}
type RumorMessage struct {
	Origin string
	ID     uint32
	Text   string
}
type PeerStatus struct {
	Identifier string
	NextID     uint32
}
type ConectionInfo struct {
	MessageToGossip *RumorMessage
	Timeout         int //Tambien se podria meter aqui un timer
}
type StatusPacket struct {
	Want []PeerStatus
}

type GossiperSocket struct {
	Address *net.UDPAddr
	Conn    *net.UDPConn
}
type GossipPacket struct {
	Simple *SimpleMessage
	Rumor  *RumorMessage
	Status *StatusPacket
}

type UDPAddr struct {
	IP   net.IP
	Port int
	Zone string // IPv6 scoped addressing zone
}
