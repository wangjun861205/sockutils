package sockutils

import (
	"fmt"
	"log"
	"net"
	"sync"
	"testing"
	"time"
)

func TestConnector(t *testing.T) {
	connChan := make(chan *Connector)
	l, err := net.Listen("tcp", "127.0.0.1:10000")
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				fmt.Println(err)
				continue
			}
			tcpConn := conn.(*net.TCPConn)
			tcpConn.SetNoDelay(true)
			connector, err := NewConnector(tcpConn, "\r\n\r\n", 10*time.Second)
			if err != nil {
				fmt.Println(err)
				continue
			}
			go func() {
				connChan <- connector
			}()
		}
	}()
	go func() {
		for {
			conn := <-connChan
			go func() {
				for {
					select {
					case <-conn.Done():
						return
					case b, ok := <-conn.ReadChan():
						if !ok {
							return
						}
						fmt.Println(string(b))
						conn.Write([]byte("receive success"))
					}
				}
			}()
		}
	}()
	conn, err := net.Dial("tcp", "127.0.0.1:10000")
	if err != nil {
		log.Fatal(err)
	}
	tcpConn := conn.(*net.TCPConn)
	tcpConn.SetNoDelay(true)
	connector, err := NewConnector(tcpConn, "\r\n\r\n", 10*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 100000; i++ {
		wg.Add(1)
		connector.Write([]byte("hello world"))
	}
	go func() {
		for {
			b := <-connector.ReadChan()
			fmt.Println(string(b))
			wg.Done()
		}
	}()
	wg.Wait()
}
