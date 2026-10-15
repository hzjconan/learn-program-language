package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

// TestWithTimeout_NoGoroutineLeak 覆盖 withTimeout 提前返回的两条路径：超时、调用方取消。
//
// 修复前（ch 无缓冲）两条路径都会留下一个卡在 `ch <-` 上的 goroutine，
// synctest 在 bubble 结束时报 deadlock：
//
//	goroutine N [chan send (durable), synctest bubble 1]
//
// ⚠️ 验证「修复前红」时用 -run 一次只跑一个子测试：deadlock 是 panic，
// 第一个子测试 panic 后整个测试进程就退出了，后面的根本不会跑。
func TestWithTimeout_NoGoroutineLeak(t *testing.T) {
	const (
		timeout = time.Second
		fnTime  = 2 * time.Second // fn 比超时慢，保证 withTimeout 一定提前返回
	)

	tests := []struct {
		name        string
		cancelAfter time.Duration // 0 = 调用方不取消
		wantErr     error
	}{
		{name: "超时", cancelAfter: 0, wantErr: errTimeout},
		{name: "调用方取消", cancelAfter: 500 * time.Millisecond, wantErr: context.Canceled}, // 早于超时
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if tt.cancelAfter > 0 {
					go func() {
						time.Sleep(tt.cancelAfter)
						cancel()
					}()
				}

				_, err := withTimeout(ctx, timeout, func() int {
					time.Sleep(fnTime)
					return 0
				})

				// 让 fn 跑完。不等的话 fn 的 goroutine 还在 sleep，
				// synctest 把它也算作阻塞，修复后也会报 deadlock。bubble 里时间是假的，不花真实时间。
				// ⚠️ 要放在断言【之前】：Fatalf 让本 goroutine 立刻退出，fn 还在 sleep 的话，
				// 失败消息后面会再跟一个 deadlock panic（sleep (durable)），看起来像泄漏，其实不是；
				// 而且 panic 让测试进程退出，后面的子测试都不跑了。
				time.Sleep(fnTime)

				// ⭐ 断言确实走到了要测的那条路径，否则「没泄漏」什么都不能说明
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("withTimeout err = %v, want %v", err, tt.wantErr)
				}
			})
		})
	}
}

// TestDedupAdd_CopySig 验证 add 不会持有调用方传入的 sig 切片的底层数组引用。
//
// 调用方可能从一个大 buffer 里切出 buf[:sigLen] 传入（例如复用 hash buffer），
// 如果 dedup 内部不复制，整个大 buffer 就会被 GC 挂住，造成内存泄漏。
//
// 覆盖两条路径：
//   - append 分支：len(order) < capacity 时 append
//   - 环形覆盖分支：len(order) >= capacity 时覆盖旧条目
func TestDedupAdd_CopySig(t *testing.T) {
	const capacity = 3 // 小容量，快速触发两条路径

	// 大 buffer：cap 远大于 sigLen。如果 dedup 内部不复制，
	// d.order[i] 的 cap 会是 1024 而不是 sigLen。
	buf := make([]byte, 1024)
	for i := range buf {
		buf[i] = byte(i)
	}

	d := newDedup(capacity)

	// 第一阶段：填满 capacity，走 append 分支
	// 往 buf 的不同偏移写入不同签名，每次切 buf[off:off+sigLen] 传进去
	offsets := []int{0, 100, 200} // 三个不同偏移
	for i, off := range offsets {
		copy(buf[off:off+sigLen], []byte{
			byte(i*10 + 1), byte(i*10 + 2), byte(i*10 + 3), byte(i*10 + 4),
			byte(i*10 + 5), byte(i*10 + 6), byte(i*10 + 7), byte(i*10 + 8),
			byte(i*10 + 9), byte(i*10 + 10), byte(i*10 + 11), byte(i*10 + 12),
			byte(i*10 + 13), byte(i*10 + 14), byte(i*10 + 15), byte(i*10 + 16),
		})
		if !d.add(buf[off : off+sigLen]) {
			t.Fatalf("第 %d 次 add 应该返回 true（首次签名）", i)
		}
	}

	// 此时 order 里有 3 条，全部来自 append 分支
	// 断言 1：cap 应该都是 sigLen，证明底层数组被独立分配了
	for i := 0; i < capacity; i++ {
		if cap(d.order[i]) != sigLen {
			t.Errorf("append 分支: d.order[%d].cap = %d, want %d（共享了调用方大 buffer）",
				i, cap(d.order[i]), sigLen)
		}
	}

	// 修改原始大 buffer，验证 dedup 内部的值没变
	for i, off := range offsets {
		want := make([]byte, sigLen)
		copy(want, buf[off:off+sigLen]) // 先记住期望值
		// 污染原始 buffer
		for j := range buf[off : off+sigLen] {
			buf[off+j] = 0xFF
		}
		if !bytes.Equal(d.order[i], want) {
			t.Errorf("append 分支: d.order[%d] 被原始 buffer 污染了, got %v, want %v",
				i, d.order[i], want)
		}
	}

	// 第二阶段：再 add 一条，触发环形覆盖分支（覆盖 index 0）
	off4 := 300
	newSig := []byte{
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF, 0x11, 0x22,
		0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0x00,
	}
	copy(buf[off4:off4+sigLen], newSig)
	if !d.add(buf[off4 : off4+sigLen]) {
		t.Fatalf("第 4 次 add 应该返回 true（新签名）")
	}

	// 断言 2：覆盖分支写入的那条（index 0），cap 也应该是 sigLen
	// 此时 next 指向 1，覆盖的是 index 0
	if cap(d.order[0]) != sigLen {
		t.Errorf("覆盖分支: d.order[0].cap = %d, want %d（共享了调用方大 buffer）",
			cap(d.order[0]), sigLen)
	}

	// 污染原始 buffer 的第 4 条，验证 dedup 内部没被污染
	for j := range buf[off4 : off4+sigLen] {
		buf[off4+j] = 0xFF
	}
	if !bytes.Equal(d.order[0], newSig) {
		t.Errorf("覆盖分支: d.order[0] 被原始 buffer 污染了, got %v, want %v",
			d.order[0], newSig)
	}
}
