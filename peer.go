package main

import "fmt"

type Peer struct {
	ip   string
	port uint16
}

func (p *Peer) String() string {
	return fmt.Sprintf("%s:%d", p.ip, p.port)
}
