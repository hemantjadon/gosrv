package proxy

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// ErrClosed is returned from Proxy.ListenAndTransfer when it shuts down
	// completely.
	ErrClosed proxyError = "proxy closed"

	// ErrRunning is returned from Proxy.ListenAndTransfer when it is already
	// running.
	ErrRunning proxyError = "proxy already running"

	// ErrNotRunning is returned from Proxy.Shutdown when it is not running.
	ErrNotRunning proxyError = "proxy not running"

	// ErrShuttingDown is returned from Proxy.Shutdown when it is already
	// shutting down.
	ErrShuttingDown proxyError = "proxy already shutting down"
)

// Proxy proxies the HTTP/HTTPS requests to an upstream server.
type Proxy struct {
	// Network on which proxy listens.
	Network string

	// Addr on which proxy listens.
	Addr string

	// Network of the upstream server to proxy towards.
	UpstreamNetwork string

	// Addr of the upstream server to proxy towards.
	UpstreamAddr string

	// Whether upstream server supports TLS or not.
	UpstreamTLS bool

	// Logger used to log the runtime error messages. If nil no logging is done.
	Logger logger

	mu          sync.Mutex
	listeners   map[*net.Listener]struct{}
	activeConns map[*net.Conn]struct{}

	inShutdown int32
	running    int32
}

// ListenAndTransfer listens for connections on the provided Proxy.Network and
// Proxy.Addr, and eventually proxies them to the connections dialed on
// Proxy.UpstreamNetwork and Proxy.UpstreamAddr.
//
// It is designed to primarily work with TCP connections with HTTP requests on
// the accepting connections, as it sniffs for the type of data flowing through
// the proxied connections.
//
// If upstream server does not support TLS then the downstream request is
// directly proxied to upstream.
//
// If upstream server supports TLS and if downstream request starts with HTTPS
// connect byte (22), it is directly proxied to the upstream.
//
// If upstream server supports TLS and if downstream request is a valid HTTP
// request, with Upgrade-Insecure-Requests header set with value 1 then, it
// responds with http.StatusTemporaryRedirect with an appropriate Location
// header pointing to secure HTTPS resource. Whereas if the downstream request
// does not have header Upgrade-Insecure-Requests with value 1, then the request
// is directly proxied to upstream.
//
// If the upstream server supports TLS, and the downstream request is not a
// valid HTTP request, it is still transparently proxied to the upstream.
//
// If upstreams server does not support TLS, then downstream request is
// directly proxied to the upstream.
//
// If unable to start the listener on the given Proxy.Network and Proxy.Addr
// then non-nil error is returned.
//
// When the proxy shut down completes successfully, ie. all the listeners and
// connections are closed then it returns ErrClosed.
func (p *Proxy) ListenAndTransfer() error {
	if ok := atomic.CompareAndSwapInt32(&p.running, 0, 1); !ok {
		return ErrRunning
	}
	defer atomic.StoreInt32(&p.running, 0)

	ln, err := net.Listen(p.Network, p.Addr)
	if err != nil {
		return fmt.Errorf("net listen: %w", err)
	}

	p.mu.Lock()
	if p.listeners == nil {
		p.listeners = make(map[*net.Listener]struct{})
	}
	p.listeners[&ln] = struct{}{}
	p.mu.Unlock()

	var wg sync.WaitGroup
	for {
		conn, err := ln.Accept()
		if err != nil {
			break
		}

		p.mu.Lock()
		if p.activeConns == nil {
			p.activeConns = make(map[*net.Conn]struct{})
		}
		p.activeConns[&conn] = struct{}{}
		p.mu.Unlock()

		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := p.Proxy(conn); err != nil {
				p.logErrf("proxy: %v", err)
			}

			p.mu.Lock()
			delete(p.activeConns, &conn)
			p.mu.Unlock()
		}()
	}

	p.mu.Lock()
	delete(p.listeners, &ln)
	p.mu.Unlock()

	wg.Wait()
	return ErrClosed
}

// Shutdown shuts down the proxy gracefully. It firsts closes the active
// listeners, then it sets read and write deadlines on all the active
// connections, this is the hard limit for all the communications to complete.
//
// If shutdown is successful then nil error is returned. Otherwise a non-nil
// error provides the reason.
//
// If proxy is not running then ErrNotRunning is returned immediately.
//
// If proxy is already in shutdown mode then ErrShuttingDown is returned
// immediately.
func (p *Proxy) Shutdown(ctx context.Context) error {
	if running := atomic.LoadInt32(&p.running); running == 0 {
		return ErrNotRunning
	}
	if !atomic.CompareAndSwapInt32(&p.inShutdown, 0, 1) {
		return ErrShuttingDown
	}
	defer atomic.StoreInt32(&p.inShutdown, 0)

	var err error
	p.mu.Lock()
	for ln := range p.listeners {
		if cerr := (*ln).Close(); cerr != nil && err == nil {
			err = cerr
		}
	}

	for conn := range p.activeConns {
		// Set the deadline for communication.
		if cerr := (*conn).SetDeadline(time.Now().Add(proxyShutdownIdleTimeout)); cerr != nil && err != nil {
			err = cerr
		}
	}
	p.mu.Unlock()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
			p.mu.Lock()
			if len(p.activeConns) == 0 {
				p.mu.Unlock()
				return err
			}
			p.mu.Unlock()
		}
	}
}

