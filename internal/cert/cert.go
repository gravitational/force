package cert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"

	"github.com/gravitational/trace"

	log "github.com/sirupsen/logrus"
)

// RSAKeySize is the size of the RSA key.
const RSAKeySize = 2048

// TLSCredentials keeps the typical 3 components of a proper HTTPS configuration
type TLSCredentials struct {
	// PublicKey in PEM format
	PublicKey []byte
	// PrivateKey in PEM format
	PrivateKey []byte
	Cert       []byte
}

// GenerateSelfSignedCert generates a self signed certificate that
// is valid for given domain names and ips, returns PEM-encoded bytes with key and cert
func GenerateSelfSignedCert(hostNames []string) (*TLSCredentials, error) {
	priv, err := rsa.GenerateKey(rand.Reader, RSAKeySize)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	notBefore := time.Now()
	notAfter := notBefore.Add(time.Hour * 24 * 365 * 10) // 10 years

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	entity := pkix.Name{
		CommonName:   "localhost",
		Country:      []string{"US"},
		Organization: []string{"localhost"},
	}

	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Issuer:                entity,
		Subject:               entity,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// collect IP addresses localhost resolves to and add them to the cert. template:
	template.DNSNames = append(hostNames, "localhost.local")
	ips, _ := net.LookupIP("localhost")
	if ips != nil {
		template.IPAddresses = append(ips, net.ParseIP("::1"))
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(priv.Public())
	if err != nil {
		log.Error(err)
		return nil, trace.Wrap(err)
	}

	return &TLSCredentials{
		PublicKey:  pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: publicKeyBytes}),
		PrivateKey: pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}),
		Cert:       pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes}),
	}, nil
}
