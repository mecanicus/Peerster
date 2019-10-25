package types

import "net"

const (
	Simple      int = 1
	Rumor       int = 2
	Status      int = 3
	Private     int = 4
	FileRequest int = 5
	FileReply   int = 6
)
const SharedFiles string = "_SharedFiles"
const Downloads string = "_Downloads"

type Message struct {
	Text        string
	Destination *string
	File        *string
	Request     *[]byte
}
type FileInfo struct {
	FileSize       int64
	Metafile       []byte
	MetaHash       [32]byte
	HashesOfChunks map[[32]byte][]byte
}
type DownloadInfo struct {
	PathToSave        string        //Where to save it
	FileName          string        //Name of the file to save it to
	Timeout           int           //Timeout to repeat the request
	Metafile          []byte        //The entire metafile
	MetaHash          []byte        //Hash of the metafile
	LastHashRequested []byte        //Last hash that has been requested
	ChunkInformation  []ChunkStruct //Where all the chunk's info is stored (hashes/data)
	Destination       string        //Used in case of failed download to know who to repeat the request
}
type ChunkStruct struct {
	ChunkHash []byte
	ChunkData []byte
}
type DataRequest struct {
	Origin      string
	Destination string
	HopLimit    uint32
	HashValue   []byte
}
type DataReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	HashValue   []byte
	Data        []byte
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
type PrivateMessage struct {
	Origin      string
	ID          uint32
	Text        string
	Destination string
	HopLimit    uint32
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
	Simple      *SimpleMessage
	Rumor       *RumorMessage
	Status      *StatusPacket
	Private     *PrivateMessage
	DataRequest *DataRequest
	DataReply   *DataReply
}

type UDPAddr struct {
	IP   net.IP
	Port int
	Zone string // IPv6 scoped addressing zone
}
