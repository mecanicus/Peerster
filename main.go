package main

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dedis/protobuf"
	. "github.com/mecanicus/Peerster/types"
)

type Gossiper struct {
	UIPort                    int
	Addr                      string
	Name                      string
	KnownPeers                []string
	Mode                      bool
	Want                      []PeerStatus
	wantMutex                 sync.RWMutex
	SavedMessages             map[string][]RumorMessage
	savedMessagesMutex        sync.RWMutex
	savedPrivateMessages      map[string][]PrivateMessage
	TalkingPeers              map[string]ConectionInfo
	talkingPeersMutex         sync.RWMutex
	RoutingTable              map[string]string
	routingTableMutex         sync.RWMutex
	RoutingTableControl       map[string]uint32 //Key name of peer, value higher ID received
	routingTableControlMutex  sync.RWMutex
	StoredFiles               map[string]*FileInfo //The key is the file name
	FilesBeingDownloaded      []*DownloadInfo
	filesBeingDownloadedMutex sync.RWMutex
	Socket                    *GossiperSocket
	Rtimer                    int
	AntiEntropyTimeout        int
}

func flagReader() (*string, *string, *bool, *int, *string, *int, *int) {
	uiPort := flag.Int("UIPort", 8080, "UIPort")
	gossipAddr := flag.String("gossipAddr", "127.0.0.1:5000", "gossipAddr")
	gossiperName := flag.String("name", "GossiperName", "name")
	peersList := flag.String("peers", "", "peers list")
	gossiperMode := flag.Bool("simple", false, "mode to run")
	antiEntropyTimeout := flag.Int("antiEntropy", 10, "Anti entropy timeout")
	rtimer := flag.Int("rtimer", 0, "Amount of time between route messages, in seconds")
	flag.Parse()
	return gossipAddr, gossiperName, gossiperMode, uiPort, peersList, antiEntropyTimeout, rtimer
}

func gossiperUISocketOpen(address string, UIPort int) *GossiperSocket {

	splitterAux := strings.Split(address, ":")
	UIAddress := splitterAux[0] + ":" + strconv.Itoa(UIPort)

	udpAddr, err := net.ResolveUDPAddr("udp4", UIAddress)
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		panic(err)
	}
	return &GossiperSocket{
		Address: udpAddr,
		Conn:    udpConn,
	}

}
func gossiperSocketOpen(address string) *GossiperSocket {

	udpAddr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		panic(err)
	}
	udpConn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		panic(err)
	}

	return &GossiperSocket{
		Address: udpAddr,
		Conn:    udpConn,
	}

}
func listenSocket(socket *GossiperSocket, gossiper *Gossiper) {
	buf := make([]byte, 2000)
	for {
		_, _, err := socket.Conn.ReadFromUDP(buf)
		if err != nil {
			panic(err)
		}
		// Do stuff with the read bytes
		packet := &GossipPacket{}
		protobuf.Decode(buf, packet)
		message := packet.Simple
		fmt.Println("SIMPLE MESSAGE origin " + message.OriginalName + " from " + message.RelayPeerAddr + " contents " + message.Contents)

		if checkPeersListSimple(message.RelayPeerAddr, gossiper) == false {
			addPeerToListSimple(message.RelayPeerAddr, gossiper)
		}
		//printKnownPeers(gossiper)
		sendToPeersComingFromPeer(message, gossiper)
	}
}

