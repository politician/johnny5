package main

import (
	"log"
	"net"
	"net/http"
	"sync"
)

type HttpServer struct {
	http.Server

	wl    WaitListener
	state HttpServerState
}

type HttpServerState int

const (
	NotReady HttpServerState = iota
	Listening
	Stopping
	Stopped
)

type httpServerState int

func NewHttpServer(addr string, handler http.Handler) *HttpServer {
	if addr == "" {
		addr = ":http"
	}

	srv := &HttpServer{
		Server: http.Server{
			Addr:    addr,
			Handler: handler,
		},
		state: NotReady,
	}

	return srv
}

// Stop the server by closing the underlying listener.  Block until the service
// is really stopped.
func (srv *HttpServer) Stop() (err error) {
	defer func() { srv.state = Stopped }()

	if srv.state == NotReady {
		return
	}

	srv.state = Stopping

	err = srv.wl.Close()
	if err != nil {
		return
	}

	log.Println("refusing connections to " + srv.Addr)
	log.Println("waiting for existing requests to drain...")

	srv.wl.Wait()

	log.Printf("drained")

	return
}

func (srv *HttpServer) IsStopping() bool {
	return srv.state == Stopping
}

func (srv *HttpServer) Start() (err error) {
	wl, err := listenTCP(srv.Addr)
	if err != nil {
		return
	}

	srv.wl = wl
	srv.state = 1

	go func() {
		// This function will not return until the listener is closed.
		err := srv.Server.Serve(srv.wl)
		if err != nil && !srv.IsStopping() {
			log.Fatalf("Serve: %v", err)
		}
	}()

	return
}

func listenTCP(laddr string) (wl WaitListener, err error) {
	l, err := net.Listen("tcp", laddr)
	if err != nil {
		return
	}

	wl = &waitListener{
		Listener: l,
		wg:       &sync.WaitGroup{},
	}
	return
}

type WaitListener interface {
	net.Listener
	Wait()
}

type waitListener struct {
	net.Listener
	wg *sync.WaitGroup
}

type waitConn struct {
	net.Conn
	wg *sync.WaitGroup
}

func (l *waitListener) Wait() {
	l.wg.Wait()
}

// Add to the wait group on success and returns a waitConn.
func (l *waitListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return c, err
	}

	log.Printf("connection accepted: %s", c.RemoteAddr().String())

	l.wg.Add(1)
	return &waitConn{c, l.wg}, nil
}

// Calls Done() on the wait group.
func (c *waitConn) Close() error {
	err := c.Conn.Close()
	c.wg.Done()
	log.Printf("connection closed: %s", c.RemoteAddr().String())
	return err
}
