package logx

import (
	"errors"
	"testing"
)

type errorWriter struct {
	failAtCallN int
}

func (w *errorWriter) Write(p []byte) (n int, err error) {
	w.failAtCallN--
	if w.failAtCallN > 0 {
		return len(p), nil
	}
	return 0, errors.New("模拟写文件出错")
}

func TestFormat_Extra_Errors(t *testing.T) {
	testf := TextFormatter{}
	r := sampleReport(t)

	if err := testf.Format(&errorWriter{3}, r); err == nil {
		t.Errorf("模拟写入文件时出错, 实际没有返回error")
	}
}
