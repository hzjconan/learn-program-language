package api

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/hzjconan/learn-program-language/go/internal/apperr"
)

// FuzzParseListFilter 让 Go 自己找会让解析器出问题的查询串（D15 §5）。
//
// ⭐ 注意本文件是 `package api`（白盒），不是 `api_test` ——
// parseListFilter 是小写的，只能从包内测。这是 D6 讲的
// 「黑盒 vs 白盒」的实际用法：**测内部函数就得白盒**。
//
// TODO(D15)：实现我。
//
// # 怎么写
//
//	f.Add(...)                    加几个种子语料（正常的 + 边界的）
//	f.Fuzz(func(t, s string) {    s 是 fuzz 引擎生成的查询串
//	    req := httptest.NewRequest("GET", "/orders?"+s, nil)
//	    filter, err := parseListFilter(req)
//	    ...断言不变量...
//	})
//
// # ⭐ fuzz 测的是【不变量】，不是【具体输出】
//
// 你没法预期每个随机输入的正确结果，但你能说出这些必然成立的话：
//
//	① 【绝不 panic】—— 这是 fuzz 最主要的价值
//	② 出错时，错误必须是 apperr.KindInvalid（不能是 Internal，
//	   那意味着解析器把「用户输入不合法」当成了「服务器出问题」→ 500）
//	③ 解析成功时，Limit 不能是负数
//	④ 解析成功时，Customer/Status 要么是 nil 要么指向一个确定的值
//
// # 种子语料建议
//
//	""                         空
//	"limit=20"                 正常
//	"customer=&status=paid"    空值 + 正常值（§D12 那条「没传」vs「传空」）
//	"limit=-1"                 边界
//	"limit=99999999999999999999"  溢出
//	"customer=%zz"             非法百分号编码
//
// # 跑
//
//	go test -fuzz=FuzzParseListFilter -fuzztime=30s ./internal/api/
//
// ⚠️ 找到反例时它会存进 testdata/fuzz/FuzzParseListFilter/，
// 之后每次普通 `go test` 都会重跑那个输入 —— **自动变成回归测试**。
// ⭐ 那个文件要【提交进仓库】，别当成临时产物删掉。
func FuzzParseListFilter(f *testing.F) {
	// ① 种子语料：正常的 + 边界的
	f.Add("")
	f.Add("limit=20")
	f.Add("customer=alice&status=pending")
	f.Add("customer=alice&status=paid")
	f.Add("customer=alice&status=cancelled")
	f.Add("customer=alice&status=shipped")
	f.Add("customer=alice&status=pending&limit=10")
	f.Add("customer=") // 空值（D12 §1.2 那条
	f.Add("limit=-1")
	f.Add("limit=99999999999999999999") // 溢出
	f.Add("customer=%zz")               // 非法百分号编码
	f.Add("limit=20&limit=30")          // 重复参数

	f.Fuzz(func(t *testing.T, rawQuery string) {
		// 直接造一个 *http.Request，不走 httptest 的 target 解析，避免下面这种写法
		// rawQuery是一个空格的时候，拼出的url是不合法的，直接出错，走不到后面的代码
		// req := httptest.NewRequest("GET", "/orders?"+rawQuery, nil)
		req := &http.Request{
			Method: "GET",
			URL:    &url.URL{Path: "/orders", RawQuery: rawQuery},
		}

		filter, err := parseListFilter(req)

		if err != nil {
			kind, ok := apperr.KindOf(err)
			if !ok || kind != apperr.KindInvalid {
				t.Fatalf("解析失败时必须返回 Invalid，得到 kind=%v ok=%v err=%v\n"+
					"（⚠️ Internal 会变成 500 —— 把用户输入问题报成了服务器故障）",
					kind, ok, err)
			}
			return
		}

		// 解析成功时的不变量
		if filter.Limit < 0 {
			t.Fatalf("解析成功却返回了负数 limit: %d（rawQuery=%q）", filter.Limit, rawQuery)
		}
	})
}
