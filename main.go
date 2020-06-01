// gosrv is a simple http file server with CLI and https support.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

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
			_, _ = fmt.Fprintf(os.Stdout, "closing down, press ctrl+c again to force quit\n")
		case <-ctx.Done():
		}
		<-sigChan
		os.Exit(exitCodeOnInterrupt)
	}()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s\n", err.Error())
		os.Exit(exitCodeOnErr)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gosrv", flag.ContinueOnError)
	help := fs.Bool("help", false, "show help")
	host := fs.String("host", "localhost", "host name")
	port := fs.String("port", "8080", "port number")
	cert := fs.String("cert", "", "SSL cert")
	key := fs.String("key", "", "SSL cert key")
	dir := fs.String("dir", ".", "directory to serve")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("unable to parse options: %w", err)
	}

	if help != nil && *help {
		fs.SetOutput(os.Stdout)
		_, _ = fmt.Fprintf(os.Stdout, "Usage of gosrv:\n")
		fs.PrintDefaults()
		return nil
	}

	if host == nil || port == nil || dir == nil || *host == "" || *port == "" || *dir == "" {
		return fmt.Errorf("host, port and dir are required")
	}

	if (key == nil || *key == "") && (cert != nil && *cert != "") {
		return fmt.Errorf("key is required along with the cert")
	}
	if (key != nil && *key != "") && (cert == nil || *cert == "") {
		return fmt.Errorf("cert is required along with the key")
	}

	dirAbsPath, err := filepath.Abs(*dir)
	if err != nil {
		return fmt.Errorf("unable to determine dir absolute path: %w", err)
	}

	cleartext := (key == nil || *key == "") && (cert == nil || *cert == "")

	addr := strings.Join([]string{*host, *port}, ":")
	srv := setupHTTP(addr, dirAbsPath)

	go func() {
		handle := func(err error) {
			if err != nil {
				_, _ = fmt.Fprintf(stderr, "start server: %s\n", err.Error())
				return
			}
			_, _ = fmt.Fprintf(stdout, "closed server\n")
		}

		if cleartext {
			_, _ = fmt.Fprintf(stdout, "starting http server on %s\n", srv.Addr)
			handle(runHTTP(srv))
		} else {
			_, _ = fmt.Fprintf(stdout, "starting https server on %s\n", srv.Addr)
			handle(runHTTPTLS(srv, *cert, *key))
		}
	}()

	<-ctx.Done()
	if err := stopHTTP(srv); err != nil {
		_, _ = fmt.Fprintf(stderr, "stop server: %s\n", err.Error())
	}
	return nil
}

func setupHTTP(addr, dir string) *http.Server {
	return &http.Server{
		Addr:         addr,
		Handler:      http.FileServer(http.Dir(dir)),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 1 * time.Minute,
		IdleTimeout:  5 * time.Second,
	}
}

func runHTTP(srv *http.Server) error {
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

func runHTTPTLS(srv *http.Server, certFile, keyFile string) error {
	if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen and serve tls: %w", err)
	}
	return nil
}

func stopHTTP(srv *http.Server) error {
	if err := srv.Shutdown(context.Background()); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

const (
	exitCodeOnErr       = 1
	exitCodeOnInterrupt = 2
)
