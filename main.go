package main

import (
	"time"
	"github.com/gorilla/websocket"
	"regexp"
	"strings"
	"net/http"
	"log"
	"flag"
	"html/template"
	"fmt"
)
const (
	writeWait = 10* time.Second
	pongWait = 60* time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMessageSize = 512
	authToken = "123456"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize: 1024,
	WriteBufferSize: 1024,
}

type connection struct {
	ws 			*websocket.Conn
	send 		chan []byte
	auth 		bool
	username 	[]byte
}

func (c *connection) write(mt int, payload []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return c.ws.WriteMessage(mt,payload)
}

type tmessage struct {
	content 		[]byte
	fromuser 		[]byte
	touser 			[]byte
	mtype 			int
	createtime 		string
}
type hub struct {
	connections 	map[*connection]bool
	broadcast 		chan *tmessage
	register 		chan *connection
	unregister 		chan *connection
}

var h = hub{
	broadcast: make(chan *tmessage),
	register: make(chan *connection),
	unregister: make(chan *connection),
	connections: make(map[*connection]bool),
}

func (c *connection) readPump()  {
	fmt.Println("readPump")
	defer func() {
		h.unregister <- c
		c.ws.Close()
	}()
	c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetWriteDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		c.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.ws.ReadMessage()

		if err != nil {
			break
		}
		mtype := 2
		text := string(message)
		fmt.Println("text: ",text)
		reg := regexp.MustCompile(`=[^&]+`)
		s := reg.FindAllString(text,-1)
		fmt.Println("s: ",s)
		if len(s) == 2 {
			fromuser := strings.Replace(s[0],"=", "",1)
			token := strings.Replace(s[1],"=","",1)
			if token == authToken {
				c.username = []byte(fromuser)
				c.auth = true
				message = []byte(fromuser + " join")
				mtype = 1
			}
		}
		touser := []byte("all")
		reg2 := regexp.MustCompile(`^@.*? `)
		s2 := reg2.FindAllString(text,-1)

		if len(s2) == 1 {
			s2[0] = strings.Replace(s2[0],"@","",1)
			s2[0] = strings.Replace(s2[0]," ", "" ,1)
			touser = []byte(s2[0])
		}
		if c.auth == true {
			t := time.Now().Unix()
			h.broadcast <- &tmessage{content:message,fromuser:c.username,touser:touser,mtype:mtype,createtime:time.Unix(t,0).String()}
		}
	}

}

func (c *connection) writePump()  {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()
	for {
		select {
		case message, ok := <- c.send:
			if !ok {
				c.write(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.write(websocket.TextMessage,message); err != nil {
				return
			}
		case <- ticker.C:
			if err := c.write(websocket.PingMessage,[]byte{}); err != nil {
				return
			}
		}
	}
}

//处理客户端对wensocket的请求
func serveWs(w http.ResponseWriter, r *http.Request)  {
	fmt.Println("serveWs client connect")
	ws, err := upgrader.Upgrade(w,r,nil)
	if err != nil {
		log.Panicln(err)
		return
	}
	c := &connection{send:make(chan []byte,256),ws: ws, auth: false}
	h.register <- c
	go c.writePump()
	c.readPump()
}



func (h *hub) run()  {
	for {
		select {
		case c := <- h.register:
			h.connections[c] = true
		case c := <- h.unregister :
			if _,ok := h.connections[c]; ok {
				delete(h.connections,c)
				close(c.send)
			}
		case m := <- h.broadcast :
			fmt.Println(m)
			for c := range h.connections {
				var send_flag = false
				var send_msg []byte
				if m.mtype == 1 {
					send_msg = []byte("system: "+string(m.content))
				}else if m.mtype == 2 {
					send_msg = []byte(string(m.fromuser)+" say: "+string(m.content))
				}else {
					send_msg = []byte(string(m.content))
				}

				if string(m.touser) != "all" {
					if string(c.username) == string(m.touser) || string(c.username) == string(m.fromuser) {
						send_flag = true
					}
					if send_flag {
						select {
						case c.send <- send_msg:
						default:
							close(c.send)
							delete(h.connections,c)
						}
					}
				}else {
					select {
					case c.send <- send_msg:
					default:
						close(c.send)
						delete(h.connections,c)
					}
				}
			}

		}

	}
}


var addr = flag.String("addr",":8080","http service address")
var homeTempl = template.Must(template.ParseFiles("home.html"))

func serveHome(w http.ResponseWriter,r *http.Request)  {
	fmt.Println("serverHome")
	if r.URL.Path != "/" {
		http.Error(w,"not found",404)
		return
	}
	if r.Method != "GET" {
		http.Error(w,"method not allowed",405)
	}
	w.Header().Set("Content-type","text/html; charset=utf-8")
	homeTempl.Execute(w,r.Host)
}

func main()  {

	flag.Parse()
	go h.run()
	http.HandleFunc("/",serveHome)
	http.HandleFunc("/ws",serveWs)
	err := http.ListenAndServe(*addr,nil)
	if err != nil {
		log.Fatal("ListenAndServe: ",err)
	}
}