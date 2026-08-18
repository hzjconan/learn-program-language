package store

import (
	"fmt"
)

func ExampleRecord() {
	fmt.Printf("%#v", Record{
		ID:   "u1",
		Name: "我是第1条记录",
		Tags: []string{"tag1", "tag2"},
	})
	// Output: store.Record{ID:"u1", Name:"我是第1条记录", Tags:[]string{"tag1", "tag2"}}
}
