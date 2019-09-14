package ssh

import (
	"crypto/subtle"
	"io"

	"github.com/gravitational/trace"
	"golang.org/x/crypto/ssh"
)

func checkHostCertificate(certs []ssh.PublicKey, key ssh.PublicKey, addr string) bool {
	for _, cert := range certs {
		if _, ok := cert.(*ssh.Certificate); ok {
			if KeysEqual(key, cert) {
				return true
			}
		}
	}
	return false
}

func checkHostKey(entries []ssh.PublicKey, inkey ssh.PublicKey, addr string) bool {
	for _, key := range entries {
		if _, ok := key.(*ssh.Certificate); !ok {
			if KeysEqual(inkey, key) {
				return true
			}
		}
	}
	return false
}

// KeysEqual is constant time compare of the keys to avoid timing attacks
func KeysEqual(ak, bk ssh.PublicKey) bool {
	a := ssh.Marshal(ak)
	b := ssh.Marshal(bk)
	return (len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1)
}

func parseKnownHosts(bytes []byte) ([]ssh.PublicKey, error) {
	var err error
	var keys []ssh.PublicKey
	var pubKey ssh.PublicKey
	for err == nil {
		_, _, pubKey, _, bytes, err = ssh.ParseKnownHosts(bytes)
		if err == nil {
			keys = append(keys, pubKey)
		}
	}
	if err != io.EOF {
		return nil, trace.Wrap(err)
	}
	return keys, nil
}