// Proxy proxies provided connection to upstream.
func (p *Proxy) Proxy(conn net.Conn) error {
	defer func() {
		if err := conn.Close(); err != nil {
			p.logErrf("proxy: downstream: conn close: %v", err)
		}
	}()

	if err := conn.SetReadDeadline(time.Now().Add(proxyReadTimeout)); err != nil {
		return fmt.Errorf("downstream: conn set read deadline: %v", err)
	}

	var buf bytes.Buffer
	_, err := io.CopyN(&buf, conn, 1)
	if err != nil && errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("downstream: conn read first byte: %v", err)
	}

	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return fmt.Errorf("downstream: conn unset read deadline: %v", err)
	}

	firstByte := buf.Bytes()[0]

	pc := customReaderConn{
		Conn: conn,
		rdr:  io.MultiReader(bytes.NewReader([]byte{firstByte}), conn),
	}
	if !p.UpstreamTLS {
		return p.connectAndTransfer(pc)
	}
	if p.UpstreamTLS && (firstByte == httpsFirstByte) {
		return p.connectAndTransfer(pc)
	}

	if err := pc.SetReadDeadline(time.Now().Add(proxyReadTimeout)); err != nil {
		return fmt.Errorf("downstream: conn set read deadline: %v", err)
	}

	buf = bytes.Buffer{}
	req, err := http.ReadRequest(bufio.NewReader(io.TeeReader(pc, &buf)))
	if err != nil {
		pc = customReaderConn{Conn: conn, rdr: &buf}
		return p.connectAndTransfer(pc)
	}

	if err := pc.SetReadDeadline(time.Time{}); err != nil {
		return fmt.Errorf("downstream: conn unset read deadline: %v", err)
	}

	pc = customReaderConn{Conn: conn, rdr: &buf}
	if req.Header.Get(headerUpgradeInsecureRequests) != strconv.Itoa(1) {
		return p.connectAndTransfer(pc)
	}

	redirectLoc := fmt.Sprintf("https://%s%s", req.Host, req.URL.Path)
	res := temporaryRedirect(req.Proto, redirectLoc)
	if _, err := conn.Write(res); err != nil {
		return fmt.Errorf("downstream: conn write redirect response: %v\n", err)
	}
	return nil
}

func (p *Proxy) connectAndTransfer(conn net.Conn) error {
	uconn, err := net.Dial(p.UpstreamNetwork, p.UpstreamAddr)
	if err != nil {
		return fmt.Errorf("upstream: net dial: %v", err)
	}
	defer func() {
		if err := uconn.Close(); err != nil {
			p.logErrf("proxy: upstream: conn close: %v", err)
		}
	}()

	go p.copy(uconn, conn)
	p.copy(conn, uconn)

	return nil
}

func (p *Proxy) copy(dst, src net.Conn) {
	if _, err := io.Copy(dst, src); err != nil {
		p.logErrf("proxy: connections: io copy (src %s -> dst %s): %v", src.RemoteAddr(), dst.RemoteAddr(), err)
	}
}

func (p *Proxy) logErr(args ...interface{}) {
	if p.Logger == nil {
		return
	}
	p.Logger.Println(append([]interface{}{"ERROR"}, args...)...)
}

func (p *Proxy) logErrf(format string, args ...interface{}) {
	p.logErr(fmt.Sprintf(format, args...))
}

func temporaryRedirect(proto, location string) []byte {
	statusCode := http.StatusTemporaryRedirect
	statusText := http.StatusText(statusCode)
	status := fmt.Sprintf("%d %s", statusCode, statusText)

	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("%s %s\r\n", proto, status))
	sb.WriteString(fmt.Sprintf("%s: %s\r\n", "Content-Length", "0"))
	sb.WriteString(fmt.Sprintf("%s: %s\r\n", "Location", location))
	sb.WriteString(fmt.Sprintf("%s: %s\r\n", "Vary", headerUpgradeInsecureRequests))
	sb.WriteString("\r\n")

	return []byte(sb.String())
}

type logger interface {
	Println(args ...interface{})
}

type customReaderConn struct {
	net.Conn
	rdr io.Reader
}

func (c customReaderConn) Read(p []byte) (int, error) {
	return c.rdr.Read(p)
}

type proxyError string

func (e proxyError) Error() string {
	return string(e)
}

const (
	proxyReadTimeout         = 1 * time.Second
	proxyShutdownIdleTimeout = 3 * time.Second

	httpsFirstByte byte = 22

	headerUpgradeInsecureRequests = "Upgrade-Insecure-Requests"
)
