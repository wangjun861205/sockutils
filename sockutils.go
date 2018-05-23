package sockutils

import (
	"bytes"
	"fmt"
	"net"
	"sync"
)

type _reader struct {
	conn      *net.TCPConn
	readChan  chan []byte
	delimiter string
	done      chan struct{}
}

func newReader(conn *net.TCPConn, delimiter string) *_reader {
	reader := &_reader{
		conn:      conn,
		readChan:  make(chan []byte),
		delimiter: delimiter,
		done:      make(chan struct{}),
	}
	go reader.run()
	return reader
}

func (r *_reader) run() {
	buf := make([]byte, 10e3)
	content := make([]byte, 0, 10e4)
	for {
		n, err := r.conn.Read(buf)
		if err != nil {
			close(r.readChan)
			close(r.done)
			return
		}
		content = append(content, buf[:n]...)
		if dataList := bytes.Split(content, []byte(r.delimiter)); len(dataList) > 1 {
			content = dataList[len(dataList)-1]
			go func() {
				defer func() {
					err := recover()
					if err != nil {
						fmt.Println("Connector reader error: " + err.(error).Error())
					}
				}()
				for i := 0; i < len(dataList)-1; i++ {
					r.readChan <- dataList[i]
				}
			}()
		}
	}
}

type _writer struct {
	conn      *net.TCPConn
	writeChan chan []byte
	delimiter string
	done      chan struct{}
}

func newWriter(conn *net.TCPConn, delimiter string) *_writer {
	writer := &_writer{
		conn,
		make(chan []byte),
		delimiter,
		make(chan struct{})}
	go writer.run()
	return writer
}

func (w *_writer) run() {
	for {
		b, ok := <-w.writeChan
		if !ok {
			close(w.done)
			return
		}
		_, err := w.conn.Write(append(b, []byte(w.delimiter)...))
		if err != nil {
			return
		}
	}
}

type Connector struct {
	reader *_reader
	writer *_writer
	done   chan struct{}
	argMap sync.Map
}

func NewConnector(conn *net.TCPConn, delimiter string) *Connector {
	connector := &Connector{newReader(conn, delimiter), newWriter(conn, delimiter), make(chan struct{}), sync.Map{}}
	go connector.run()
	return connector
}

func (c *Connector) run() {
	<-c.reader.done
	close(c.writer.writeChan)
	<-c.writer.done
	close(c.done)
	c.reader.conn.Close()
	fmt.Printf("%s conn has closed\n", c.reader.conn.RemoteAddr().String())
}

func (c *Connector) ReadChan() chan []byte {
	return c.reader.readChan
}

func (c *Connector) Write(b []byte) {
	go func() {
		defer func() {
			err := recover()
			if err != nil {
				fmt.Println("Writer write error: " + err.(error).Error())
			}
		}()
		c.writer.writeChan <- b
	}()
}

func (c *Connector) Close() {
	c.reader.conn.Close()
	<-c.done
}

func (c *Connector) Done() chan struct{} {
	return c.done
}

func (c *Connector) GetArg(key interface{}) (interface{}, bool) {
	return c.argMap.Load(key)
}

func (c *Connector) SetArg(key interface{}, value interface{}) {
	c.argMap.Store(key, value)
}