func listenUISocket(UISocket *GossiperSocket, gossiper *Gossiper) {
	buf := make([]byte, 2000)
	for {

		_, _, err := UISocket.Conn.ReadFromUDP(buf)

		if err != nil {
			panic(err)
		}
		packet := &Message{}
		protobuf.Decode(buf, packet)

		message := SimpleMessage{
			OriginalName:  gossiper.Name,
			RelayPeerAddr: gossiper.Addr,
			Contents:      packet.Text,
		}
		fmt.Println("CLIENT MESSAGE " + message.Contents)
		sendToPeersComingFromClient(&message, gossiper)
	}
}
func listenSocketNotSimple(socket *GossiperSocket, gossiper *Gossiper) {
	for {
		buf := make([]byte, 8300)
		_, sender, err := socket.Conn.ReadFromUDP(buf)
		if err != nil {
			panic(err)
		}

		packet := &GossipPacket{}
		protobuf.Decode(buf, packet)

		peerTalking := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
		gossiper.talkingPeersMutex.RLock()
		talkingPeersMap := gossiper.TalkingPeers
		_, exists := talkingPeersMap[peerTalking]
		gossiper.talkingPeersMutex.RUnlock()
		//Add to peer list if necessary
		if checkPeersList(sender, gossiper) == false {
			addPeerToList(sender, gossiper)
		}
		//printKnownPeers(gossiper)
		packetType := packetType(*packet)
		switch packetType {
		case Status:
			message := packet.Status

			//Is a status message of an already open connection
			if exists == true {
				connectionRenewal(peerTalking, gossiper)
				statusDecisionMaking(sender, message, gossiper)
				//Reiniciar el timeout

				continue

				//Es un mensaje de status de alguien nuevo, probablemente algoritmo anti entrophy
			} else {
				statusDecisionMaking(sender, message, gossiper)
				continue
			}
		case Rumor:
			//Es un mensaje de rumor
			message := packet.Rumor
			//TODO: Do I have to print this line if is a route message?
			if message.Text != "" {
				fmt.Println("RUMOR origin " + message.Origin + " from " + peerTalking + " ID " + fmt.Sprint(message.ID) + " contents " + message.Text)
			}
			nextHopManagement(message, peerTalking, gossiper)
			//El paquete es un rumor
			//AHORA ESTA SEPARADO EN DOS CASOS, AHORA MISMO NO HACE FALTA PORQUE SON IGUALES, A LO MEJOR ES UTIL EN EL FUTURO
			//Tenemos una conexion abierta con el
			if exists == true {
				if checkingIfExpectedMessageAndSave(message, peerTalking, gossiper) == true {
					//Empezar rumormorgering process con otro peer
					choosenPeer := choseRandomPeerAndSendRumorPackage(message, gossiper)
					connectionCreationRumorMessage(choosenPeer, message, gossiper)

					//Mandar un mensaje de status de vuelta (todo ok)
					createNewStatusPackageAndSend(sender, gossiper)
					continue

				} else {
					//El paquete no es nuevo o es muy nuevo, mandar status de vuelta
					//No parece necesario renovar la conexion
					//connectionRenewal(peerTalking, gossiper)
					createNewStatusPackageAndSend(sender, gossiper)
					continue
				}

				//No tenemos una conexion abierta con el
			} else {
				if checkingIfExpectedMessageAndSave(message, peerTalking, gossiper) == true {
					//Empezar rumormorgering process con otro peer

					choosenPeer := choseRandomPeerAndSendRumorPackage(message, gossiper)
					connectionCreationRumorMessage(choosenPeer, message, gossiper)

					//Mandar un mensaje de status de vuelta (todo ok)
					createNewStatusPackageAndSend(sender, gossiper)
					continue

				} else {
					//El paquete no es nuevo o es muy nuevo, mandar status de vuelta
					createNewStatusPackageAndSend(sender, gossiper)
					continue
				}
			}
		case Private:
			//It is a private message
			message := packet.Private
			sendPrivateMessage(message, false, gossiper)
		case FileRequest:
			message := packet.DataRequest
			fileDataRequestManagement(message, gossiper)

		case FileReply:
			message := packet.DataReply
			fileDataReplyManagement(message, gossiper)
		}

	}
}
func connectionCreationRumorMessage(sender string, message *RumorMessage, gossiper *Gossiper) {
	gossiper.talkingPeersMutex.Lock()
	talkingPeersMap := gossiper.TalkingPeers
	talkingPeersMap[sender] = ConectionInfo{
		MessageToGossip: message,
		Timeout:         10,
	}
	gossiper.TalkingPeers = talkingPeersMap
	gossiper.talkingPeersMutex.Unlock()
}
func dataReplyCreationAndSend(message *DataRequest, gossiper *Gossiper) {

	hashFileRequested := message.HashValue
	dataReplyToSend := &DataReply{}

	//We are going to check all the hashes to see if we have it.
	for _, storedFile := range gossiper.StoredFiles {
		//Metafile is being requested
		storedMetahash := storedFile.MetaHash[:]
		if bytes.Equal(storedMetahash, hashFileRequested) {
			//fmt.Println("Enviando metafile: ", sha256.Sum256(storedFile.Metafile))
			dataReplyToSend = &DataReply{
				Origin:      gossiper.Name,
				Destination: message.Origin,
				HopLimit:    10,
				HashValue:   storedMetahash,
				Data:        storedFile.Metafile,
			}
			break
		}
		//One chunk is going to be requested
		for hashOfTheChunk, dataOfTheChunk := range storedFile.HashesOfChunks {
			hashOfChunkAux := hashOfTheChunk[:]
			if bytes.Equal(hashOfChunkAux, hashFileRequested) {
				//fmt.Println("Enviando chunk: ", sha256.Sum256(dataOfTheChunk))
				dataReplyToSend = &DataReply{
					Origin:      gossiper.Name,
					Destination: message.Origin,
					HopLimit:    10,
					HashValue:   hashOfChunkAux,
					Data:        dataOfTheChunk,
				}
				break
			}
		}

	}
	if dataReplyToSend.Data == nil {
		dataReplyToSend = &DataReply{
			Origin:      gossiper.Name,
			Destination: message.Origin,
			HopLimit:    10,
			HashValue:   message.HashValue,
		}
	}
	//Send the reply packet
	//If the hop limit has been exceeded
	if dataReplyToSend.HopLimit <= 0 {
		return
	}
	//Just to check the message is not going to us
	if dataReplyToSend.Destination != gossiper.Name {
		dataReplyToSend.HopLimit = dataReplyToSend.HopLimit - 1
	}
	gossiper.routingTableMutex.RLock()
	nextHop := gossiper.RoutingTable[dataReplyToSend.Destination]
	gossiper.routingTableMutex.RUnlock()
	addressNextHop, _ := net.ResolveUDPAddr("udp", nextHop)
	packetToSend := &GossipPacket{DataReply: dataReplyToSend}
	packetBytes, err := protobuf.Encode(packetToSend)
	if err != nil {
		panic(err)
	}
	gossiper.Socket.Conn.WriteToUDP(packetBytes, addressNextHop)
	//time.Sleep(5 * time.Second)
}
func fileDataRequestManagement(message *DataRequest, gossiper *Gossiper) {
	//We are the owners of the file being requested
	if message.Destination == gossiper.Name {
		dataReplyCreationAndSend(message, gossiper)

		//The file request is for someone else
	} else {
		sendDataRequest(message, gossiper)
	}
}
func sendDataRequest(message *DataRequest, gossiper *Gossiper) {
	//If the hop limit has been exceeded
	if message.HopLimit <= 0 {
		return
	}
	//Just to check the message is not going to us
	if message.Destination != gossiper.Name {
		message.HopLimit = message.HopLimit - 1
	}

	//If not send to next hop
	gossiper.routingTableMutex.RLock()
	nextHop := gossiper.RoutingTable[message.Destination]
	gossiper.routingTableMutex.RUnlock()
	addressNextHop, _ := net.ResolveUDPAddr("udp", nextHop)
	packetToSend := &GossipPacket{DataRequest: message}
	packetBytes, err := protobuf.Encode(packetToSend)
	if err != nil {
		panic(err)
	}
	gossiper.Socket.Conn.WriteToUDP(packetBytes, addressNextHop)
}
func fileDataReplyManagement(message *DataReply, gossiper *Gossiper) {

	//The reply is sent to us because we sent a request at some point
	if message.Destination == gossiper.Name {
		dataDownloadManagement(message, gossiper)

		//The file reply is for someone else, just route
	} else {

		//If the hop limit has been exceeded
		if message.HopLimit <= 0 {
			return
		}
		message.HopLimit = message.HopLimit - 1

		//If not send to next hop
		gossiper.routingTableMutex.RLock()
		nextHop := gossiper.RoutingTable[message.Destination]
		gossiper.routingTableMutex.RUnlock()
		addressNextHop, _ := net.ResolveUDPAddr("udp", nextHop)
		packetToSend := &GossipPacket{DataReply: message}
		packetBytes, err := protobuf.Encode(packetToSend)
		if err != nil {
			panic(err)
		}
		gossiper.Socket.Conn.WriteToUDP(packetBytes, addressNextHop)
	}

}
func dataDownloadManagement(message *DataReply, gossiper *Gossiper) {

	//If this is not done instead of saving the value it saves a reference
	data := make([]byte, len(message.Data))
	copy(data, message.Data)
	origin := message.Origin
	sha256fDataAux := sha256.Sum256(data)
	sha256fData := sha256fDataAux[:]
	//Find the session
	var session *DownloadInfo
	var index int
	gossiper.filesBeingDownloadedMutex.RLock()
	for oneIndex, oneSession := range gossiper.FilesBeingDownloaded {
		if bytes.Equal(oneSession.LastHashRequested, message.HashValue) {
			session = oneSession
			index = oneIndex
			break
		}
	}
	gossiper.filesBeingDownloadedMutex.RUnlock()
	sha256Expected := session.LastHashRequested
	//Check is he has the file (the data field is empty)
	if checkPeerHasFile(data, index, gossiper) == false {
		//If the does not have it delete session and leave method
		return
	}

	//Check if the file has being received correctly or is not the one we are looking for
	if !bytes.Equal(sha256fData, sha256Expected) {

		//TODO: Do something if not
		return
	} else {
		//Check if the received hash is of the metahash
		//If it is the metahash
		if bytes.Equal(sha256fData, session.MetaHash) {
			//Save the info also to the gossiper struct
			var chunk ChunkStruct
			for i := 0; i < len(data); i += 32 {
				chunkAux := data[i : i+32]
				//We have they keys of the map (hashes) but not the values
				chunk.ChunkHash = chunkAux
				session.ChunkInformation = append(session.ChunkInformation, chunk)
			}

			//We are going to ask for the first chunk
			gossiper.filesBeingDownloadedMutex.Lock()
			gossiper.FilesBeingDownloaded[index] = &DownloadInfo{
				PathToSave:        session.PathToSave,
				FileName:          session.FileName,
				Timeout:           5,
				Metafile:          data,
				MetaHash:          session.MetaHash,
				LastHashRequested: session.ChunkInformation[0].ChunkHash,
				ChunkInformation:  session.ChunkInformation,
				Destination:       session.Destination,
			}
			gossiper.filesBeingDownloadedMutex.Unlock()
			dataRequest := &DataRequest{
				Destination: origin,
				HopLimit:    10,
				Origin:      gossiper.Name,
				HashValue:   session.ChunkInformation[0].ChunkHash,
			}
			var metaHashAux [32]byte
			copy(metaHashAux[:], session.MetaHash)
			fileToStore := &FileInfo{
				MetaHash:       metaHashAux,
				Metafile:       data,
				HashesOfChunks: make(map[[32]byte][]byte),
			}
			gossiper.StoredFiles[session.FileName] = fileToStore
			sendDataRequest(dataRequest, gossiper)
			//Is the hash of a chunk of the file
		} else {

			var positionLastChunkReceived int
			for index2, oneChunk := range session.ChunkInformation {
				//If check to which chunk we are receiving the data
				if bytes.Equal(oneChunk.ChunkHash, sha256fData) {

					session.ChunkInformation[index2].ChunkData = data
					positionLastChunkReceived = index2
					break
				}
			}

			//Have to save the last info in case is the last chunk
			gossiper.filesBeingDownloadedMutex.Lock()
			gossiper.FilesBeingDownloaded[index] = session
			gossiper.filesBeingDownloadedMutex.Unlock()
			//Check if we have files left to download
			//If we do
			if (positionLastChunkReceived+1 < len(session.ChunkInformation)) && (session.ChunkInformation[positionLastChunkReceived+1].ChunkData) == nil {
				//Request next chunk
				gossiper.filesBeingDownloadedMutex.Lock()
				gossiper.FilesBeingDownloaded[index] = &DownloadInfo{
					PathToSave:        session.PathToSave,
					FileName:          session.FileName,
					Timeout:           5,
					Metafile:          session.Metafile,
					MetaHash:          session.MetaHash,
					LastHashRequested: session.ChunkInformation[positionLastChunkReceived+1].ChunkHash,
					ChunkInformation:  session.ChunkInformation,
					Destination:       session.Destination,
				}
				gossiper.filesBeingDownloadedMutex.Unlock()
				dataRequest := &DataRequest{
					Destination: origin,
					HopLimit:    10,
					Origin:      gossiper.Name,
					HashValue:   session.ChunkInformation[positionLastChunkReceived+1].ChunkHash,
				}

				var metaChunkAux [32]byte
				copy(metaChunkAux[:], message.HashValue)

				gossiper.StoredFiles[session.FileName].HashesOfChunks[metaChunkAux] = data
				gossiper.filesBeingDownloadedMutex.RLock()
				fmt.Println("DOWNLOADING " + gossiper.FilesBeingDownloaded[index].FileName + " chunk " + strconv.Itoa(positionLastChunkReceived+1) + " from " + gossiper.FilesBeingDownloaded[index].Destination)
				gossiper.filesBeingDownloadedMutex.RUnlock()
				sendDataRequest(dataRequest, gossiper)
			} else {
				fileDownloadedSuccessfully(index, message, gossiper)
			}
		}

	}
}
func checkPeerHasFile(dataOfMessage []byte, index int, gossiper *Gossiper) bool {
	if dataOfMessage == nil || len(dataOfMessage) == 0 {
		//Close session
		gossiper.filesBeingDownloadedMutex.Lock()
		gossiper.FilesBeingDownloaded = append(gossiper.FilesBeingDownloaded[:index], gossiper.FilesBeingDownloaded[index+1:]...)
		gossiper.filesBeingDownloadedMutex.Unlock()
		return false
	}
	return true
}
func fileDownloadedSuccessfully(index int, message *DataReply, gossiper *Gossiper) {

	gossiper.filesBeingDownloadedMutex.RLock()
	session := gossiper.FilesBeingDownloaded[index]

	pathToSaveFinalFile := session.PathToSave
	var finalFile []byte

	for i := 0; i < len(session.ChunkInformation); i++ {
		//Join all the data from the chunks and save it to finalFile
		chunkData2 := session.ChunkInformation[i].ChunkData
		finalFile = append(finalFile, chunkData2...)
	}
	gossiper.filesBeingDownloadedMutex.RUnlock()
	//Save file in filesystem
	ioutil.WriteFile(pathToSaveFinalFile, finalFile, 0644)

	fmt.Println("RECONSTRUCTED file " + session.FileName)

	//Update available files
	var metaHashAux [32]byte
	copy(metaHashAux[:], session.MetaHash)
	fileToStore := &FileInfo{
		FileSize:       int64(len(finalFile)),
		MetaHash:       metaHashAux,
		Metafile:       session.Metafile,
		HashesOfChunks: make(map[[32]byte][]byte),
	}

	gossiper.StoredFiles[session.FileName] = fileToStore

	var chunkHashAux [32]byte
	for i := 0; i < len(session.ChunkInformation); i++ {
		chunkHash := session.ChunkInformation[i].ChunkHash
		chunkData := session.ChunkInformation[i].ChunkData
		copy(chunkHashAux[:], chunkHash)
		gossiper.StoredFiles[session.FileName].HashesOfChunks[chunkHashAux] = chunkData
	}

	//Close session
	gossiper.filesBeingDownloadedMutex.Lock()
	gossiper.FilesBeingDownloaded = append(gossiper.FilesBeingDownloaded[:index], gossiper.FilesBeingDownloaded[index+1:]...)
	gossiper.filesBeingDownloadedMutex.Unlock()
}
func fileDownloadRequest(packet *Message, gossiper *Gossiper) {
	message := &DataRequest{
		Origin:      gossiper.Name,
		Destination: *packet.Destination,
		HashValue:   *packet.Request,
		HopLimit:    10,
	}
	//If the hop limit has been exceeded
	if message.HopLimit <= 0 {
		return
	}
	//Just to check the message is not going to us
	if message.Destination != gossiper.Name {
		message.HopLimit = message.HopLimit - 1
	}

	//Open sesion of download
	gossiper.filesBeingDownloadedMutex.Lock()
	gossiper.FilesBeingDownloaded = append(gossiper.FilesBeingDownloaded, &DownloadInfo{
		FileName:          *packet.File,
		PathToSave:        filepath.Join(Downloads, *packet.File),
		Timeout:           5,
		LastHashRequested: message.HashValue,
		MetaHash:          message.HashValue,
		Destination:       message.Destination,
	})
	gossiper.filesBeingDownloadedMutex.Unlock()
	//If not send to next hop
	gossiper.routingTableMutex.RLock()
	nextHop := gossiper.RoutingTable[message.Destination]
	gossiper.routingTableMutex.RUnlock()
	addressNextHop, _ := net.ResolveUDPAddr("udp", nextHop)
	packetToSend := &GossipPacket{DataRequest: message}
	packetBytes, err := protobuf.Encode(packetToSend)
	if err != nil {
		panic(err)
	}
	fmt.Println("DOWNLOADING metafile of " + *packet.File + " from " + message.Destination)
	gossiper.Socket.Conn.WriteToUDP(packetBytes, addressNextHop)
}
func connectionRenewal(sender string, gossiper *Gossiper) {
	gossiper.talkingPeersMutex.Lock()
	defer gossiper.talkingPeersMutex.Unlock()
	_, ok := gossiper.TalkingPeers[sender]
	if ok {
		talkingPeersMap := gossiper.TalkingPeers
		talkingPeersMap[sender] = ConectionInfo{
			MessageToGossip: gossiper.TalkingPeers[sender].MessageToGossip,
			Timeout:         10,
		}
	} else {
		return
	}
}

