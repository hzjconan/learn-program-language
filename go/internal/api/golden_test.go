package api

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hzjconan/learn-program-language/go/internal/orders"
)

// updateGoldens 设为 true 时，TestOrderResponseGolden 会把当前输出写进
// testdata/*.json，替换掉旧快照。
//
// 用法：go test ./internal/api/ -run Golden -args -update
var updateGoldens = flag.Bool("update", false, "更新 golden 文件")

// TestOrderResponseGolden 用 golden file 锁住响应体的形状（D15 §6）。
//
// TODO(D15)：实现我。
//
// # 怎么写
//
//	var update = flag.Bool("update", false, "更新 golden 文件")
//
//	got := json.MarshalIndent(toResponse(sample), "", "  ")
//	golden := filepath.Join("testdata", "order_response.json")
//	if *update {
//	    os.WriteFile(golden, got, 0o644)
//	    return
//	}
//	want, err := os.ReadFile(golden)
//	...比较...
//
//	go test ./internal/api/ -update    # 更新快照
//
// # ⭐ 它的价值：API 契约的变更会在 git diff 里显形
//
// 你改了个内部结构，`git diff` 里突然出现响应体的变化 ——
// review 时一眼就看到「这个改动会影响客户端」。
// 手写断言做不到这一点：加一个字段，所有断言照样通过。
//
// # ⚠️ 两个陷阱
//
//  1. **别把不稳定的东西写进去** —— CreatedAt 必须固定成一个常量时间，
//     否则每次跑都不一样。（随机 ID、map 遍历顺序同理。）
//  2. **-update 要谨慎** —— 顺手一跑，错误的输出就被固化成「期望」了。
//     ⭐ 每次 -update 之后【必须 review diff】，和 review 代码一样。
//
// # 建议的样本
//
// 一个带两个 item 的订单 + 一个 Items 为空的订单（后者验证 `[]` 而不是 `null`）。
func TestOrderResponseGolden(t *testing.T) {
	// fixedTime 是快照里用的固定时间。
	//
	// ⚠️ 绝对不能用 time.Now() —— 每次跑都不一样，快照永远对不上。
	var fixedTime = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		file  string
		order *orders.Order
	}{
		{
			name: "完整订单",
			file: "full_order_response.json",
			order: &orders.Order{
				ID: 1, Customer: "alice", Note: "尽快发货",
				Status: orders.StatusPaid, TotalCents: 2250, CreatedAt: fixedTime,
				Items: []orders.Item{
					{ID: 1, SKU: "A-1", Qty: 2, PriceCents: 500},
					{ID: 2, SKU: "B-2", Qty: 1, PriceCents: 1250},
				},
			},
		},
		{
			// ⭐ 这个用例验证两件事：
			//   ① Items 为空时是 [] 不是 null
			//   ② Note 为空时被 omitempty 省掉（字段整个不出现）
			name: "没有 item、没有备注",
			file: "no_items_no_note_order_response.json",
			order: &orders.Order{
				ID: 2, Customer: "bob",
				Status: orders.StatusPending, CreatedAt: fixedTime,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.MarshalIndent(toResponse(tc.order), "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, '\n')

			golden := filepath.Join("testdata", tc.file)

			if *updateGoldens {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("已更新 %s", golden)
				return
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("读 golden 失败（第一次跑要加 -update）: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("响应体和快照不一致\n--- 快照 ---\n%s\n--- 现在 ---\n%s", want, got)
			}
		})
	}
}
