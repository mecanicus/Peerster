package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"

	"github.com/dedis/protobuf"
	. "github.com/mecanicus/Peerster/types"
)

//GossiperAPI a
//type GossiperAPI types.Gossiper

func (gossiper *Gossiper) gossiperIDHandler(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(gossiper.Name)

	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

func (gossiper *Gossiper) nodeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		js, _ := json.Marshal(gossiper.KnownPeers)

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
	if r.Method == "POST" {
		/*buffer := make([]byte, 2048)
		n, _ := r.Body.Read(buffer)
		s := string(buffer[:n])
		peer := strings.Split(s, "=")[1]
		fmt.Println(peer)
		gossiper.KnownPeers = append(gossiper.KnownPeers, peer)
		//Just to make Ajax happy
		js, _ := json.Marshal("Saved")
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)*/
		peer := r.FormValue("nodeText")
		//fmt.Println(peer)
		gossiper.KnownPeers = append(gossiper.KnownPeers, peer)
		//Just to make Ajax happy
		js, _ := json.Marshal("Saved")
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)

	}
}
func (gossiper *Gossiper) privateMessagesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		var origins []string
		for origin := range gossiper.RoutingTable {
			origins = append(origins, origin)
		}
		js, _ := json.Marshal(origins)

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
	if r.Method == "POST" {
		privateMessageText := r.FormValue("privateMessageString")
		selectedOrigin := r.FormValue("selectedOrigin")
		pointerSelectedOrigin := &selectedOrigin
		privateMessage := &PrivateMessage{
			Origin:      gossiper.Name,
			ID:          0,
			Text:        privateMessageText,
			Destination: *pointerSelectedOrigin,
			HopLimit:    10,
		}
		sendPrivateMessage(privateMessage, gossiper)
		js, _ := json.Marshal("Saved")

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
}
func (gossiper *Gossiper) requestFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		fileHashRequested, _ := hex.DecodeString(r.FormValue("requestedFileHash"))
		selectedOrigin := r.FormValue("selectedOrigin")
		fileName := r.FormValue("fileName")
		message := &DataRequest{
			Origin:      gossiper.Name,
			Destination: selectedOrigin,
			HashValue:   fileHashRequested,
			HopLimit:    10,
		}

		message.HopLimit = message.HopLimit - 1
		//If the hop limit has been exceeded
		if message.HopLimit <= 0 {
			return
		}
		//Open sesion of download
		gossiper.FilesBeingDownloaded = append(gossiper.FilesBeingDownloaded, &DownloadInfo{
			FileName:          fileName,
			PathToSave:        filepath.Join(Downloads, fileName),
			Timeout:           5,
			LastHashRequested: message.HashValue,
			MetaHash:          message.HashValue,
			Destination:       message.Destination,
		})
		//If not send to next hop
		nextHop := gossiper.RoutingTable[message.Destination]
		addressNextHop, _ := net.ResolveUDPAddr("udp", nextHop)
		packetToSend := &GossipPacket{DataRequest: message}
		packetBytes, err := protobuf.Encode(packetToSend)
		if err != nil {
			panic(err)
		}
		fmt.Println("DOWNLOADING metafile of " + fileName + " from " + message.Destination)
		gossiper.Socket.Conn.WriteToUDP(packetBytes, addressNextHop)
		js, _ := json.Marshal("Saved")

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
}
func (gossiper *Gossiper) filesUploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		fileName := r.FormValue("fileName")
		fmt.Println(fileName)
		fileIndexing(fileName, gossiper)
		js, _ := json.Marshal("Saved")

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)

	}
}
func (gossiper *Gossiper) messagesHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method == "GET" {
		var messages []string
		for peerName, messagesListPeer := range gossiper.SavedMessages {

			for _, message := range messagesListPeer {
				//To avoid showing the user route messages
				if message.Text != "" {
					stringToSend := "<strong>Peer Name: </strong>" + peerName
					stringToSend += "<strong> ID: </strong>" + fmt.Sprint(message.ID) + "<strong> Text: </strong>" + message.Text + " "
					messages = append(messages, stringToSend)
				}
			}

		}
		js, _ := json.Marshal(messages)

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
	if r.Method == "POST" {
		//buffer := make([]byte, 1024)
		//n, _ := r.Body.Read(buffer)
		//s := string(buffer[:n])
		messageText := r.FormValue("messageText")
		//messageText := strings.Split(s, "=")[1]

		IDMessage := gossiper.Want[0].NextID
		message := RumorMessage{
			Origin: gossiper.Name,
			ID:     IDMessage,
			Text:   messageText,
		}
		gossiper.Want[0].NextID = gossiper.Want[0].NextID + 1
		messaggesOfClient := gossiper.SavedMessages[gossiper.Name]
		messaggesOfClient = append(messaggesOfClient, message)
		gossiper.SavedMessages[gossiper.Name] = messaggesOfClient

		fmt.Println("CLIENT MESSAGE " + message.Text)
		sendToPeersComingFromClientNotSimple(&message, gossiper)

		js, _ := json.Marshal("Saved")

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)

	}
}

func listenAPISocket(gossiper *Gossiper) {

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/id", gossiper.gossiperIDHandler)
	http.HandleFunc("/messages", gossiper.messagesHandler)
	http.HandleFunc("/node", gossiper.nodeHandler)
	http.HandleFunc("/privateMessage", gossiper.privateMessagesHandler)
	http.HandleFunc("/fileUpload", gossiper.filesUploadHandler)
	http.HandleFunc("/requestFile", gossiper.requestFileHandler)

	http.ListenAndServe(":8080", nil)

}
