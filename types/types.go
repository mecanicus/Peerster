package types

import "net"

const (
	Simple         int = 1
	Rumor          int = 2
	Status         int = 3
	Private        int = 4
	FileRequest    int = 5
	FileReply      int = 6
	SearchRequestM int = 7
	SearchReplyM   int = 8
	TLCMessageM    int = 9
	AckTLCM        int = 10
)
const SharedFiles string = "_SharedFiles"
const Downloads string = "_Downloads"

type Message struct {
	Text        string
	Destination *string
	File        *string
	Request     *[]byte
	Budget      *uint64
	Keywords    *string
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
	DestinationSearch []string      //Used in case of downloading from search request as different people has the chunks
}
type SearchRequest struct {
	Origin   string
	Budget   uint64
	Keywords []string
}
type SearchRequestSessions struct {
	SearchRequest *SearchRequest
	TimeElapsed   int
}
type ClientSearchSessions struct {
	ClientMessage *Message
	TimeElapsed   int
}
type SearchReply struct {
	Origin      string
	Destination string
	HopLimit    uint32
	Results     []*SearchResult
}
type SearchResult struct {
	FileName     string
	MetafileHash []byte
	ChunkMap     []uint64
	ChunkCount   uint64
}
type FileSearchChunks struct {
	FileName     string
	MetafileHash []byte
	ChunkCount   uint64
	ChunkStatus  []ChunkStatusStruct
}
type ChunkStatusStruct struct {
	Owners []string //Array of peers that has it
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
type TxPublish struct {
	Name         string
	Size         int64 // Size in bytes
	MetafileHash []byte
}
type BlockPublish struct {
	PrevHash    [32]byte //(used in Exercise 4, for now 0)
	Transaction TxPublish
}
type TLCMessage struct {
	Origin      string
	ID          uint32
	Confirmed   int
	TxBlock     BlockPublish
	VectorClock *StatusPacket //(used in Exercise 3, for now nil)
	Fitness     float32       //(used in Exercise 4, for now 0)
}

type TLCAck PrivateMessage

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
	Origin      string // peer acking the msg
	ID          uint32 // the same ID as the acked TLCMessage
	Text        string // can be empty
	Destination string // the source of the acked TLCMessage
	HopLimit    uint32 // default 10, otherwise -hopLimit flag
}
type PeerStatus struct {
	Identifier string
	NextID     uint32
}
type ConectionInfo struct {
	MessageToGossip *RumorMessage
	Timeout         int //Tambien se podria meter aqui un timer
	MessageTLC      *TLCMessage
}
type TLCMensInfo struct {
	TLCMessage TLCMessage
	Timeout    int
	AmountACK  map[string]bool //Key the peer ACKing, value if acked or not
	StopAcking bool
}
type StatusPacket struct {
	Want []PeerStatus
}

type GossiperSocket struct {
	Address *net.UDPAddr
	Conn    *net.UDPConn
}
type GossipPacket struct {
	Simple        *SimpleMessage
	Rumor         *RumorMessage
	Status        *StatusPacket
	Private       *PrivateMessage
	DataRequest   *DataRequest
	DataReply     *DataReply
	SearchRequest *SearchRequest
	SearchReply   *SearchReply
	TLCMessage    *TLCMessage
	Ack           *TLCAck
}

type UDPAddr struct {
	IP   net.IP
	Port int
	Zone string // IPv6 scoped addressing zone
}
