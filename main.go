package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"
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

type socketWriter struct {
	network string
	addr    string
}

type socketReader struct {
	network string
	addr    string
}

func NewSocketWriter(ip string, port string) *socketWriter {
	return &socketWriter{
		network: "tcp",
		addr:    fmt.Sprintf("%s:%s", ip, port),
	}
}

func NewSocketReader(ip string, port string) *socketReader {
	return &socketReader{
		network: "tcp",
		addr:    fmt.Sprintf("%s:%s", ip, port),
	}
}

func main() {
	fmt.Printf("testing socket\n")
	go listen(NewSocketReader("127.0.0.1", "31500"))

	time.Sleep(2 * time.Second) // wait for server to start
	sw := NewSocketWriter("127.0.0.1", "31500")
	conn, err := net.Dial(sw.network, sw.addr)
	if err != nil {
		log.Fatalf("connection failed (remote: %s): %s\n", sw.addr, err)
	}
	// bw := bufio.NewWriter(conn)
	// bw.Write([]byte("sending to server"))
	// bw.Flush()

	// to transmit
	tData := "AAAAAAAAAAAAAAAAAAAA"
	data := &Netdata{
		Len:  uint32(len(tData) + 12),
		Cmd:  byte(1),
		Arg:  [10]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		Act:  byte(1),
		Data: []byte(tData),
	}
	fmt.Printf("transmitting (size: %d): [ %s ]\n", len(data.encode()), data.encode())
	bw := bufio.NewWriter(conn)
	bw.Write(data.encode())
	bw.Flush()
	time.Sleep(2 * time.Second) // wait for server to print
	conn.Close()
}

func listen(sr *socketReader) {
	listner, err := net.Listen(sr.network, sr.addr)
	if err == nil {
		fmt.Printf("server started (listning at: %s)\n", sr.addr)
		for {
			go handleConnection(listner.Accept())
		}
	}
	defer listner.Close()
	fmt.Printf("server failed (addr: %s): %v\n", sr.addr, err)
}

func handleConnection(conn net.Conn, err error) {
	if err != nil {
		fmt.Printf("incomming client conection failed %s\n", err)
		return
	}
	defer conn.Close()
	fmt.Printf("incomming conection: %s\n", conn.RemoteAddr())

	buf := make([]byte, 20)
	len, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("read failed %v", err)
	}

	// fmt.Printf("received (size: %d): %v\n", len(data), data)
	fmt.Printf("received (size: %d): %v\n", len, buf[0:len])
}
