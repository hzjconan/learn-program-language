package store

import (
	"errors"
	"strings"
	"testing"
)

// ---------- 测试替身 ----------
//
// 这几个假实现能存在，全靠 CopyAll / Count 依赖的是【接口】而不是 *MemStore。
// 这就是依赖倒置给你的东西：不用起数据库、不用改生产代码，就能注入故障。

// errDiskFull 是假的底层故障，用来验证错误有没有被 %w 正确包装。
var errDiskFull = errors.New("磁盘满了")

// getterOnly 只实现 Getter，【没有】Len —— 用来验证 Count 的降级分支。
type getterOnly struct{ records map[string]Record }

func (g getterOnly) Get(id string) (Record, error) {
	if r, ok := g.records[id]; ok {
		return r, nil
	}
	return Record{}, ErrNotFound
}

// failAfterPutter 成功写入 n 条之后开始报错。
type failAfterPutter struct {
	n   int
	got []Record
}

func (p *failAfterPutter) Put(r Record) error {
	if len(p.got) >= p.n {
		return errDiskFull
	}
	p.got = append(p.got, r)
	return nil
}

// ---------- 编译期断言 ----------

var (
	_ Getter = getterOnly{}
	_ Putter = (*failAfterPutter)(nil)
)

// ---------- MemStore ----------

// TestMemStoreZeroValueUsable 验证不需要构造函数（D4 §1「零值可用」）。
func TestMemStoreZeroValueUsable(t *testing.T) {
	var m MemStore // 注意：没有 NewMemStore()

	if err := m.Put(Record{ID: "u1", Name: "小张"}); err != nil {
		t.Fatalf("零值 MemStore 上 Put 失败: %v\n"+
			"（nil map 可读不可写，Put 里要惰性初始化 m.records）", err)
	}

	got, err := m.Get("u1")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.Name != "小张" {
		t.Errorf("Get(\"u1\").Name = %q, want %q", got.Name, "小张")
	}
}

func TestMemStorePutOverwrite(t *testing.T) {
	var m MemStore
	_ = m.Put(Record{ID: "u1", Name: "旧名字"})
	_ = m.Put(Record{ID: "u1", Name: "新名字"})

	got, err := m.Get("u1")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.Name != "新名字" {
		t.Errorf("同 ID 应该覆盖：Name = %q, want %q", got.Name, "新名字")
	}
	if n := m.Len(); n != 1 {
		t.Errorf("覆盖不该增加条数：Len() = %d, want 1", n)
	}
}

// TestMemStoreGetNotFound 同时检查两件事：
// 哨兵错误能不能被 errors.Is 认出来，以及错误消息里有没有带上 id。
func TestMemStoreGetNotFound(t *testing.T) {
	var m MemStore
	_, err := m.Get("不存在的id")

	if err == nil {
		t.Fatal("Get 不存在的 id 应该返回错误")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false，err = %v\n"+
			"（是不是用了 fmt.Errorf 的 %%v 而不是 %%w？只有 %%w 才能被 errors.Is 穿透）", err)
	}
	if !strings.Contains(err.Error(), "不存在的id") {
		t.Errorf("错误消息里没有带上 id：%q\n"+
			"（线上只看到「记录不存在」是没法排查的 —— 是哪条记录？）", err)
	}
}

// ---------- Validate 与 ValidationError ----------

func TestValidationErrorMessage(t *testing.T) {
	e := &ValidationError{Field: "ID", Reason: "不能为空"}
	want := "字段 ID 不合法：不能为空"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		in        Record
		wantField string // "" 表示应该通过校验
	}{
		{name: "合法", in: Record{ID: "u1", Name: "小张"}, wantField: ""},
		{name: "合法：Tags 可以为空", in: Record{ID: "u1", Name: "小张", Tags: nil}, wantField: ""},
		{name: "ID 为空", in: Record{Name: "小张"}, wantField: "ID"},
		{name: "Name 为空", in: Record{ID: "u1"}, wantField: "Name"},
		{name: "两个都空时先报 ID", in: Record{}, wantField: "ID"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Validate(tt.in)
			if tt.wantField == "" {
				if got != nil {
					t.Errorf("Validate() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("Validate() = nil, want 字段 %s 的错误", tt.wantField)
			}
			if got.Field != tt.wantField {
				t.Errorf("Validate().Field = %q, want %q", got.Field, tt.wantField)
			}
		})
	}
}

// ---------- ⚠️ 今天的题眼 ----------

// TestSaveReturnsTrueNil 抓的就是 D5 §4 那个坑。
//
// 如果 Save 写成 `return Validate(r)`，这个测试会失败 —— 因为 Validate 返回的
// nil 是 *ValidationError 类型的 nil，装进 error 接口后变成 (*ValidationError, nil)，
// 而接口只有【两格都空】才 == nil。
func TestSaveReturnsTrueNil(t *testing.T) {
	var m MemStore
	err := Save(&m, Record{ID: "u1", Name: "小张"}) // 完全合法的记录

	if err != nil {
		t.Fatalf("Save 合法记录却返回了非 nil 的 error：\n"+
			"    err      = %v\n"+
			"    %%T       = %T      ← 如果这里是 *store.ValidationError，你就踩中了\n"+
			"    err==nil = %v\n"+
			"（别写 return Validate(r)。Validate 返回的是具体类型，它的 nil 装进\n"+
			" error 接口后类型那一格非空，整个接口就不等于 nil 了。\n"+
			" 正确写法：if verr := Validate(r); verr != nil { return verr }; ... return nil）",
			err, err, err == nil)
	}

	// 顺带确认它真的写进去了
	if _, err := m.Get("u1"); err != nil {
		t.Errorf("Save 之后 Get 不到：%v", err)
	}
}

