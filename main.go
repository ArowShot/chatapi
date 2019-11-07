package main

import (
	"container/list"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

// ClientRequest holds a message that the client sends directly to the server
type ClientRequest struct {
	ReqType string `json:"type"`
	Data    string `json:"data"`
}

// Client holds details about the connected clients
type Client struct {
	connection *websocket.Conn
	username   string
	chatRoom   *ChatRoom
}

// Message holds details about a message sent in a chat room
type Message struct {
	Text   string `json:"text"`
	Room   string `json:"room"`
	Sender string `json:"sender"`
}

// ChatRoom holds a list of messages and clients as well as a name
type ChatRoom struct {
	name     string
	messages *[]Message
	clients  *[]*Client
}

// Server holds all the connected clients, the chat room map, and a queue of messages to be processed
type Server struct {
	chatRooms    *map[string]*ChatRoom
	messageQueue *list.List
	clients      []*Client
}

func main() {
	chatRooms := make(map[string]*ChatRoom) //  Create the chatRooms map to use in server creation
	chatServer := Server{                   // Create the chat server
		chatRooms:    &chatRooms, // Using the chatRooms map we created
		messageQueue: list.New(), // and a new empty queue
	}

	clientIndex := 0 // Count the number of connected clinets to auto assign usernames

	go func() { // Process messages in the queue on another thread
		for { // Loop forever
			newMesssage := chatServer.messageQueue.Front() // Get the next message from the queue
			if newMesssage != nil {                        // If the message exists
				chatServer.messageQueue.Remove(newMesssage) // Remove it from the end of the queue
				message, ok := newMesssage.Value.(Message)  // Check that it is a Message type
				if ok {                                     // If it is
					fmt.Printf("got msg %v\n", message)
					destination := message.Room                             // Get the room name from the message
					room, roomFound := (*chatServer.chatRooms)[destination] // Get the room from the rooms map
					fmt.Printf("%v", room)
					if roomFound { // If the room exists
						*room.messages = append(*room.messages, message) // Add the message to the room
						for _, client := range *room.clients {           // Loop through the clients in the room
							fmt.Printf("Writing to client %v\n", client.username)
							client.connection.WriteJSON(message) // Send the message to all conneced clients in the room
						}
					}
				}
			}
		}
	}()

	upgrader := websocket.Upgrader{ // Upgrader is used to create a websocket connection
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) { // Handle new requests to the /ws route
		conn, err := upgrader.Upgrade(w, r, nil) // Upgrade the connection
		if err != nil {                          // If there is an error
			log.Println(err)
			return // Print it and stop processing
		}

		clientIndex++ // Increment the client count

		client := &Client{ // Create a new client
			connection: conn,                                // withthe connection refernece
			username:   fmt.Sprintf("user-%v", clientIndex), // and an auto-generated username
		}

		chatServer.clients = append(chatServer.clients, client) // Add the client to the clients array

		fmt.Printf("%v connected\n", client.username)

		go func() { // Process messages from this client on a new thread
			defer conn.Close() // Close then connection when it is over

			for { // Loop forever
				msg := ClientRequest{}
				err := conn.ReadJSON(&msg) // Read the message as json into the msg variable
				fmt.Println(msg)
				if err != nil { // If there is an error
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.Printf("error: %v", err) // Print if the error was an unexpected closing
					}
					break // Otherwise just skip the message
				}

				switch msg.ReqType { // Do different actions depending on the message type
				case "message": // If it is a "message"
					if client.chatRoom != nil { // If the client is in a room
						fmt.Printf("%v to %v: \"%v\"\n", client.username, client.chatRoom.name, msg.Data)
						chatServer.messageQueue.PushBack(Message{ // Add the message to the processing queue
							Room:   client.chatRoom.name,
							Sender: client.username,
							Text:   msg.Data,
						})
					}
				case "username": // If it is a "username"
					fmt.Printf("%v changing username to %v\n", client.username, msg.Data)
					client.username = msg.Data // Set the client's username
				case "join": // If it is a  "join"
					fmt.Printf("%v joining %v\n", client.username, msg.Data)
					if client.chatRoom != nil { // If the client is in a chat room
						// Remove it from that chat room
						for i, otherclient := range *client.chatRoom.clients {
							if otherclient == client {
								*client.chatRoom.clients = append((*client.chatRoom.clients)[:i], (*client.chatRoom.clients)[i+1:]...)
							}
						}
					}

					chatRoom, exists := (*chatServer.chatRooms)[msg.Data] // Get the chat room from the map
					if !exists {                                          // If it doesn't already exist
						messageList := make([]Message, 0) // make a Message array
						clientList := make([]*Client, 0)  // make a Client array
						chatRoom = &ChatRoom{             // Create the chat room with the lists
							name:     msg.Data,
							messages: &messageList,
							clients:  &clientList,
						}
						(*chatServer.chatRooms)[msg.Data] = chatRoom // Add the chat room to the map
					}
					client.chatRoom = chatRoom                            // Set the client's chat room
					*chatRoom.clients = append(*chatRoom.clients, client) // Add the client to the chat room's client array
					for _, msg := range *client.chatRoom.messages {       // Loop through the existing messages in the chat room
						client.connection.WriteJSON(msg) // And send the new client those old messages
					}
				}
			}
		}()
	})

	http.Handle("/", http.FileServer(http.Dir("./")))

	http.ListenAndServe("0.0.0.0:8080", nil) // Listen on port 8080
}
