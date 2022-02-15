package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

type state byte

const (
	OFFLINE state = iota + 1
	CONNECTED
)

type Netdata struct {
	Len  uint32
	Cmd  uint8
	Arg  [10]byte
	Act  uint8
	Data []byte
}

func (n *Netdata) encode() []byte {
	buf := make([]byte, n.Len+4)
	binary.LittleEndian.PutUint32(buf[:4], n.Len)
	buf[4] = n.Cmd
	buf[15] = n.Act
	copy(buf[16:], n.Data)
	return buf
}

type client struct {
	network string
	addr    string
	conn    net.Conn
	state   state
	txBufr  chan Netdata
	txDone  chan bool
}

type server struct {
	network string
	addr    string
}

func NewClient(ip string, port string, cap int) *client {
	return &client{
		network: "tcp",
		addr:    fmt.Sprintf("%s:%s", ip, port),
		state:   OFFLINE,
		txBufr:  make(chan Netdata, cap), // between writer and transmitter
		txDone:  make(chan bool),         // all data in channel is transmitted
	}
}

func NewServer(ip string, port string) *server {
	return &server{
		network: "tcp",
		addr:    fmt.Sprintf("%s:%s", ip, port),
	}
}

func (c *client) Writer(p []byte) (l int, err error) {
	tx := Netdata{
		Len:  uint32(len(p) + 12),
		Cmd:  byte(1),
		Arg:  [10]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		Act:  byte(1),
		Data: make([]byte, len(p)), // allocate or else new one will override data in channel
	}
	copy(tx.Data, p)
	c.txBufr <- tx
	return
}

func main() {
	fmt.Printf("testing socket\n")
	go listen(NewServer("127.0.0.1", "31500"))
	time.Sleep(2 * time.Second) // wait for server to start

	// client
	cli := NewClient("127.0.0.1", "31500", 10)
	cli.run()
	time.Sleep(2 * time.Second) // wait for client

	// to transmit
	tData := make([]byte, 6)
	for i := range [200]int32{} {
		binary.LittleEndian.PutUint16(tData[:2], uint16(i))
		copy(tData[2:], "TEST")
		cli.Writer([]byte(tData))
	}

	cli.close()
	time.Sleep(2 * time.Second) // wait for server to print
}

func (c *client) close() {
	fmt.Printf("[CLIENT]  disconnecting (remote: %s)\n", c.conn.RemoteAddr())
	defer c.conn.Close()
	c.state = OFFLINE // no more writes
	close(c.txBufr)
	<-c.txDone // wait for channel to empty
	fmt.Printf("[CLIENT]  disconnected (remote: %s)\n", c.conn.RemoteAddr())
}

func (c *client) run() error {
	var err error
	c.conn, err = net.Dial(c.network, c.addr)
	if err != nil {
		log.Fatalf("[CLIENT]  connection failed (remote: %s): %s\n", c.addr, err)
		return err
	}
	c.state = CONNECTED
	fmt.Printf("[CLIENT]  connected (remote: %s)\n", c.addr)
	go c.handle()
	return nil
}

func (c *client) handle() {
	for {
		if tx, open := <-c.txBufr; open {
			bw := bufio.NewWriter(c.conn)
			l, _ := bw.Write(tx.encode())
			fmt.Printf("[CLIENT]  transmitted (size: %d): %v%s\n", l, tx.Data[:2], string(tx.Data[2:]))
			bw.Flush()
		} else {
			fmt.Printf("[CLIENT]  closing handler\n")
			c.txDone <- true
			return
		}
	}
}

func listen(sr *server) {
	listner, err := net.Listen(sr.network, sr.addr)
	if err == nil {
		fmt.Printf("[SERVER]  started (listning at: %s)\n", sr.addr)
		for {
			go handleConnection(listner.Accept())
		}
	}
	defer listner.Close()
	fmt.Printf("[SERVER]  failed (addr: %s): %v\n", sr.addr, err)
}

func handleConnection(conn net.Conn, err error) {
	if err != nil {
		fmt.Printf("[SERVER] connection failed %s\n", err)
		return
	}
	defer conn.Close()
	fmt.Printf("[SERVER]  incomming conection: %s\n", conn.RemoteAddr())

	buf := make([]byte, 22)
	for {
		len, err := conn.Read(buf)
		if err != nil {
			if err == io.EOF {
				fmt.Printf("[SERVER]  closing client connection (addr: %s)\n", conn.RemoteAddr())
			} else {
				fmt.Printf("[SERVER]  read failed %v", err)
			}
			conn.Close()
			return
		}
		fmt.Printf("[SERVER]  received (size: %d): %v\n", len, buf[0:len])
		time.Sleep(100 * time.Microsecond)
	}
}