func TestSaveInvalid(t *testing.T) {
	var m MemStore
	err := Save(&m, Record{Name: "小张"}) // 缺 ID

	if err == nil {
		t.Fatal("Save 不合法的记录应该返回错误")
	}

	// errors.As 把 error 接口还原回具体类型，这样才能读 Field
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("errors.As 拿不到 *ValidationError，err = %v (%T)", err, err)
	}
	if verr.Field != "ID" {
		t.Errorf("verr.Field = %q, want %q", verr.Field, "ID")
	}

	if n := m.Len(); n != 0 {
		t.Errorf("校验失败不该写入：Len() = %d, want 0", n)
	}
}

func TestSavePropagatesPutError(t *testing.T) {
	p := &failAfterPutter{n: 0} // 第一次 Put 就失败
	err := Save(p, Record{ID: "u1", Name: "小张"})

	if err == nil {
		t.Fatal("底层 Put 失败时 Save 必须把错误传出来")
	}
	if !errors.Is(err, errDiskFull) {
		t.Errorf("errors.Is(err, errDiskFull) = false，err = %v\n（包装要用 %%w）", err)
	}
}

// ---------- 可选接口 ----------

func TestCount(t *testing.T) {
	var m MemStore
	_ = m.Put(Record{ID: "u1", Name: "a"})
	_ = m.Put(Record{ID: "u2", Name: "b"})

	if got := Count(&m); got != 2 {
		t.Errorf("Count(*MemStore) = %d, want 2（*MemStore 实现了 Counter）", got)
	}

	// getterOnly 没有 Len 方法 —— 断言失败，应该降级
	g := getterOnly{records: map[string]Record{"u1": {ID: "u1"}}}
	if got := Count(g); got != -1 {
		t.Errorf("Count(不支持计数的实现) = %d, want -1\n"+
			"（用 comma-ok 断言 Counter，探测不到就降级，别 panic）", got)
	}
}

// ---------- 依赖倒置 ----------

func TestCopyAll(t *testing.T) {
	src := getterOnly{records: map[string]Record{
		"u1": {ID: "u1", Name: "小张"},
		"u2": {ID: "u2", Name: "小王"},
		"u3": {ID: "u3", Name: "小李"},
	}}

	var dst MemStore
	n, err := CopyAll(&dst, src, []string{"u1", "u2", "u3"})
	if err != nil {
		t.Fatalf("CopyAll 失败: %v", err)
	}
	if n != 3 {
		t.Errorf("copied = %d, want 3", n)
	}
	if got := dst.Len(); got != 3 {
		t.Errorf("dst.Len() = %d, want 3", got)
	}
}

// TestCopyAllSkipsNotFound 验证 ErrNotFound 是【跳过】不是【失败】。
func TestCopyAllSkipsNotFound(t *testing.T) {
	src := getterOnly{records: map[string]Record{
		"u1": {ID: "u1", Name: "小张"},
		"u3": {ID: "u3", Name: "小李"},
	}}

	var dst MemStore
	n, err := CopyAll(&dst, src, []string{"u1", "不存在", "u3"})
	if err != nil {
		t.Fatalf("不存在的 id 应该被跳过，而不是返回错误：%v", err)
	}
	if n != 2 {
		t.Errorf("copied = %d, want 2（只算真的拷过去的）", n)
	}
}

// TestCopyAllPropagatesPutError 验证三件事：错误传播、%w 包装、出错前的计数。
func TestCopyAllPropagatesPutError(t *testing.T) {
	src := getterOnly{records: map[string]Record{
		"u1": {ID: "u1", Name: "小张"},
		"u2": {ID: "u2", Name: "小王"},
		"u3": {ID: "u3", Name: "小李"},
	}}

	dst := &failAfterPutter{n: 2} // 写两条之后开始报错
	n, err := CopyAll(dst, src, []string{"u1", "u2", "u3"})

	if err == nil {
		t.Fatal("dst 报错时 CopyAll 必须立即返回错误")
	}
	if !errors.Is(err, errDiskFull) {
		t.Errorf("errors.Is(err, errDiskFull) = false，err = %v\n（包装要用 %%w，不然调用方没法判断故障类型）", err)
	}
	if !strings.Contains(err.Error(), "u3") {
		t.Errorf("错误消息里没说是哪个 id 出的问题：%q\n"+
			"（拷 1000 条时只报「磁盘满了」，你怎么知道从哪续传？）", err)
	}
	if n != 2 {
		t.Errorf("copied = %d, want 2（出错前已经成功的条数，不是 0）", n)
	}
}

func TestCopyAllEmpty(t *testing.T) {
	var dst MemStore
	n, err := CopyAll(&dst, getterOnly{}, nil)
	if err != nil || n != 0 {
		t.Errorf("CopyAll(空 ids) = (%d, %v), want (0, nil)", n, err)
	}
}
