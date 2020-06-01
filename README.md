# gosrv

gosrv is a simple HTTP file server designed for prototyping applications. It
is secure by default (ie. HTTPS first). The main goal is to ease the
introduction of HTTPS while developing client side web applications.

## Usage 

**With HTTPS**

This is the recommended way to use `gosrv`. SSL cert and key can be generated
easily with the steps mentioned below.

```console
$ cd /dir/to/serve
$ gosrv -cert /path/to/ssl/cert -key /path/to/ssl/key
```

**With HTTP (insecure)**

Although this mode should not be used, but option is available, just launch the
server without the `-cert` and `-key` flags.

```console
$ cd /dir/to/serve
$ gosrv
```

**Custom host and port**

By default, server runs on `localhost:8080` but you can change this by using
`-host` and `-port` flags.

```console
$ cd /dir/to/serve
$ gosrv -host 0.0.0.0 -port 9000 
```
 
 **Serve alternate directory**

By default, server server the current working directory `.` but you can serve
any directory by using `-dir` flag.

```console
$ cd /dir/not/to/serve
$ gosrv -dir /some/other/dir/to/serve
```

## Installation

```bash
$ go get -u github.com/hemantjadon/gosrv
```

## Generating SSL cert and key for localhost

Follow these steps to generate new `cert` and `key` pair along with a `ca` file
which can be put into OS/browser's trust store. It is required to have `openssl` 
installed  beforehand, which in most cases will already be there.

1. Generate a `ca.key` file which will allow you to become your own certificate
signing authority. You will be asked a passphrase, choose a secret one. 

```console
$  openssl genrsa -des3 -out ca.key 8192
# Generates RSA private key, with 8129 bit long modulus.  
```

2. Use the key generated in the previous step to generate a `ca.pem` file which
will be registered with OS/browser's trust store.

```console
$ openssl req -x509 -new -nodes -key ca.key -sha256 -days 4096 -out ca.pem
```

3. Now generate `localhost.csr` and a `localhost.key` file. You might be asked
few questions within the generator prompt, fill them as you like, you will also
be prompted for the passphrase, again choose a secret one. 

```console
$ openssl req -new -sha256 -nodes -out localhost.csr -newkey rsa:8192 -keyout localhost.key
```

4. Now create a `localhost.ext` file which contains some description about your
certificate, the primary of which is identification information `subjectAltName`,
which essentially means which all origins this certificate is valid for, we 
are choosing `DNS.1 = localhost` and `IP.2 = 127.0.0.1` as these are the 
entities we want this certificate to be valid for.
  

```
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
IP.2 = 127.0.0.1
```

5. Now finally you can generate a certificate using all the files generated
in previous steps.

```console
$  openssl x509 -req -in localhost.csr -CA ca.pem -CAkey ca.key -CAcreateserial -out localhost.crt -days 4096 -sha256 -extfile localhost.ext
```

6. Now finally add the `ca.pem` file generated in the 2nd step to the 
OS/browser's trust store, like KeyChain Access for macOS etc.