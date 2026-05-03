package server

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"

	"gopm/internal/common"
	"gopm/internal/protocol"
)

// Mapping holds a single port mapping registration.
type Mapping struct {
	MapPort     int
	LocalAddr   string
	ClientName  string
	ControlConn net.Conn
	Writer      *bufio.Writer
	writeMu     sync.Mutex // protects Writer from concurrent writes
	Reader      *bufio.Reader
	Listener    net.Listener
	LastSeen    time.Time
	done        chan struct{}
}

// PendingConn holds a public connection waiting for a client data connection.
type PendingConn struct {
	ConnID    string
	PublicConn net.Conn
	MapPort   int
	CreatedAt time.Time
	timer     *time.Timer
}

// Server is the gopm server instance.
type Server struct {
	controlPort int
	token       string
	verbose     bool

	mu       sync.RWMutex
	mappings map[int]*Mapping          // map_port -> Mapping
	pending  map[string]*PendingConn   // conn_id -> PendingConn
	done     chan struct{}             // signals server shutdown
	ln       net.Listener
}
// NewServer creates a new server instance.
func NewServer(controlPort int, token string, verbose bool) *Server {
	return &Server{
		controlPort: controlPort,
		token:       token,
		verbose:     verbose,
		mappings:    make(map[int]*Mapping),
		pending:     make(map[string]*PendingConn),
		done:        make(chan struct{}),
	}
}

// Run starts the server and blocks until Shutdown is called or a fatal error occurs.
func (s *Server) Run() error {
	addr := fmt.Sprintf(":%d", s.controlPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	s.ln = ln
	common.Info("server listening on %s", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil // graceful shutdown
			default:
				common.Warn("accept error: %v", err)
				continue
			}
		}
		go s.handleConn(conn)
	}
}

// Shutdown gracefully stops the server, closing all mappings and the listener.
func (s *Server) Shutdown() {
	close(s.done)
	if s.ln != nil {
		s.ln.Close()
	}
	s.mu.Lock()
	for port := range s.mappings {
		s.removeMappingLocked(port)
	}
	s.mu.Unlock()
}

func (s *Server) handleConn(conn net.Conn) {
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Read first message to determine connection type
	msgType, data, err := protocol.ReadMessage(reader)
	if err != nil {
		common.Warn("failed to read first message from %s: %v", conn.RemoteAddr(), err)
		conn.Close()
		return
	}

	switch msgType {
	case protocol.TypeRegister:
		var req protocol.RegisterReq
		if err := protocol.DecodeMessage(data, &req); err != nil {
			s.sendError(conn, writer, protocol.TypeRegisterErr, "invalid register message")
			return
		}
		s.handleRegister(conn, reader, writer, &req)

	case protocol.TypeJoin:
		var req protocol.JoinReq
		if err := protocol.DecodeMessage(data, &req); err != nil {
			s.sendError(conn, writer, protocol.TypeJoinErr, "invalid join message")
			return
		}
		s.handleJoin(conn, reader, writer, &req)

	default:
		common.Warn("unknown first message type %q from %s", msgType, conn.RemoteAddr())
		conn.Close()
	}
}

func (s *Server) handleRegister(conn net.Conn, reader *bufio.Reader, writer *bufio.Writer, req *protocol.RegisterReq) {
	// Validate token
	if !s.checkToken(req.Token) {
		s.sendError(conn, writer, protocol.TypeRegisterErr, "unauthorized")
		return
	}

	// Validate map port
	if req.MapPort <= 0 || req.MapPort > 65535 {
		s.sendError(conn, writer, protocol.TypeRegisterErr, "invalid_map_port")
		return
	}

	// Check port conflict
	s.mu.Lock()
	if _, exists := s.mappings[req.MapPort]; exists {
		s.mu.Unlock()
		s.sendError(conn, writer, protocol.TypeRegisterErr, fmt.Sprintf("map port %d already in use", req.MapPort))
		return
	}

	// Start map port listener
	mapLn, err := net.Listen("tcp", fmt.Sprintf(":%d", req.MapPort))
	if err != nil {
		s.mu.Unlock()
		s.sendError(conn, writer, protocol.TypeRegisterErr, fmt.Sprintf("listen failed: %v", err))
		return
	}

	m := &Mapping{
		MapPort:     req.MapPort,
		LocalAddr:   req.LocalAddr,
		ClientName:  req.Name,
		ControlConn: conn,
		Writer:      writer,
		Reader:      reader,
		Listener:    mapLn,
		LastSeen:    time.Now(),
		done:        make(chan struct{}),
	}

	s.mappings[req.MapPort] = m
	s.mu.Unlock()

	// Send register_ok
	m.writeMu.Lock()
	err = protocol.WriteMessage(writer, &protocol.RegisterOK{
		Type:    protocol.TypeRegisterOK,
		MapPort: req.MapPort,
		Message: "ok",
	})
	m.writeMu.Unlock()
	if err != nil {
		common.Warn("failed to send register_ok: %v", err)
		s.removeMapping(req.MapPort)
		return
	}

	name := req.Name
	if name == "" {
		name = conn.RemoteAddr().String()
	}
	common.Info("client registered: name=%s map=%d local=%s", name, req.MapPort, req.LocalAddr)
	common.Info("mapping listen started on :%d", req.MapPort)

	// Start accepting public connections on map port
	go s.acceptPublicConns(m)

	// Start heartbeat health check
	go s.healthCheck(m)

	// Read loop for control messages (ping)
	s.controlReadLoop(m)
}

