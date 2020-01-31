package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	logging "github.com/ipfs/go-log"

	"golang.org/x/crypto/acme/autocert"
)

/*
	Proxy is nearly impossible to test locally, so there are no tests here,
	but the basic idea is to listen on 443/80 given a domain name and terminate TLS at this
	proxy, forwarding the TCP traffic to the Backend.

	Tupelo uses this in order to offer wss backends and not just ws backends so that the browsers can connect
	to Tupelo p2p nodes.
*/

var logger = logging.Logger("tlsproxy")

// Backend describes the TCP connection the server should talk to (unencrypted)
type Backend struct {
	Addr           string `yaml:"addr"`
	ConnectTimeout int    `yaml:"connect_timeout"`
}

// Server is a TLS termination proxy that will take incoming TLS connections from
// a domain and proxy them to an insecure TCP backend. It uses autocert
// to get a LetsEncrypt certificate on demand for serving the TLS.
type Server struct {
	BindDomain    string
	Backend       Backend
	CertDirectory string
	Manager       *autocert.Manager
	listener      net.Listener
	didSetup      bool
}

func (s *Server) setup() {
	var cache autocert.Cache
	if s.CertDirectory != "" {
		cache = autocert.DirCache(s.CertDirectory)
	}
	m := &autocert.Manager{
		Cache:      cache,
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(s.BindDomain),
	}
	s.Manager = m
	l := m.Listener() // port 443 is the only thing allowed here
	s.listener = l
	s.didSetup = true
}

func (s *Server) Run() error {
	if !s.didSetup {
		s.setup()
	}

	logger.Debugf("starting http handler")
	go func() {
		err := http.ListenAndServe(":80", s.Manager.HTTPHandler(nil))
		if err != nil {
			panic(fmt.Errorf("error running http handler: %w", err))
		}
	}()

	logger.Infof("Serving connections on %v", s.listener.Addr())
	return s.acceptLoop()
}

func (s *Server) acceptLoop() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return fmt.Errorf("error accepting: %w", err)
		}
		s.handleConn(conn)
	}
}

func (s *Server) handleConn(c net.Conn) {
	backend := s.Backend
	// dial the backend
	backendConn, err := net.DialTimeout("tcp", backend.Addr, time.Duration(backend.ConnectTimeout)*time.Millisecond)
	if err != nil {
		logger.Errorf("Failed to dial backend connection %v: %v", backend.Addr, err)
		c.Close()
		return
	}
	logger.Debugf("Initiated new connection to backend: %v %v", backendConn.LocalAddr(), backendConn.RemoteAddr())
	go proxy(backendConn.(*net.TCPConn), c)
}

func proxy(srvConn *net.TCPConn, cliConn net.Conn) {
	// channels to wait on the close event for each connection
	serverClosed := make(chan struct{}, 1)
	clientClosed := make(chan struct{}, 1)

	go broker(srvConn, cliConn, clientClosed)
	go broker(cliConn, srvConn, serverClosed)

	// wait for one half of the proxy to exit, then trigger a shutdown of the
	// other half by calling CloseRead(). This will break the read loop in the
	// broker and allow us to fully close the connection cleanly without a
	// "use of closed network connection" error.
	var waitFor chan struct{}
	select {
	case <-clientClosed:
		// the client closed first and any more packets from the server aren't
		// useful, so we can optionally SetLinger(0) here to recycle the port
		// faster.
		err := srvConn.SetLinger(0)
		if err != nil {
			logger.Errorf("error setting server linger after client closed: %v", err)
		}

		err = srvConn.CloseRead()
		if err != nil {
			logger.Errorf("error closing server: %v", err)
		}

		waitFor = serverClosed
	case <-serverClosed:
		cliConn.Close()
		waitFor = clientClosed
	}

	// Wait for the other connection to close.
	// This "waitFor" pattern isn't required, but gives us a way to track the
	// connection and ensure all copies terminate correctly; we can trigger
	// stats on entry and deferred exit of this function.
	<-waitFor
}

// This does the actual data transfer.
// The broker only closes the Read side.
func broker(dst, src net.Conn, srcClosed chan struct{}) {
	// We can handle errors in a finer-grained manner by inlining io.Copy (it's
	// simple, and we drop the ReaderFrom or WriterTo checks for
	// net.Conn->net.Conn transfers, which aren't needed). This would also let
	// us adjust buffersize.
	_, err := io.Copy(dst, src)

	if err != nil {
		logger.Errorf("Copy error: %s", err)
	}
	if err := src.Close(); err != nil {
		logger.Errorf("Close error: %s", err)
	}
	srcClosed <- struct{}{}
}
