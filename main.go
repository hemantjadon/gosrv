// gosrv is a simple http file server with CLI and https support.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/hemantjadon/gosrv/internal/pkg/cli"
	"github.com/hemantjadon/gosrv/internal/pkg/proxy"
)

const AppName = "gosrv"

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt)
	defer func() {
		signal.Stop(sigChan)
		cancel()
	}()
	go func() {
		select {
		case <-sigChan:
			cancel()
			_, _ = fmt.Fprintf(os.Stdout, "%s: closing down, press ctrl+c again to force quit\n", AppName)
		case <-ctx.Done():
		}
		<-sigChan
		os.Exit(exitCodeOnInterrupt)
	}()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if len(err.Error()) != 0 {
			_, _ = fmt.Fprintf(os.Stderr, "%s: %s\n", AppName, err.Error())
		}
		os.Exit(exitCodeOnErr)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	inputs, err := cli.Parse(AppName, args)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: cli: %s\n", AppName, err.Error())
		cli.WriteHelp(stderr, AppName)
		return fmt.Errorf("")
	}

	if inputs.Help {
		cli.WriteHelp(stderr, AppName)
		return nil
	}

	dirAbsPath, err := filepath.Abs(inputs.Dir)
	if err != nil {
		return fmt.Errorf("unable to determine dir absolute path: %w", err)
	}

	cleartext := (len(inputs.KeyFile) == 0) && (len(inputs.CertFile) == 0)

	httpLis, err := setupHTTPListener(inputs.Host)
	if err != nil {
		return fmt.Errorf("setup http tcp listener: %w", err)
	}
	defer httpLis.Close()

	httpHost, httpPort, err := net.SplitHostPort(httpLis.Addr().String())
	if err != nil {
		return fmt.Errorf("http tcp listener addr split host port: %w", err)
	}
	httpAddr := net.JoinHostPort(httpHost, httpPort)

	var logger *log.Logger
	if inputs.Quiet {
		logger = setupLogger(ioutil.Discard)
	} else {
		logger = setupLogger(stderr)
	}

	proxyAddr := net.JoinHostPort(inputs.Host, inputs.Port)
	prx := setupProxy(proxyAddr, httpAddr, !cleartext, logger)

	srv := setupHTTPServer(dirAbsPath)

	srvClosedCh := make(chan struct{})
	go func() {
		defer close(srvClosedCh)
		handle := func(err error) {
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "%s: error: start server: %s\n", AppName, err.Error())
				return
			}
			_, _ = fmt.Fprintf(stdout, "%s: closed server\n", AppName)
		}

		if cleartext {
			_, _ = fmt.Fprintf(stdout, "%s: starting http server on %s\n", AppName, proxyAddr)
			handle(runHTTP(httpLis, srv))
		} else {
			_, _ = fmt.Fprintf(stdout, "%s: starting https server on %s\n", AppName, proxyAddr)
			handle(runHTTPTLS(httpLis, srv, inputs.CertFile, inputs.KeyFile))
		}
	}()

	prxClosedCh := make(chan struct{})
	go func() {
		defer close(prxClosedCh)

		if err := prx.ListenAndTransfer(); err != nil && !errors.Is(err, proxy.ErrClosed) {
			_, _ = fmt.Fprintf(stderr, "%s: error: running proxy: %s\n", AppName, err.Error())
		}
	}()

	closedCh := make(chan struct{})
	closingCh := make(chan struct{})
	go func() {
		defer close(closedCh)
		select {
		case <-prxClosedCh:
			if err := srv.Shutdown(context.Background()); err != nil {
				_, _ = fmt.Fprintf(stderr, "%s: error: stop server: %s\n", AppName, err.Error())
			}
		case <-srvClosedCh:
			if err := prx.Shutdown(context.Background()); err != nil {
				_, _ = fmt.Fprintf(stderr, "%s: error: closing proxy: %s\n", AppName, err.Error())
			}
		case <-closingCh:
		}
	}()

	select {
	case <-closedCh:
		return nil
	case <-ctx.Done():
	}

	close(closingCh)

	if err := prx.Shutdown(context.Background()); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: error: closing proxy: %s\n", AppName, err.Error())
	}

	if err := srv.Shutdown(context.Background()); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: error: stop server: %s\n", AppName, err.Error())
	}

	return nil
}

func setupLogger(w io.Writer) *log.Logger {
	return log.New(w, "", log.Ldate|log.Ltime|log.Lmicroseconds)
}

func setupProxy(proxyAddr, upstreamAddr string, upstreamTLS bool, logger *log.Logger) *proxy.Proxy {
	prx := proxy.Proxy{
		Network:         "tcp",
		Addr:            proxyAddr,
		UpstreamNetwork: "tcp",
		UpstreamAddr:    upstreamAddr,
		UpstreamTLS:     upstreamTLS,
		Logger:          logger,
	}
	return &prx
}

func setupHTTPListener(host string) (net.Listener, error) {
	l, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return nil, fmt.Errorf("net listen tcp: %w", err)
	}
	return l, nil
}

func setupHTTPServer(dir string) *http.Server {
	srv := http.Server{
		Handler:      http.FileServer(http.Dir(dir)),
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}
	return &srv
}

func runHTTP(l net.Listener, srv *http.Server) error {
	if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

func runHTTPTLS(l net.Listener, srv *http.Server, certFile, keyFile string) error {
	if err := srv.ServeTLS(l, certFile, keyFile); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen and serve tls: %w", err)
	}
	return nil
}

const (
	exitCodeOnErr       = 1
	exitCodeOnInterrupt = 2
)

const (
	readTimeout  = 5 * time.Second
	idleTimeout  = 5 * time.Second
	writeTimeout = 60 * time.Second
)
