package ssh

import (
	"net"
	"time"

	"github.com/gravitational/trace"
	"golang.org/x/crypto/ssh"
)

// Dialer adds timeout to the SSH handshake
type Dialer struct {
}

func (s *Dialer) Dial(network, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	d := &net.Dialer{
		Timeout:   defaultDialTimeout,
		KeepAlive: defaultKeepAlive,
	}
	conn, err := d.Dial(network, addr)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(defaultDialTimeout)); err != nil {
		return nil, trace.Wrap(err)
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	err = conn.SetReadDeadline(time.Time{})
	if err != nil {
		c.Close()
		return nil, trace.Wrap(err)
	}
	return ssh.NewClient(c, chans, reqs), nil
}

const (
	defaultDialTimeout = 30 * time.Second
	defaultKeepAlive   = 5 * time.Second
)
