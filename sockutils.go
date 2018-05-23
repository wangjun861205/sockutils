package sockutils

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

type _reader struct {
	conn      *net.TCPConn
	readChan  chan []byte
	delimiter string
	done      chan struct{}
	timeout   time.Duration
}

func newReader(conn *net.TCPConn, delimiter string, timeout time.Duration) *_reader {
	reader := &_reader{
		conn:      conn,
		readChan:  make(chan []byte),
		delimiter: delimiter,
		done:      make(chan struct{}),
		timeout:   timeout,
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
		if err := r.conn.SetReadDeadline(time.Now().Add(r.timeout)); err != nil {
			continue
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

func NewConnector(conn *net.TCPConn, delimiter string, timeout time.Duration) (*Connector, error) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.SetNoDelay(true); err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.SetKeepAlive(true); err != nil {
		conn.Close()
		return nil, err
	}
	connector := &Connector{newReader(conn, delimiter, timeout), newWriter(conn, delimiter), make(chan struct{}), sync.Map{}}
	go connector.run()
	return connector, nil
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

type TCPListener struct {
	addr      string
	delimiter string
	timeout   time.Duration
	connChan  chan *Connector
	listener  *net.TCPListener
	close     chan struct{}
	done      chan struct{}
}

func NewTCPListener(addr, delimiter string, timeout time.Duration) (*TCPListener, error) {
	netListener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	netTCPListener, ok := netListener.(*net.TCPListener)
	if !ok {
		return nil, errors.New("listener type assertion error")
	}
	l := &TCPListener{
		addr:      addr,
		delimiter: delimiter,
		timeout:   timeout,
		connChan:  make(chan *Connector),
		listener:  netTCPListener,
		close:     make(chan struct{}),
		done:      make(chan struct{}),
	}
	go l.run()
	return l, nil
}

func (l *TCPListener) run() {
	for {
		select {
		case <-l.close:
			close(l.connChan)
			close(l.done)
			fmt.Println("TCPListener has closed")
			return
		default:
			conn, err := l.listener.AcceptTCP()
			if err != nil {
				fmt.Println("TCPListener: " + err.Error())
				continue
			}
			connector, err := NewConnector(conn, l.delimiter, l.timeout)
			if err != nil {
				continue
			}
			go func() {
				defer func() {
					err := recover()
					if err != nil {
						fmt.Println("TCPListener: " + err.(error).Error())
					}
				}()
				l.connChan <- connector
			}()
		}
	}
}

func (l *TCPListener) Close() {
	close(l.close)
	l.listener.Close()
}

func (l *TCPListener) ConnChan() chan *Connector {
	return l.connChan
}

func (l *TCPListener) Done() chan struct{} {
	return l.done
}

func Dial(addr, delimiter string, timeout time.Duration) (*Connector, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return nil, errors.New("sockutils.Dial(): tcp connect type assert error")
	}
	connector, err := NewConnector(tcpConn, delimiter, timeout)
	if err != nil {
		return nil, err
	}
	return connector, nil
}