func listenUISocketNotSimple(UISocket *GossiperSocket, gossiper *Gossiper) {

	for {
		buf := make([]byte, 2000)

		_, _, err := UISocket.Conn.ReadFromUDP(buf)

		if err != nil {
			panic(err)
		}
		packet := &Message{}
		protobuf.Decode(buf, packet)

		//Request file
		if packet.File != nil && packet.Destination != nil {

			fileDownloadRequest(packet, gossiper)
			continue
		}
		//Share file
		if packet.File != nil {
			fileToBeChunked := *packet.File
			fileIndexing(fileToBeChunked, gossiper)
			continue
		}

		//Private message
		if packet.Destination != nil {
			privateMessageCreation(packet, gossiper)
			continue
		}
		//Normal message
		gossiper.wantMutex.Lock()

		IDMessage := gossiper.Want[0].NextID
		message := RumorMessage{
			Origin: gossiper.Name,
			ID:     IDMessage,
			Text:   packet.Text,
		}
		gossiper.Want[0].NextID = gossiper.Want[0].NextID + 1
		gossiper.wantMutex.Unlock()
		gossiper.savedMessagesMutex.Lock()
		messaggesOfClient := gossiper.SavedMessages[gossiper.Name]
		messaggesOfClient = append(messaggesOfClient, message)
		gossiper.SavedMessages[gossiper.Name] = messaggesOfClient
		gossiper.savedMessagesMutex.Unlock()
		fmt.Println("CLIENT MESSAGE " + message.Text)
		sendToPeersComingFromClientNotSimple(&message, gossiper)
	}
}
func checkPeersList(sender *net.UDPAddr, gossiper *Gossiper) bool {
	peerAddress := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
	for _, peerAddressExaminated := range gossiper.KnownPeers {
		if peerAddress == peerAddressExaminated {
			return true
		}
	}
	return false
}
func checkPeersListSimple(relayAddress string, gossiper *Gossiper) bool {
	peerAddress := relayAddress
	for _, peerAddressExaminated := range gossiper.KnownPeers {
		if peerAddress == peerAddressExaminated {
			return true
		}
	}
	return false
}
func fileIndexing(fileToBeChunked string, gossiper *Gossiper) {

	var hashesOfChunks [][32]byte
	file, err := os.Open(filepath.Join(SharedFiles, fileToBeChunked))

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	defer file.Close()
	fileInfo, _ := file.Stat()
	var fileSize int64 = fileInfo.Size()

	//8 KB of chunk sice
	const fileChunk = 1 * (1 << 13)

	// calculate total number of parts the file will be chunked into
	totalPartsNum := uint64(math.Ceil(float64(fileSize) / float64(fileChunk)))

	//fmt.Printf("Splitting to %d pieces.\n", totalPartsNum)
	//Save the size of the original file
	gossiper.StoredFiles[fileToBeChunked] = &FileInfo{}
	gossiper.StoredFiles[fileToBeChunked].HashesOfChunks = make(map[[32]byte][]byte)
	gossiper.StoredFiles[fileToBeChunked].FileSize = fileSize
	for i := uint64(0); i < totalPartsNum; i++ {

		partSize := int(math.Min(fileChunk, float64(fileSize-int64(i*fileChunk))))
		partBuffer := make([]byte, partSize)
		//Read the chunk
		file.Read(partBuffer)

		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		//We create a new stored file with key the name of the file and then fill the map of hashed
		gossiper.StoredFiles[fileToBeChunked].HashesOfChunks[sha256.Sum256(partBuffer)] = partBuffer

		//Also store the hash in an array to calculate the metahash without the need of looking at the dictionary of hashes
		hashesOfChunks = append(hashesOfChunks, sha256.Sum256(partBuffer))

	}
	calculateMetaHash(fileToBeChunked, hashesOfChunks, fileSize, gossiper)

}
func calculateMetaHash(fileToBeChunked string, hashesOfChunks [][32]byte, fileSize int64, gossiper *Gossiper) error {

	var dataOfMetahashBytes []byte

	//Here we create the content of the metafile
	for _, hashOfChunk := range hashesOfChunks {
		dataOfMetahashBytes = append(dataOfMetahashBytes, hashOfChunk[:]...)
	}
	gossiper.StoredFiles[fileToBeChunked].Metafile = dataOfMetahashBytes
	gossiper.StoredFiles[fileToBeChunked].MetaHash = sha256.Sum256(dataOfMetahashBytes)
	/*a := sha256.Sum256(dataOfMetahashBytes)
	b := a[:]
	fmt.Println(hex.EncodeToString(b))*/
	return nil
}
func packetType(packet GossipPacket) int {

	if packet.Simple != nil {
		return Simple
	}
	if packet.Rumor != nil {
		return Rumor
	}
	if packet.Status != nil {
		return Status
	}
	if packet.Private != nil {
		return Private
	}
	if packet.DataReply != nil {
		return FileReply
	}
	if packet.DataRequest != nil {
		return FileRequest
	}
	return -1
}
func nextHopManagement(message *RumorMessage, peerTalking string, gossiper *Gossiper) {
	//TODO: Ask if we should update it if the ID is higher than expected, at the moment it does so
	//TODO: Ask what no output of DSDV messages mean, do we refresh the table without printing?
	origin := message.Origin
	// We take the last message of the peer and check the ID
	/*gossiper.savedMessagesMutex.RLock()
	_, ok := gossiper.SavedMessages[origin]
	gossiper.savedMessagesMutex.RUnlock()*/
	gossiper.routingTableControlMutex.RLock()
	_, ok2 := gossiper.RoutingTableControl[origin]
	gossiper.routingTableControlMutex.RUnlock()
	//First time we see it
	if !ok2 {
		gossiper.routingTableControlMutex.Lock()
		gossiper.RoutingTableControl[origin] = 1
		gossiper.routingTableControlMutex.Unlock()
		gossiper.routingTableMutex.Lock()
		gossiper.RoutingTable[origin] = peerTalking
		gossiper.routingTableMutex.Unlock()
		if message.Text != "" {
			fmt.Println("DSDV " + origin + " " + peerTalking)
		}
		return
	}
	//If newer message save it
	if message.ID > gossiper.RoutingTableControl[origin] {
		gossiper.routingTableControlMutex.Lock()
		gossiper.RoutingTableControl[origin] = message.ID
		gossiper.routingTableControlMutex.Unlock()
		gossiper.routingTableMutex.Lock()
		gossiper.RoutingTable[origin] = peerTalking
		gossiper.routingTableMutex.Unlock()
		if message.Text != "" {
			fmt.Println("DSDV " + origin + " " + peerTalking)
		}
	}
	/*
		//If it doesn't exist (first message from him) we save it
		if !ok {
			gossiper.routingTableMutex.Lock()
			gossiper.RoutingTable[origin] = peerTalking
			gossiper.routingTableMutex.Unlock()

			if message.Text != "" {
				fmt.Println("DSDV " + origin + " " + peerTalking)
			}
			return
			//If the received message is newer than the one we have

		} else {
			gossiper.routingTableMutex.Lock()
			gossiper.RoutingTable[origin] = peerTalking
			gossiper.routingTableMutex.Unlock()

			if message.Text != "" {
				fmt.Println("DSDV " + origin + " " + peerTalking)
			}
			return
		}*/
}
func privateMessageCreation(clientMessage *Message, gossiper *Gossiper) {
	privateMessage := &PrivateMessage{
		Origin:      gossiper.Name,
		ID:          0,
		Text:        clientMessage.Text,
		Destination: *clientMessage.Destination,
		HopLimit:    10,
	}

	sendPrivateMessage(privateMessage, true, gossiper)
}
func sendPrivateMessage(message *PrivateMessage, ourClient bool, gossiper *Gossiper) {
	if ourClient {
		fmt.Println("CLIENT MESSAGE " + message.Text + " dest " + message.Destination)
	}
	if message.Destination == gossiper.Name {
		//_, ok := gossiper.savedPrivateMessages[message.Origin]
		//Save it
		//if ok {
		privateMessagesOfPeer := gossiper.savedPrivateMessages[message.Origin]
		privateMessagesOfPeer = append(privateMessagesOfPeer, *message)
		gossiper.savedPrivateMessages[message.Origin] = privateMessagesOfPeer
		/*} else {
			//First private message of that peer
			gossiper.savedPrivateMessages[message.Origin] = *message
		}*/
		//if the sender was our client
		if !ourClient {
			fmt.Println("PRIVATE origin " + message.Origin + " hop-limit " + strconv.FormatUint(uint64(message.HopLimit), 10) + " contents " + message.Text)
		}
		//The message is for someone else
	} else {

		//If the hop limit has been exceeded
		if message.HopLimit <= 0 {
			return
		}
		message.HopLimit = message.HopLimit - 1

		//If not send to next hop
		gossiper.routingTableMutex.RLock()
		nextHop := gossiper.RoutingTable[message.Destination]
		gossiper.routingTableMutex.RUnlock()
		addressNextHop, _ := net.ResolveUDPAddr("udp", nextHop)
		packetToSend := &GossipPacket{Private: message}
		packetBytes, err := protobuf.Encode(packetToSend)
		if err != nil {
			panic(err)
		}
		gossiper.Socket.Conn.WriteToUDP(packetBytes, addressNextHop)
	}
}
func createAndSendNextHopMessage(gossiper *Gossiper) {
	gossiper.wantMutex.Lock()
	IDMessage := gossiper.Want[0].NextID
	nextHopMessage := &RumorMessage{
		Origin: gossiper.Name,
		ID:     IDMessage,
		Text:   "",
	}
	gossiper.Want[0].NextID = gossiper.Want[0].NextID + 1
	gossiper.wantMutex.Unlock()

	gossiper.savedMessagesMutex.Lock()
	gossiper.SavedMessages[gossiper.Name] = append(gossiper.SavedMessages[gossiper.Name], *nextHopMessage)
	gossiper.savedMessagesMutex.Unlock()

	choseRandomPeerAndSendRumorPackage(nextHopMessage, gossiper)
}
func addPeerToListSimple(relayAddress string, gossiper *Gossiper) {
	peerAddress := relayAddress
	gossiper.KnownPeers = append(gossiper.KnownPeers, peerAddress)
}
func addPeerToList(sender *net.UDPAddr, gossiper *Gossiper) {
	peerAddress := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
	gossiper.KnownPeers = append(gossiper.KnownPeers, peerAddress)
}
func sendToPeersComingFromClient(message *SimpleMessage, gossiper *Gossiper) {
	packetToSend := &GossipPacket{Simple: message}
	fmt.Print("PEERS ")
	packetBytes, _ := protobuf.Encode(packetToSend)
	for index, peerAddress := range gossiper.KnownPeers {
		choosenPeerAddress, _ := net.ResolveUDPAddr("udp", peerAddress)
		gossiper.Socket.Conn.WriteToUDP(packetBytes, choosenPeerAddress)
		if index == (len(gossiper.KnownPeers) - 1) {
			fmt.Println(peerAddress)
			continue
		}
		if peerAddress != "" {
			fmt.Print(peerAddress + ",")
			continue
		}
	}
}
func printKnownPeers(gossiper *Gossiper) {
	fmt.Print("PEERS ")
	for index, peerAddress := range gossiper.KnownPeers {
		if index == (len(gossiper.KnownPeers) - 1) {
			fmt.Println(peerAddress)
			continue
		}
		//Just in case we don´t have peers
		if peerAddress != "" {
			fmt.Print(peerAddress + ",")
		}
	}
}
func sendToPeersComingFromClientNotSimple(message *RumorMessage, gossiper *Gossiper) {

	choosenPeer := choseRandomPeerAndSendRumorPackage(message, gossiper)
	connectionCreationRumorMessage(choosenPeer, message, gossiper)

}
func sendToPeersComingFromPeer(message *SimpleMessage, gossiper *Gossiper) {
	originalRelay := message.RelayPeerAddr
	message.RelayPeerAddr = gossiper.Addr
	packetToSend := &GossipPacket{Simple: message}
	for _, peerAddress := range gossiper.KnownPeers {
		if peerAddress == originalRelay {
			//Not broadcast back to him
			continue
		} else {
			packetBytes, _ := protobuf.Encode(packetToSend)
			choosenPeerAddress, _ := net.ResolveUDPAddr("udp", peerAddress)
			gossiper.Socket.Conn.WriteToUDP(packetBytes, choosenPeerAddress)
		}
	}
}
func checkingIfExpectedMessageAndSave(message *RumorMessage, peerTalking string, gossiper *Gossiper) bool {
	origin := message.Origin
	ID := message.ID
	check := true

	gossiper.wantMutex.RLock()
	for _, peerStatusExaminated := range gossiper.Want {

		if peerStatusExaminated.Identifier == origin {
			check = false
		}
	}
	gossiper.wantMutex.RUnlock()
	if check == true {
		wantInfo := PeerStatus{
			Identifier: origin,
			NextID:     1,
		}
		gossiper.wantMutex.Lock()
		gossiper.Want = append(gossiper.Want, wantInfo)
		gossiper.wantMutex.Unlock()
	}
	gossiper.wantMutex.RLock()
	for index, peerStatusExaminated := range gossiper.Want {
		if origin == peerStatusExaminated.Identifier {
			if ID == (peerStatusExaminated.NextID) {
				//Save the new index and save the package
				gossiper.wantMutex.RUnlock()
				gossiper.savedMessagesMutex.Lock()
				messaggesOfOrigin := gossiper.SavedMessages[origin]
				//It is important that they are stored in incoming order
				messaggesOfOrigin = append(messaggesOfOrigin, *message)
				gossiper.SavedMessages[origin] = messaggesOfOrigin
				gossiper.savedMessagesMutex.Unlock()
				gossiper.wantMutex.Lock()
				gossiper.Want[index].Identifier = origin
				gossiper.Want[index].NextID = ID + 1
				gossiper.wantMutex.Unlock()
				return true
			}
		}
	}
	gossiper.wantMutex.RUnlock()
	return false

}
func createNewStatusPackageAndSend(sender *net.UDPAddr, gossiper *Gossiper) {
	gossiper.wantMutex.RLock()
	statusPacket := &StatusPacket{Want: gossiper.Want}
	gossiper.wantMutex.RUnlock()
	packetToSend := &GossipPacket{Status: statusPacket}

	packetBytes, err := protobuf.Encode(packetToSend)
	if err != nil {
		panic(err)
	}
	gossiper.Socket.Conn.WriteToUDP(packetBytes, sender)
}
func sendRumorPackage(rumorMessage *RumorMessage, sender *net.UDPAddr, gossiper *Gossiper) {
	packetToSend := &GossipPacket{Rumor: rumorMessage}
	packetBytes, err := protobuf.Encode(packetToSend)
	//senderAddress := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
	//fmt.Println("MONGERING with " + senderAddress)
	gossiper.Socket.Conn.WriteToUDP(packetBytes, sender)
	if err != nil {
		panic(err)
	}
}
func statusDecisionMaking(sender *net.UDPAddr, statusMessage *StatusPacket, gossiper *Gossiper) {
	/*senderAddress := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
	stringToSend := ""
	for _, peerWantStatus := range statusMessage.Want {
		stringToSend = stringToSend + " peer " + peerWantStatus.Identifier + " nextID " + fmt.Sprint(peerWantStatus.NextID)
	}
	fmt.Println("STATUS from " + senderAddress + stringToSend)*/

	//No need for lock, only reading in slice
	for _, peerStatusExaminated := range gossiper.Want {
		newPeer := true
		for _, peerStatusExaminatedOfOtherPeer := range statusMessage.Want {
			if peerStatusExaminated.Identifier == peerStatusExaminatedOfOtherPeer.Identifier {
				newPeer = false
				//Estamos mirando la misma persona tanto en memoria local como en el otro peer
				if peerStatusExaminated.NextID == (peerStatusExaminatedOfOtherPeer.NextID) {
					continue
					//Tenemos los mismos mensajes EN LA PERSONA QUE ESTAMOS MIRANDO

				} else if peerStatusExaminated.NextID > peerStatusExaminatedOfOtherPeer.NextID {
					//Tenemos mas mensajes que el otro, es decir podemos enviar mas, mandar rumour package
					gossiper.savedMessagesMutex.RLock()
					for _, rumorMesagesOfAPeer := range gossiper.SavedMessages[peerStatusExaminated.Identifier] {
						if rumorMesagesOfAPeer.ID == (peerStatusExaminatedOfOtherPeer.NextID) {

							//Se busca el mensaje que el otro no tiene y se le manda
							rumorMessage := rumorMesagesOfAPeer
							gossiper.savedMessagesMutex.RUnlock()
							sendRumorPackage(&rumorMessage, sender, gossiper)
							return
						}
					}
					gossiper.savedMessagesMutex.RUnlock()
				}
			}
		}
		if newPeer {
			//El otro no tiene conocimiento de este hombre, le mando el primer mensaje de el
			if peerStatusExaminated.NextID != 1 {
				gossiper.savedMessagesMutex.RLock()
				rumorMessage := gossiper.SavedMessages[peerStatusExaminated.Identifier][0]
				gossiper.savedMessagesMutex.RUnlock()
				sendRumorPackage(&rumorMessage, sender, gossiper)
				return
			}

		}
	}
	//Si llegamos aqui es porque no tenemos ningun mensaje mas que el otro
	for _, peerStatusExaminated := range gossiper.Want {
		for _, peerStatusExaminatedOfOtherPeer := range statusMessage.Want {
			if peerStatusExaminated.Identifier == peerStatusExaminatedOfOtherPeer.Identifier {
				//Estamos mirando la misma persona tanto en memoria local como en el otro peer
				if peerStatusExaminated.NextID == (peerStatusExaminatedOfOtherPeer.NextID) {
					continue
					//Tenemos los mismos mensajes EN LA PERSONA QUE ESTAMOS MIRANDO
				} else if peerStatusExaminated.NextID < peerStatusExaminatedOfOtherPeer.NextID {
					//Tenemos menos mensajes que el otro, tenemos que pedirle, mandar status package
					createNewStatusPackageAndSend(sender, gossiper)
					return
				}
			}
		}
	}
	//fmt.Println("IN SYNC WITH " + senderAddress)
	//Si hemos llegado aqui es porque tenemos todo en igualdad de condiciones a si que hay que tirar moneda

	result := rand.Int()
	if (result % 2) == 0 {
		//Cerrar el objeto conexion con quien estamos hablando y abrir una nueva con el afortunado
		senderAddress := sender.IP.String() + ":" + strconv.Itoa(sender.Port)

		//Aqui se deberia entrar siempre excepto si es el caso de antientropia
		//Que te escriben un status packet y teneis los mismos paquetes
		//O si alguien te escribe y no te vale ningun paquete y tu le das tuyos
		gossiper.talkingPeersMutex.RLock()
		_, ok := gossiper.TalkingPeers[senderAddress]
		gossiper.talkingPeersMutex.RUnlock()
		if ok {
			//Recuperamos el mensaje
			gossiper.talkingPeersMutex.Lock()
			messageInClosingSesion := gossiper.TalkingPeers[senderAddress].MessageToGossip
			//Cerramos la sesion
			delete(gossiper.TalkingPeers, senderAddress)
			gossiper.talkingPeersMutex.Unlock()
			//Elegimos nuevo peer y le mandamos la sesion
			choosenPeer := choseRandomPeerAndSendRumorPackage(messageInClosingSesion, gossiper)
			connectionCreationRumorMessage(choosenPeer, messageInClosingSesion, gossiper)
			//fmt.Println("FLIPPED COIN sending rumor to " + choosenPeer)
		}

	} else {
		//Borramos la conexion y nos quedamos quietos
		senderAddress := sender.IP.String() + ":" + strconv.Itoa(sender.Port)
		gossiper.talkingPeersMutex.Lock()
		delete(gossiper.TalkingPeers, senderAddress)
		gossiper.talkingPeersMutex.Unlock()
	}
}

