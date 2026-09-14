// Package ws 提供设备命令 WebSocket 通道：
//
//	app 端连接  ws://<host>:8080/ws
//	服务端通过 POST /api/commands/push 向所有在线客户端推送
//	{"type":"command","command":"open_camera","params":{...}}
package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// client 单个 WebSocket 连接（writeMu 串行化对同一连接的写入）
type client struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

// Hub 管理所有设备端 WebSocket 连接
type Hub struct {
	mu      sync.RWMutex
	clients map[*client]struct{}
}

// upgrader WebSocket 升级器。开发/演示环境放开跨源，生产环境应收紧 CheckOrigin。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 30 * time.Second // 必须小于 pongWait
)

// NewHub 创建命令推送 Hub
func NewHub() *Hub {
	return &Hub{clients: make(map[*client]struct{})}
}

// HandleWS 升级 GET /ws 连接，维持心跳并监听断连
func (h *Hub) HandleWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // 升级失败时 upgrader 已写入响应
	}

	cl := &client{conn: conn}

	h.mu.Lock()
	h.clients[cl] = struct{}{}
	h.mu.Unlock()
	log.Println("[ws] 设备连接接入, 当前连接数:", h.Count())

	// 读循环：检测断连/异常
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

loop:
	for {
		select {
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				break loop
			}
		case <-readDone:
			break loop
		}
	}

	h.mu.Lock()
	delete(h.clients, cl)
	h.mu.Unlock()
	_ = conn.Close()
	log.Println("[ws] 设备连接断开, 当前连接数:", h.Count())
}

// BroadcastCommand 向所有在线客户端推送 {"type":"command","command":...,"params":...}，
// 返回成功送达的连接数。
func (h *Hub) BroadcastCommand(command string, params any) int {
	payload, err := json.Marshal(map[string]any{
		"type":    "command",
		"command": command,
		"params":  params,
	})
	if err != nil {
		log.Printf("[ws] 命令序列化失败: %v", err)
		return 0
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	n := 0
	for cl := range h.clients {
		cl.writeMu.Lock()
		err := cl.conn.SetWriteDeadline(time.Now().Add(writeWait))
		if err == nil {
			err = cl.conn.WriteMessage(websocket.TextMessage, payload)
		}
		cl.writeMu.Unlock()
		if err != nil {
			continue // 写失败由读循环清理断连
		}
		n++
	}
	return n
}

// Count 当前在线连接数
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
