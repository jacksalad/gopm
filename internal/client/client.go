package client

import (
	"bufio"
	"fmt"
	"net"
	"time"

	"gopm/internal/common"
	"gopm/internal/protocol"
)

// ClientConfig holds the client configuration.
type ClientConfig struct {
	ServerAddr string
	LocalAddr  string
	MapPort    int
	Token      string
	Name       string
	Retry      bool
	Verbose    bool
}

// Client is the gopm client instance.
type Client struct {
	cfg         ClientConfig
	controlConn net.Conn
	reader      *bufio.Reader
	writer      *bufio.Writer
	done        chan struct{}
}

// NewClient creates a new client instance.
func NewClient(cfg ClientConfig) *Client {
	return &Client{
		cfg:  cfg,
		done: make(chan struct{}),
	}
}

// Run starts the client main loop with optional auto-reconnect.
func (c *Client) Run() error {
	if c.cfg.Retry {
		retryIntervals := []time.Duration{1 * time.Second, 2 * time.Second, 5 * time.Second}
		retryIdx := 0

		for {
			err := c.connectAndRegister()
			if err == nil {
				err = c.runSession()
			}

			select {
			case <-c.done:
				return nil
			default:
			}

			common.Warn("control connection lost, reconnecting...")

			interval := retryIntervals[retryIdx]
			if retryIdx < len(retryIntervals)-1 {
				retryIdx++
			}

			select {
			case <-time.After(interval):
			case <-c.done:
				return nil
			}
		}
	}

	// No retry: single attempt
	if err := c.connectAndRegister(); err != nil {
		return err
	}
	return c.runSession()
}

// connectAndRegister establishes TCP connection and sends register.
func (c *Client) connectAndRegister() error {
	common.Info("connecting to server %s", c.cfg.ServerAddr)

	conn, err := net.DialTimeout("tcp", c.cfg.ServerAddr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial server: %w", err)
	}

	c.controlConn = conn
	c.reader = bufio.NewReader(conn)
	c.writer = bufio.NewWriter(conn)

	// Send register
	if err := protocol.WriteMessage(c.writer, &protocol.RegisterReq{
		Type:      protocol.TypeRegister,
		MapPort:   c.cfg.MapPort,
		LocalAddr: c.cfg.LocalAddr,
		Token:     c.cfg.Token,
		Name:      c.cfg.Name,
	}); err != nil {
		conn.Close()
		return fmt.Errorf("send register: %w", err)
	}

	// Wait for register_ok
	msgType, data, err := protocol.ReadMessage(c.reader)
	if err != nil {
		conn.Close()
		return fmt.Errorf("read register response: %w", err)
	}

	switch msgType {
	case protocol.TypeRegisterOK:
		var ok protocol.RegisterOK
		if err := protocol.DecodeMessage(data, &ok); err != nil {
			conn.Close()
			return fmt.Errorf("decode register_ok: %w", err)
		}
		common.Info("register success map=%d -> local=%s", c.cfg.MapPort, c.cfg.LocalAddr)
		return nil

	case protocol.TypeRegisterErr:
		var regErr protocol.RegisterError
		if err := protocol.DecodeMessage(data, &regErr); err != nil {
			conn.Close()
			return fmt.Errorf("register failed (decode error: %v)", err)
		}
		conn.Close()
		return fmt.Errorf("register failed: %s", regErr.Error)

	default:
		conn.Close()
		return fmt.Errorf("unexpected response type: %s", msgType)
	}
}

// runSession runs the control message read loop and heartbeat.
// Blocks until the connection is lost or shutdown.
func (c *Client) runSession() error {
	defer c.controlConn.Close()

	done := make(chan struct{})
	defer close(done)

	// Start heartbeat
	go c.heartbeatLoop(done)

	// Read loop
	for {
		msgType, data, err := protocol.ReadMessage(c.reader)
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}

		switch msgType {
		case protocol.TypeNewConn:
			var msg protocol.NewConnMsg
			if err := protocol.DecodeMessage(data, &msg); err != nil {
				common.Warn("decode new_conn error: %v", err)
				continue
			}
			go c.handleNewConn(&msg)

		case protocol.TypePong:
			common.Debug("received pong")

		case protocol.TypeShutdown:
			var msg protocol.ShutdownMsg
			if err := protocol.DecodeMessage(data, &msg); err == nil {
				common.Info("server shutdown: %s", msg.Reason)
			}
			return fmt.Errorf("server shutdown")

		default:
			common.Warn("unexpected message type: %s", msgType)
		}
	}
}

func (c *Client) heartbeatLoop(done chan struct{}) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := protocol.WriteMessage(c.writer, &protocol.PingMsg{
				Type: protocol.TypePing,
				Ts:   time.Now().Unix(),
			}); err != nil {
				common.Warn("ping failed: %v", err)
				return
			}
			common.Debug("sent ping")
		case <-done:
			return
		}
	}
}

func (c *Client) handleNewConn(msg *protocol.NewConnMsg) {
	common.Info("new_conn conn_id=%s", msg.ConnID)

	// Dial server for data connection
	dataConn, err := net.DialTimeout("tcp", c.cfg.ServerAddr, 5*time.Second)
	if err != nil {
		common.Warn("dial server for data conn failed: %v", err)
		return
	}

	dataWriter := bufio.NewWriter(dataConn)
	dataReader := bufio.NewReader(dataConn)

	// Send join
	if err := protocol.WriteMessage(dataWriter, &protocol.JoinReq{
		Type:   protocol.TypeJoin,
		ConnID: msg.ConnID,
		Token:  c.cfg.Token,
	}); err != nil {
		common.Warn("send join failed: %v", err)
		dataConn.Close()
		return
	}

	// Read join_ok
	msgType, _, err := protocol.ReadMessage(dataReader)
	if err != nil {
		common.Warn("read join response failed: %v", err)
		dataConn.Close()
		return
	}

	if msgType == protocol.TypeJoinErr {
		common.Warn("join error for conn_id=%s", msg.ConnID)
		dataConn.Close()
		return
	}

	if msgType != protocol.TypeJoinOK {
		common.Warn("unexpected join response type: %s", msgType)
		dataConn.Close()
		return
	}

	// Dial local service
	localConn, err := net.DialTimeout("tcp", c.cfg.LocalAddr, 5*time.Second)
	if err != nil {
		common.Warn("dial local %s failed: %v", c.cfg.LocalAddr, err)
		dataConn.Close()
		return
	}

	common.Info("data tunnel established: conn_id=%s local=%s", msg.ConnID, c.cfg.LocalAddr)

	// Bridge: dataConn <-> localConn
	common.PipeConns(dataConn, localConn)
}

// Stop signals the client to stop.
func (c *Client) Stop() {
	close(c.done)
	if c.controlConn != nil {
		c.controlConn.Close()
	}
}
