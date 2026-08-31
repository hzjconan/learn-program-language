package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
)

// Item 是存储里的一条记录。
type Item struct {
	// Key 是主键。
	Key string `json:"key"`
	// Value 是内容。
	Value string `json:"value"`
}

// Store 是一个并发安全的内存 KV 存储。
//
// 注意这里的锁 —— D9 §2 的实践：mu 放在它保护的字段【上面】，
// 所有方法用指针接收者（含 sync.Mutex 的类型不能拷贝，D9 §2）。
type Store struct {
	mu sync.RWMutex
	m  map[string]string

	// subs 管理变更事件的订阅者（见 events.go）。
	//
	// ⭐ 它有【自己的锁】，不复用 Store.mu。理由见 broadcast 的注释：
	// 广播时绝不能持有 Store.mu，否则和订阅者调 Get 会形成 AB-BA 死锁（D9 §11）。
	subs subscribers
}

// NewStore 返回一个可用的空 Store。
func NewStore() *Store {
	return &Store{m: make(map[string]string)}
}

// Get 返回 key 对应的值。
func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[key]
	return v, ok
}

// Put 写入一个键值对，返回它是否是新建的（false 表示覆盖了已有的）。
func (s *Store) Put(key, value string) (created bool) {
	// ⚠️ 注意这里【没有】用 defer Unlock —— 因为广播必须在解锁之后（见下）。
	s.mu.Lock()
	_, existed := s.m[key]
	s.m[key] = value
	s.mu.Unlock()

	// ⭐ 解锁【之后】才广播。持锁广播的话，订阅者正好在调 Get 就会死锁（D9 §11）。
	s.broadcast(Event{Op: "put", Key: key, Value: value})
	return !existed
}

// Delete 删除 key，返回它之前是否存在。
func (s *Store) Delete(key string) bool {
	s.mu.Lock()
	_, existed := s.m[key]
	delete(s.m, key)
	s.mu.Unlock()

	if existed { // 不存在就不用通知
		s.broadcast(Event{Op: "delete", Key: key})
	}
	return existed
}

// Len 返回条目数。
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.m)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// ⭐ 用 encoding/json 而不是手拼字符串 —— 它会自动转义 value 里的
	// 双引号、换行、反斜杠。手拼的话这三种输入都会产生【非法 JSON】，
	// 而 `x","injected":"yes` 这种还能往响应里注入额外字段（JSON 注入）。
	//
	// 这里忽略 Encode 的错误：header 和状态码都已经写出去了，
	// 此刻写失败通常是客户端断开，没有补救余地，也无法再改状态码（§3）。
	//nolint:errcheck // 响应已开始写，失败无法补救
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	if msg == "" {
		writeJSON(w, status, nil)
	} else {
		writeJSON(w, status, map[string]string{"error": msg})
	}
}

// NewRouter 返回 KV API 的路由。
//
// TODO(D11)：实现我。
//
// 用 Go 1.22 的新路由语法（§2），四个端点：
//
//	GET    /items/{key}   → 200 + JSON {"key":"k","value":"v"}；不存在返回 404
//	PUT    /items/{key}   → 请求体是纯文本的 value
//	                        新建返回 201，覆盖返回 200，都带 JSON body
//	DELETE /items/{key}   → 存在返回 204（无 body），不存在返回 404
//	GET    /health        → 200 + JSON {"status":"ok","items":N}
//
// 硬要求：
//
//   - **错误响应也要是 JSON**：`{"error":"消息"}`，Content-Type 为 application/json
//     ⚠️ 所以不能用 http.Error —— 它会把 Content-Type 覆盖成 text/plain（§3）
//   - **限制请求体大小**：用 http.MaxBytesReader 限制 1MB（§6），超了返回 413
//   - **必须用 r.Context()**：虽然这个内存存储用不上，但要养成习惯，
//     而且 review 时我会看（真实场景下这是 D10 那套的入口）
//   - PUT 的 key 为空、body 为空时返回 400
func NewRouter(s *Store) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /items/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		value, ok := s.Get(key)
		if !ok {
			writeErr(w, http.StatusNotFound, "键不存在")
			return
		}
		writeJSON(w, http.StatusOK, Item{Key: key, Value: value})
	})

	mux.HandleFunc("PUT /items/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		if key == "" {
			writeErr(w, http.StatusBadRequest, "键为空")
			return
		}

		// 限制请求体最大为 1 MB (1024 * 1024 字节)
		r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			var maxBytesErr *http.MaxBytesError
			if !errors.As(err, &maxBytesErr) {
				writeErr(w, http.StatusInternalServerError, "读取值失败")
			} else {
				writeErr(w, http.StatusRequestEntityTooLarge, "请求体大小超过1MB")
			}
			return
		}

		value := string(bodyBytes)
		if value == "" {
			writeErr(w, http.StatusBadRequest, "值为空")
			return
		}

		created := s.Put(key, value)
		if created {
			writeJSON(w, http.StatusCreated, Item{Key: key, Value: value})
		} else {
			writeJSON(w, http.StatusOK, Item{Key: key, Value: value})
		}
	})

	mux.HandleFunc("DELETE /items/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		if key == "" {
			writeErr(w, http.StatusBadRequest, "键为空")
			return
		}
		existed := s.Delete(key)
		if existed {
			w.WriteHeader(http.StatusNoContent)
		} else {
			writeErr(w, http.StatusNotFound, "")
		}
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "items": s.Len()})
	})

	return mux
}