func (s *Server) controlReadLoop(m *Mapping) {
	defer func() {
		s.removeMapping(m.MapPort)
	}()

	for {
		msgType, data, err := protocol.ReadMessage(m.Reader)
		if err != nil {
			common.Info("control connection lost for map=%d", m.MapPort)
			return
		}

		m.LastSeen = time.Now()

		switch msgType {
		case protocol.TypePing:
			var ping protocol.PingMsg
			if err := protocol.DecodeMessage(data, &ping); err != nil {
				continue
			}
			m.writeMu.Lock()
			protocol.WriteMessage(m.Writer, &protocol.PongMsg{
				Type: protocol.TypePong,
				Ts:   ping.Ts,
			})
			m.writeMu.Unlock()
			common.Debug("ping/pong map=%d", m.MapPort)

		default:
			common.Warn("unexpected message type %q on control connection map=%d", msgType, m.MapPort)
		}
	}
}

func (s *Server) acceptPublicConns(m *Mapping) {
	for {
		pubConn, err := m.Listener.Accept()
		if err != nil {
			select {
			case <-m.done:
				return // mapping removed
			default:
				common.Warn("accept error on map port %d: %v", m.MapPort, err)
				return
			}
		}

		go s.handlePublicConn(m, pubConn)
	}
}

func (s *Server) handlePublicConn(m *Mapping, pubConn net.Conn) {
	connID := common.GenerateConnID()
	srcAddr := pubConn.RemoteAddr().String()

	common.Info("new visitor map=%d src=%s conn_id=%s", m.MapPort, srcAddr, connID)

	// Create pending entry
	pending := &PendingConn{
		ConnID:     connID,
		PublicConn: pubConn,
		MapPort:    m.MapPort,
		CreatedAt:  time.Now(),
	}

	// Set timeout timer
	pending.timer = time.AfterFunc(10*time.Second, func() {
		s.mu.Lock()
		delete(s.pending, connID)
		s.mu.Unlock()
		common.Warn("pending timeout conn_id=%s", connID)
		pubConn.Close()
	})

	s.mu.Lock()
	s.pending[connID] = pending
	s.mu.Unlock()

	// Notify client via control connection (protected by writeMu)
	m.writeMu.Lock()
	err := protocol.WriteMessage(m.Writer, &protocol.NewConnMsg{
		Type:    protocol.TypeNewConn,
		ConnID:  connID,
		MapPort: m.MapPort,
		SrcAddr: srcAddr,
	})
	m.writeMu.Unlock()

	if err != nil {
		common.Warn("failed to send new_conn to client: %v", err)
		s.mu.Lock()
		delete(s.pending, connID)
		s.mu.Unlock()
		pending.timer.Stop()
		pubConn.Close()
	}

}
// healthCheck monitors heartbeat timeout for a mapping.
// If no message is received within 45 seconds, the mapping is removed.
func (s *Server) healthCheck(m *Mapping) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if time.Since(m.LastSeen) > 45*time.Second {
				common.Warn("heartbeat timeout for map=%d, removing mapping", m.MapPort)
				s.removeMapping(m.MapPort)
				return
			}
		case <-m.done:
			return
		}
	}
}

func (s *Server) handleJoin(conn net.Conn, reader *bufio.Reader, writer *bufio.Writer, req *protocol.JoinReq) {
	// Validate token
	if !s.checkToken(req.Token) {
		s.sendError(conn, writer, protocol.TypeJoinErr, "unauthorized")
		return
	}

	s.mu.Lock()
	pending, ok := s.pending[req.ConnID]
	if !ok {
		s.mu.Unlock()
		s.sendError(conn, writer, protocol.TypeJoinErr, "conn_id not found")
		return
	}
	// Remove from pending and stop timeout timer
	delete(s.pending, req.ConnID)
	pending.timer.Stop()
	s.mu.Unlock()

	// Send join_ok
	if err := protocol.WriteMessage(writer, &protocol.JoinOK{
		Type:   protocol.TypeJoinOK,
		ConnID: req.ConnID,
	}); err != nil {
		common.Warn("failed to send join_ok: %v", err)
		conn.Close()
		pending.PublicConn.Close()
		return
	}

	common.Info("join success conn_id=%s", req.ConnID)

	// Bridge: publicConn <-> dataConn
	go common.PipeConns(pending.PublicConn, conn)
}

func (s *Server) removeMapping(mapPort int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeMappingLocked(mapPort)
}

// removeMappingLocked removes a mapping. Caller must hold s.mu.
func (s *Server) removeMappingLocked(mapPort int) {
	m, ok := s.mappings[mapPort]
	if !ok {
		return
	}

	// Signal accept loop and healthCheck to stop
	close(m.done)

	// Close listener
	if m.Listener != nil {
		m.Listener.Close()
	}

	// Close control connection
	if m.ControlConn != nil {
		m.ControlConn.Close()
	}

	// Clean up pending connections for this map port
	for id, p := range s.pending {
		if p.MapPort == mapPort {
			p.timer.Stop()
			p.PublicConn.Close()
			delete(s.pending, id)
		}
	}

	delete(s.mappings, mapPort)
	common.Info("client disconnected, mapping removed: :%d", mapPort)
}

func (s *Server) checkToken(clientToken string) bool {
	if s.token == "" {
		return true
	}
	return s.token == clientToken
}

func (s *Server) sendError(conn net.Conn, writer *bufio.Writer, msgType, errMsg string) {
	switch msgType {
	case protocol.TypeRegisterErr:
		protocol.WriteMessage(writer, &protocol.RegisterError{Type: msgType, Error: errMsg})
	case protocol.TypeJoinErr:
		protocol.WriteMessage(writer, &protocol.JoinError{Type: msgType, Error: errMsg})
	default:
		protocol.WriteMessage(writer, map[string]string{"type": msgType, "error": errMsg})
	}
	conn.Close()
}
