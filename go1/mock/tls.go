//
// Written by Maxim Khitrov (June 2013)
//

package mock

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"
)

// TLS client and server configuration.
var tlsConfig = struct {
	client *tls.Config
	server *tls.Config
}{}

func init() {
	var err error
	if tlsConfig.client, tlsConfig.server, err = tlsNewConfig(); err != nil {
		panic(err)
	}
}

func tlsNewConfig() (client, server *tls.Config, err error) {
	now := time.Now()
	tpl := x509.Certificate{
		SerialNumber:          new(big.Int).SetInt64(0),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             now.UTC(),
		NotAfter:              now.Add(5 * time.Minute).UTC(),
		BasicConstraintsValid: true,
		IsCA: true,
	}
	priv, err := rsa.GenerateKey(rand.Reader, 512)
	if err != nil {
		return
	}
	crt, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &priv.PublicKey, priv)
	if err != nil {
		return
	}
	key := x509.MarshalPKCS1PrivateKey(priv)
	pair, err := tls.X509KeyPair(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: crt}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: key}),
	)
	if err != nil {
		return
	}
	root, err := x509.ParseCertificate(crt)
	if err == nil {
		server = &tls.Config{Certificates: []tls.Certificate{pair}}
		client = &tls.Config{RootCAs: x509.NewCertPool(), ServerName: "localhost"}
		client.RootCAs.AddCert(root)
	}
	return
}
