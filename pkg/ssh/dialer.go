package ssh

import (
	"context"
	"net"
	"time"

	"github.com/gravitational/force"

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

// newClientConn is a wrapper around ssh.NewClientConn
func newClientConn(ctx context.Context,
	conn net.Conn,
	nodeAddress string,
	config *ssh.ClientConfig) (ssh.Conn, <-chan ssh.NewChannel, <-chan *ssh.Request, error) {

	log := force.Log(ctx)

	type response struct {
		conn   ssh.Conn
		chanCh <-chan ssh.NewChannel
		reqCh  <-chan *ssh.Request
		err    error
	}

	respCh := make(chan response, 1)
	go func() {
		conn, chans, reqs, err := ssh.NewClientConn(conn, nodeAddress, config)
		respCh <- response{conn, chans, reqs, err}
	}()

	select {
	case resp := <-respCh:
		if resp.err != nil {
			return nil, nil, nil, trace.Wrap(resp.err, "failed to connect to %q", nodeAddress)
		}
		return resp.conn, resp.chanCh, resp.reqCh, nil
	case <-ctx.Done():
		errClose := conn.Close()
		if errClose != nil {
			log.WithError(errClose).Errorf("failed to close connection")
		}
		// drain the channel
		resp := <-respCh
		return nil, nil, nil, trace.ConnectionProblem(resp.err, "failed to connect to %q", nodeAddress)
	}
}