func createNewGossiper(gossipAddr *string, gossiperName *string, gossiperMode *bool, uiPort *int, peerList *string, antiEntropyTimeout *int, rtimer *int) *Gossiper {

	splitterAux := strings.Split(*peerList, ",")
	wantList := make([]PeerStatus, 1)
	wantList[0] = PeerStatus{
		Identifier: *gossiperName,
		NextID:     1,
	}
	return &Gossiper{
		Addr:                 *gossipAddr,
		UIPort:               *uiPort,
		Name:                 *gossiperName,
		KnownPeers:           splitterAux,
		Mode:                 *gossiperMode,
		Want:                 wantList,
		SavedMessages:        make(map[string][]RumorMessage),
		savedPrivateMessages: make(map[string][]PrivateMessage),
		TalkingPeers:         make(map[string]ConectionInfo),
		RoutingTable:         make(map[string]string),
		RoutingTableControl:  make(map[string]uint32),
		StoredFiles:          make(map[string]*FileInfo),
		FilesBeingDownloaded: make([]*DownloadInfo, 0),
		AntiEntropyTimeout:   *antiEntropyTimeout,
		Rtimer:               *rtimer,
	}
}
func antiEntropy(gossiper *Gossiper) {
	ticker := time.NewTicker(time.Duration(gossiper.AntiEntropyTimeout) * time.Second)
	for range ticker.C {
		choseRandomPeerAndSendStatusPackage(gossiper)
	}
}
func setRtimer(gossiper *Gossiper) {
	if gossiper.Rtimer != 0 {
		//First route message when the peerster starts
		createAndSendNextHopMessage(gossiper)
		ticker := time.NewTicker(time.Duration(gossiper.Rtimer) * time.Second)
		for range ticker.C {
			createAndSendNextHopMessage(gossiper)
		}
	}
}
func choseRandomPeerAndSendRumorPackage(message *RumorMessage, gossiper *Gossiper) string {

	randomIndexOfPeer := rand.Int() % len(gossiper.KnownPeers)
	peerToSendRumor := gossiper.KnownPeers[randomIndexOfPeer]
	packetToSend := &GossipPacket{Rumor: message}
	packetBytes, _ := protobuf.Encode(packetToSend)
	choosenPeerAddress, _ := net.ResolveUDPAddr("udp", peerToSendRumor)
	gossiper.Socket.Conn.WriteToUDP(packetBytes, choosenPeerAddress)
	//fmt.Println("MONGERING with " + peerToSendRumor)
	return peerToSendRumor
}
func choseRandomPeerAndSendStatusPackage(gossiper *Gossiper) {

	randomIndexOfPeer := rand.Int() % len(gossiper.KnownPeers)
	peerToSendStatus := gossiper.KnownPeers[randomIndexOfPeer]
	addrPeerToSendStatus, _ := net.ResolveUDPAddr("udp4", peerToSendStatus)
	createNewStatusPackageAndSend(addrPeerToSendStatus, gossiper)
}
func timeoutChecker(gossiper *Gossiper) {
	ticker := time.NewTicker(1 * time.Second)

	for range ticker.C {
		gossiper.talkingPeersMutex.Lock()
		for key, activePeerAnalyzed := range gossiper.TalkingPeers {
			if activePeerAnalyzed.Timeout <= 0 {
				delete(gossiper.TalkingPeers, key)
			} else {
				gossiper.TalkingPeers[key] = ConectionInfo{
					MessageToGossip: gossiper.TalkingPeers[key].MessageToGossip,
					Timeout:         gossiper.TalkingPeers[key].Timeout - 1,
				}
			}
		}
		gossiper.talkingPeersMutex.Unlock()
	}

}
func timeoutOfDownload(gossiper *Gossiper) {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		gossiper.filesBeingDownloadedMutex.Lock()
		for key, fileDownload := range gossiper.FilesBeingDownloaded {
			if gossiper.FilesBeingDownloaded[key] != nil {
				//fmt.Println("Timeout: ", gossiper.FilesBeingDownloaded[key].Timeout)
				if fileDownload.Timeout <= 0 {
					//Check if it is a chunk or a metafile and print it
					//We are asking for the metachunk
					if fileDownload.Metafile == nil {
						fmt.Println("DOWNLOADING metafile of " + fileDownload.FileName + " from " + fileDownload.Destination)
						//We are asking for a chunk. Should be enough with else but just in case
					} else {
						for index, chunk := range fileDownload.ChunkInformation {
							//This is the hash you are trying to download
							if bytes.Equal(chunk.ChunkHash, fileDownload.LastHashRequested) {
								//It is +2 because 1 is because arrays start at 0 and another because you are requesting the next one
								fmt.Println("DOWNLOADING " + fileDownload.FileName + " chunk " + strconv.Itoa(index+2) + " from " + fileDownload.Destination)
								break
							}
						}

					}
					message := &DataRequest{
						Origin:      gossiper.Name,
						Destination: fileDownload.Destination,
						HashValue:   fileDownload.LastHashRequested,
						HopLimit:    10,
					}
					//If the hop limit has been exceeded
					if message.HopLimit <= 0 {
						break
					}
					//Just to check the message is not going to us
					if message.Destination != gossiper.Name {
						message.HopLimit = message.HopLimit - 1
					}

					//Restart the timeout, we can use the same session object as before
					gossiper.FilesBeingDownloaded[key].Timeout = 5
					//If not send to next hop
					gossiper.routingTableMutex.RLock()
					nextHop := gossiper.RoutingTable[message.Destination]
					gossiper.routingTableMutex.RUnlock()
					addressNextHop, _ := net.ResolveUDPAddr("udp", nextHop)
					packetToSend := &GossipPacket{DataRequest: message}
					packetBytes, err := protobuf.Encode(packetToSend)
					if err != nil {
						panic(err)
					}
					gossiper.Socket.Conn.WriteToUDP(packetBytes, addressNextHop)
				} else {
					gossiper.FilesBeingDownloaded[key].Timeout = gossiper.FilesBeingDownloaded[key].Timeout - 1

				}
			}
		}
		gossiper.filesBeingDownloadedMutex.Unlock()
	}
}
func main() {

	gossiper := createNewGossiper(flagReader())
	rand.Seed(time.Now().UTC().UnixNano())
	UIsocket := gossiperUISocketOpen(gossiper.Addr, gossiper.UIPort)
	if gossiper.Mode == true {
		go listenUISocket(UIsocket, gossiper)
	} else {
		go timeoutChecker(gossiper)
		go listenUISocketNotSimple(UIsocket, gossiper)
	}
	socket := gossiperSocketOpen(gossiper.Addr)
	gossiper.Socket = socket
	if gossiper.Mode == true {
		go listenAPISocket(gossiper)
		listenSocket(socket, gossiper)
	} else {
		if gossiper.AntiEntropyTimeout != 0 {
			go antiEntropy(gossiper)
		}
		go setRtimer(gossiper)

		go timeoutOfDownload(gossiper)
		go listenAPISocket(gossiper)

		listenSocketNotSimple(socket, gossiper)

	}

}
