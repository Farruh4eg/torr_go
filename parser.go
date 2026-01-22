package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Node struct {
	// no std::variant from cpp :(
	Value any
}

func (n Node) asDict() map[string]Node {
	d, _ := n.Value.(map[string]Node)
	return d
}

func (n Node) asList() []Node {
	d, _ := n.Value.([]Node)
	return d
}

func (n Node) asString() string {
	d, _ := n.Value.(string)
	return d
}

func (n Node) asInt() int {
	d, _ := n.Value.(int)
	return d
}

func (n Node) asInt64() int64 {
	d, _ := n.Value.(int)
	return int64(d)
}

type Parser struct {
	buffer  []byte
	infoRaw string
	pos     int
}

func NewParserFromFile(filePath string) (*Parser, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}

	p := &Parser{buffer: make([]byte, fi.Size())}
	_, err = file.Read(p.buffer)

	return p, nil
}

func NewParserFromData(data []byte) (*Parser, error) {
	buf := make([]byte, len(data))
	copy(buf, data)
	return &Parser{buffer: buf}, nil
}

func (p *Parser) parse() (Node, error) {
	if len(p.buffer) == 0 {
		return Node{}, errors.New("file is empty")
	}

	root, err := p.parseElement()
	if err != nil {
		return Node{}, err
	}

	if p.pos != len(p.buffer) {
		return Node{}, errors.New("parsing finished, but additional data was found at the end of file")
	}

	return root, nil
}

func (p *Parser) parseElement() (Node, error) {
	current := p.buffer[p.pos]

	switch {
	case current == 'i':
		return p.parseInt()
	case current == 'l':
		return p.parseList()
	case current == 'd':
		return p.parseDict()
	case current >= '0' && current <= '9':
		return p.parseString()
	default:
		return Node{}, fmt.Errorf("invalid character: %c", current)
	}
}

func (p *Parser) parseInt() (Node, error) {
	// skip 'i'
	p.pos++
	start := p.pos

	for p.pos < len(p.buffer) && p.buffer[p.pos] != 'e' {
		p.pos++
	}

	if p.pos >= len(p.buffer) {
		return Node{}, errors.New("unexpected end of file integer")
	}

	numStr := string(p.buffer[start:p.pos])

	// skip 'e'
	p.pos++

	number, err := strconv.Atoi(numStr)
	if err != nil {
		return Node{}, fmt.Errorf("invalid integer: %w", err)
	}

	return Node{Value: number}, nil
}

func (p *Parser) parseString() (Node, error) {
	seqStart := p.pos

	for p.pos < len(p.buffer) && p.buffer[p.pos] != ':' {
		p.pos++
	}

	if p.pos >= len(p.buffer) {
		return Node{}, errors.New("unexpected end of file string")
	}

	lenStr := string(p.buffer[seqStart:p.pos])
	length, err := strconv.Atoi(lenStr)
	if err != nil {
		return Node{}, fmt.Errorf("invalid length: %w", err)
	}

	// jump over the ':'
	p.pos++

	if p.pos+length > len(p.buffer) {
		return Node{}, errors.New("string length out of bounds")
	}

	content := string(p.buffer[p.pos : p.pos+length])
	p.pos += length

	return Node{Value: content}, nil
}

func (p *Parser) parseList() (Node, error) {
	p.pos++

	var list []Node
	for p.pos < len(p.buffer) && p.buffer[p.pos] != 'e' {
		elem, err := p.parseElement()
		if err != nil {
			return Node{}, err
		}

		list = append(list, elem)
	}

	if p.pos >= len(p.buffer) {
		return Node{}, errors.New("unexpected end of file list")
	}

	p.pos++

	return Node{Value: list}, nil
}

func (p *Parser) parseDict() (Node, error) {
	// skip 'd'
	p.pos++
	dict := make(map[string]Node)

	for p.pos < len(p.buffer) && p.buffer[p.pos] != 'e' {
		keyNode, err := p.parseString()
		if err != nil {
			return Node{}, err
		}
		key := keyNode.Value.(string)

		isInfoKey := key == "info"
		startPos := p.pos

		valueNode, err := p.parseElement()
		dict[key] = valueNode

		if isInfoKey {
			p.infoRaw = string(p.buffer[startPos:p.pos])
		}
	}
	// skip 'e'
	p.pos++

	return Node{Value: dict}, nil
}

func (p *Parser) printNode(node Node, indent int) {
	spaces := strings.Repeat(" ", indent)

	switch v := node.Value.(type) {
	case int, int64:
		fmt.Printf("%v\n", v)
	case string:
		if len(v) > 50 {
			fmt.Printf("[Binary blob %d bytes]\n", len(v))
		} else {
			fmt.Printf("\"%v\"\n", v)
		}
	case []Node:
		fmt.Printf("List [\n")
		for _, item := range v {
			fmt.Printf("%s ", spaces)
			p.printNode(item, indent+2)
		}
		fmt.Printf("%s]\n", spaces)
	case map[string]Node:
		fmt.Printf("Dict {\n")
		for key, value := range v {
			fmt.Printf("%s %s: ", spaces, key)
			p.printNode(value, indent+2)
		}
		fmt.Printf("%s}\n", spaces)
	default:
		fmt.Printf("Unknown type: %T\n", v)
	}
}
