package protocol

// Message types
const (
	TypeRegister     = "register"
	TypeRegisterOK   = "register_ok"
	TypeRegisterErr  = "register_error"
	TypeNewConn      = "new_conn"
	TypeJoin         = "join"
	TypeJoinOK       = "join_ok"
	TypeJoinErr      = "join_error"
	TypePing         = "ping"
	TypePong         = "pong"
	TypeShutdown     = "shutdown"
)

// BaseMessage is a minimal struct for detecting message type.
type BaseMessage struct {
	Type string `json:"type"`
}

// RegisterReq is the client->server registration request.
type RegisterReq struct {
	Type      string `json:"type"`
	MapPort   int    `json:"map_port"`
	LocalAddr string `json:"local_addr"`
	Token     string `json:"token,omitempty"`
	Name      string `json:"name,omitempty"`
}

// RegisterOK is the server->client registration success response.
type RegisterOK struct {
	Type    string `json:"type"`
	MapPort int    `json:"map_port"`
	Message string `json:"message"`
}

// RegisterError is the server->client registration failure response.
type RegisterError struct {
	Type  string `json:"type"`
	Error string `json:"error"`
}

// NewConnMsg is the server->client notification of a new public connection.
type NewConnMsg struct {
	Type    string `json:"type"`
	ConnID  string `json:"conn_id"`
	MapPort int    `json:"map_port"`
	SrcAddr string `json:"src_addr,omitempty"`
}

// JoinReq is the client->server data connection join request.
type JoinReq struct {
	Type   string `json:"type"`
	ConnID string `json:"conn_id"`
	Token  string `json:"token,omitempty"`
}

// JoinOK is the server->client join success response.
type JoinOK struct {
	Type   string `json:"type"`
	ConnID string `json:"conn_id"`
}

// JoinError is the server->client join failure response.
type JoinError struct {
	Type   string `json:"type"`
	Error  string `json:"error"`
	ConnID string `json:"conn_id,omitempty"`
}

// PingMsg is the client->server heartbeat.
type PingMsg struct {
	Type string `json:"type"`
	Ts   int64  `json:"ts"`
}

// PongMsg is the server->client heartbeat response.
type PongMsg struct {
	Type string `json:"type"`
	Ts   int64  `json:"ts"`
}

// ShutdownMsg is the server->client shutdown notification.
type ShutdownMsg struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}
