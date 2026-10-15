package main

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"
)

var errTimeout = errors.New("timeout")

// withTimeout 给一个不支持 context 的阻塞调用加超时。
//
// 超时返回 errTimeout；调用方的 ctx 被取消返回 ctx.Err()。
func withTimeout[T any](ctx context.Context, d time.Duration, fn func() T) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()

	ch := make(chan T, 1)
	go func() { ch <- fn() }()

	select {
	case v := <-ch:
		return v, nil
	case <-ctx.Done():
		var zero T
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return zero, errTimeout
		}
		return zero, ctx.Err()
	}
}

// sigLen 是去重签名的长度。
const sigLen = 16

// dedup 记住最近 capacity 条签名。满了按先进先出淘汰，所以内存有上限。
type dedup struct {
	mu       sync.Mutex
	capacity int
	set      map[string]struct{}
	order    [][]byte // 环形淘汰顺序
	next     int
}

func newDedup(capacity int) *dedup {
	return &dedup{
		capacity: capacity,
		set:      make(map[string]struct{}, capacity),
		order:    make([][]byte, 0, capacity),
	}
}

// add 记下签名；第一次见到返回 true，重复返回 false。
func (d *dedup) add(sig []byte) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.set[string(sig)]; ok {
		return false
	}
	// 拷贝传进来的切片，避免占用原始切片的底层数组导致OOM
	sig = bytes.Clone(sig)
	if len(d.order) < d.capacity {
		d.order = append(d.order, sig)
	} else {
		delete(d.set, string(d.order[d.next]))
		d.order[d.next] = sig
		d.next = (d.next + 1) % d.capacity
	}
	d.set[string(sig)] = struct{}{}
	return true
}
