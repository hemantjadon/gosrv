package cli

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
)

// Input describes CLI inputs.
type Input struct {
	Help     bool
	Quiet    bool
	Dir      string
	Host     string
	Port     string
	CertFile string
	KeyFile  string
}

// Parse parses the given args into Inputs.
func Parse(name string, args []string) (*Input, error) {
	fs := newFSet(name)

	fs.SetOutput(ioutil.Discard)

	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("parse flags: %w", err)
	}

	if fs.host == nil || fs.port == nil || fs.dir == nil || *fs.host == "" || *fs.port == "" || *fs.dir == "" {
		return nil, fmt.Errorf("host, port and dir are required")
	}

	if (fs.key == nil || *fs.key == "") && (fs.cert != nil && *fs.cert != "") {
		return nil, fmt.Errorf("key is required along with the cert")
	}
	if (fs.key != nil && *fs.key != "") && (fs.cert == nil || *fs.cert == "") {
		return nil, fmt.Errorf("cert is required along with the key")
	}

	inp := Input{
		Help:     *fs.help,
		Dir:      *fs.dir,
		Host:     *fs.host,
		Port:     *fs.port,
		CertFile: *fs.cert,
		KeyFile:  *fs.key,
		Quiet:    *fs.quiet,
	}

	return &inp, nil
}

func WriteHelp(w io.Writer, name string) {
	fs := newFSet(name)
	usageLine := fmt.Sprintf("Usage for %s\n", name)
	_, _ = w.Write([]byte(usageLine))
	fs.SetOutput(w)
	fs.PrintDefaults()
}

type fset struct {
	*flag.FlagSet
	help  *bool
	host  *string
	port  *string
	cert  *string
	key   *string
	dir   *string
	quiet *bool
}

func newFSet(name string) *fset {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)

	help := fs.Bool("help", false, "show help")
	host := fs.String("host", "localhost", "host name")
	port := fs.String("port", "8080", "port number")
	cert := fs.String("cert", "", "SSL cert")
	key := fs.String("key", "", "SSL cert key")
	dir := fs.String("dir", ".", "directory to serve")
	quiet := fs.Bool("quiet", false, "disable verbose logging")

	return &fset{
		FlagSet: fs,
		help:    help,
		host:    host,
		port:    port,
		cert:    cert,
		key:     key,
		dir:     dir,
		quiet:   quiet,
	}
}
