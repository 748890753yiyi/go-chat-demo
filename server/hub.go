package server

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
