package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
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
	network     string
	addr        string
	conn        net.Conn
	state       state
	txBufr      chan Netdata
	txDone      chan bool
	txThreshold float32 // channel threshold in %
	backup      *bufio.Writer
}

type server struct {
	network string
	addr    string
}

func NewClient(ip string, port string, cap int, threshold int, backupWriter *bufio.Writer) *client {
	return &client{
		network:     "tcp",
		addr:        fmt.Sprintf("%s:%s", ip, port),
		state:       OFFLINE,
		txBufr:      make(chan Netdata, cap), // between writer and transmitter
		txDone:      make(chan bool),         // all data in channel is transmitted
		backup:      backupWriter,
		txThreshold: float32(threshold) / float32(cap) * 100,
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

	f, _ := os.OpenFile("backup.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	defer f.Close()
	cli := NewClient("127.0.0.1", "31500", 100, 60, bufio.NewWriter(f))
	cli.run()
	time.Sleep(2 * time.Second) // wait for client

	// to transmit
	tData := make([]byte, 6)
	for i := range [10]int32{} {
		binary.LittleEndian.PutUint16(tData[:2], uint16(i))
		copy(tData[2:], "TEST")
		cli.Writer([]byte(tData))
	}
	cli.close()

	time.Sleep(2 * time.Second) // wait for server to print
}

func (c *client) close() {
	fmt.Printf("[CLIENT]  disconnecting (remote: %s)\n", c.conn.RemoteAddr())
	c.state = OFFLINE // no more writes
	close(c.txBufr)
	<-c.txDone // wait for channel to empty
	c.conn.Close()
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
	var bw *bufio.Writer
	sbw := bufio.NewWriter(c.conn)
	for {
		fmt.Printf("[CLIENT]  channel (occupied: %d, capacity: %d)\n", len(c.txBufr), cap(c.txBufr))
		bw = sbw
		if float32(len(c.txBufr))/float32(cap(c.txBufr))*100 >= c.txThreshold && c.backup != nil {
			bw = c.backup // threshold reached transmit to backup
		}
		if tx, open := <-c.txBufr; open {
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
				fmt.Printf("[SERVER]  client closed connection (addr: %s)\n", conn.RemoteAddr())
			} else {
				fmt.Printf("[SERVER]  read failed %v", err)
			}
			conn.Close()
			return
		}
		fmt.Printf("[SERVER]  received (size: %d): %v\n", len, buf[0:len])
	}
}
