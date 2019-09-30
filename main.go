package main

import "net"

type SimpleMessage struct {
	OriginalName  string
	RelayPeerAddr string
	Contents      string
}

type GossipPacket struct {
	Simple *SimpleMessage
}

type UDPAddr struct {
	IP   IP
	Port int
	Zone string // IPv6 scoped addressing zone
}

//TODO: Add a list of the gossipers known
func main() {
	packetToSend := GossipPacket{Simple: simplemessage}
}

func NewGossiper(address, name string) *Gossiper {
	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	return &Gossiper{
		address: udpAddr,
		conn:    udpConn,
		Name:    name,
	}
}
