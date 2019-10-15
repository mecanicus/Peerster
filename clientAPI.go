package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
		buffer := make([]byte, 1024)
		n, _ := r.Body.Read(buffer)
		s := string(buffer[:n])
		peer := strings.Split(s, "=")[1]
		fmt.Println(peer)
		gossiper.KnownPeers = append(gossiper.KnownPeers, peer)

		//Just to make Ajax happy
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
				stringToSend := "<strong>Peer Name: </strong>" + peerName
				stringToSend += "<strong> ID: </strong>" + fmt.Sprint(message.ID) + "<strong> Text: </strong>" + message.Text + " "
				messages = append(messages, stringToSend)

			}

		}
		js, _ := json.Marshal(messages)

		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}
	if r.Method == "POST" {
		buffer := make([]byte, 1024)
		n, _ := r.Body.Read(buffer)
		s := string(buffer[:n])
		messageText := strings.Split(s, "=")[1]

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
	http.ListenAndServe(":8080", nil)

}
