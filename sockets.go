package sockets

import (
	"fmt"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
)

//MainHub communicates from the socket reader into the socker writer and viceversa.
type MainHub struct {
	Request  chan request          //request is used to send messages from the reader to the writer
	Close    chan Conn             //close is sent from the reader to the main hub and passed to the writer to remove from it's collection of conns
	Clients  map[string]*ClientHub //Clients keeps a record of the active users.
	Register chan Conn
	Empty    chan string //if map is empty, it sends a signal to remove user from main hub
}

//ClientHub communicates with the main hub and writes to the connection
type ClientHub struct {
	ID          string
	Register    chan Conn
	Write       chan interface{}
	Broadcast   chan interface{}
	Request     chan request                 // receives a request from the main hub
	Close       chan Conn                    //receives a signal that the conn is closed so it's removed from the connections map
	Connections map[*websocket.Conn]struct{} // keeps the connection collection to write messages to.
	Handler     Handler                      // callback that takes the request
	Lock        sync.RWMutex
}

// Reader handles incoming socket connections
type Reader struct {
	ID string
}

type request struct {
	ID   string
	Msg  []byte
	Data interface{}
	Conn *websocket.Conn
}

// Request takes Data as anything and ID as string to group the connections
type Request struct {
	ID   string
	Msg  []byte
	Data interface{}
}

// Conn wraps a connection with an ID
type Conn struct {
	ID   string
	Conn *websocket.Conn
}

// Handler is a func that takes a request and exposes a broadcast function and a write function (writes to original conn only)
type Handler func(Request, func(interface{})) error

// write takes a channel and returns a function that sends to that channel
func broadcast(c chan interface{}) func(interface{}) {
	return func(i interface{}) {
		go func() { c <- i }()
	}
}

// NewMainHub creates a new MainHub. Returns a pointer.
func NewMainHub() *MainHub {
	return &MainHub{
		Register: make(chan Conn),
		Request:  make(chan request),
		Close:    make(chan Conn),
		Clients:  map[string]*ClientHub{},
		Empty:    make(chan string),
	}
}

// NewClientHub creates a new ClientHub. Returns a pointer.
func NewClientHub(id string, handler Handler) *ClientHub {
	return &ClientHub{
		ID:          id,
		Register:    make(chan Conn),
		Request:     make(chan request),
		Close:       make(chan Conn),
		Connections: map[*websocket.Conn]struct{}{},
		Handler:     handler,
		Write:       make(chan interface{}),
		Broadcast:   make(chan interface{}),
		Lock:        sync.RWMutex{},
	}
}

//Run starts the Main Hub go routine.
func (main *MainHub) Run(handler Handler) {
	for {
		select {

		//regsiter comes from reader
		//register receives the registration, creates the go routine if it doesn't exist and sends it to the client
		case register := <-main.Register: //#chan7
			// check if user exists
			_, ok := main.Clients[register.ID]
			if !ok {
				//the user doesn't exist in the map so we register it.
				main.Clients[register.ID] = NewClientHub(register.ID, handler)
				// this is the go routine that handles everything for one client
				go main.Clients[register.ID].Run(main)
			}
			//send the new connection to be registered
			main.Clients[register.ID].Register <- register //#chan1
		//req comes from reader
		case req := <-main.Request: //#chan6
			//check clients
			//pass req to client
			if client, ok := main.Clients[req.ID]; ok {
				client.Request <- req //#chan2
			}
		//close comes from reader
		case close := <-main.Close: //#chan5
			//close receives a Conn struct, send it to the client to close the connection
			if client, ok := main.Clients[close.ID]; ok {
				client.Close <- close //#chan3
			}
		//empty comes from writer. Sends the client ID
		case empty := <-main.Empty: //#chan4
			if _, ok := main.Clients[empty]; ok {
				delete(main.Clients, empty)
			}
		}

	}
}

//Run listens to the channels and handles connections, registrations and empty signals.
func (client *ClientHub) Run(main *MainHub) {
	for {
		select {
		case broadcast := <-client.Broadcast: //#chan8
			client.Lock.Lock()
			for c := range client.Connections {
				c.WriteJSON(broadcast)
			}
			client.Lock.Unlock()
		case register := <-client.Register: //#chan1
			//add the conn to the collection
			client.Connections[register.Conn] = struct{}{}
		case request := <-client.Request: //#chan2
			//pass the request to the handler
			err := client.Handler(Request{request.ID, request.Msg, request.Data}, broadcast(client.Broadcast)) //#chan8
			fmt.Println()
			if err != nil {
				client.Lock.Lock()
				request.Conn.WriteJSON(map[string]string{"error": err.Error()})
				client.Lock.Unlock()
			}
		case close := <-client.Close: //#chan3
			delete(client.Connections, close.Conn)

			//if it's empty, send to main hub an empty signal
			if len(client.Connections) == 0 {
				main.Empty <- client.ID //#chan4
				break
			}
		}
	}
}

//NewReader returns a websocket handler. Reads incoming messages and passess them around. It should receive a user string from Locals
func NewReader(main *MainHub) func(*fiber.Ctx) error {
	return websocket.New(func(c *websocket.Conn) {
		//read messages
		//handle closing of conns
		id, ok := c.Locals("socketsID").(string)
		if !ok || id == "" {
			id = "socketsPublic"
		}
		data := c.Locals("socketsData")
		//handle error

		con := Conn{id, c}
		defer func() {
			main.Close <- con //#chan5
			c.Close()
		}()
		//register the connection
		main.Register <- con //#chan7
		for {
			_, msg, err := c.ReadMessage()
			if err != nil {
				fmt.Println("Read err", err)
				break
			}

			//send request
			main.Request <- request{id, msg, data, c} //#chan6

		}
	})
}
